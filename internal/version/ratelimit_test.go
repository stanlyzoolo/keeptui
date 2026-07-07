package version

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

// resetRate zeroes the shared rate snapshot so tests don't observe each other's
// leftovers. It restores the previous value via t.Cleanup.
func resetRate(t *testing.T) {
	t.Helper()
	rlMu.Lock()
	prev := rl
	rl = RateLimit{}
	rlMu.Unlock()
	t.Cleanup(func() {
		rlMu.Lock()
		rl = prev
		rlMu.Unlock()
	})
}

// TestUpdateRateFromHeadersValid verifies that well-formed X-RateLimit-* headers
// populate the snapshot and mark it Known.
func TestUpdateRateFromHeadersValid(t *testing.T) {
	resetRate(t)
	reset := time.Now().Add(30 * time.Minute).Unix()
	h := http.Header{}
	h.Set("X-RateLimit-Limit", "5000")
	h.Set("X-RateLimit-Remaining", "4321")
	h.Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))

	updateRateFromHeaders(h)

	got := Rate()
	if !got.Known {
		t.Fatal("Known = false, want true after valid headers")
	}
	if got.Limit != 5000 {
		t.Errorf("Limit = %d, want 5000", got.Limit)
	}
	if got.Remaining != 4321 {
		t.Errorf("Remaining = %d, want 4321", got.Remaining)
	}
	if got.Reset.Unix() != reset {
		t.Errorf("Reset = %d, want %d", got.Reset.Unix(), reset)
	}
}

// TestUpdateRateFromHeadersMissing verifies that a response with no rate-limit
// headers leaves the previous snapshot untouched.
func TestUpdateRateFromHeadersMissing(t *testing.T) {
	resetRate(t)
	rlMu.Lock()
	rl = RateLimit{Limit: 60, Remaining: 42, Reset: time.Unix(1000, 0), Known: true}
	rlMu.Unlock()

	updateRateFromHeaders(http.Header{})

	got := Rate()
	if got.Limit != 60 || got.Remaining != 42 || !got.Known {
		t.Errorf("snapshot changed on missing headers: %+v", got)
	}
}

// TestUpdateRateFromHeadersMalformed verifies that non-numeric header values are
// ignored and leave the previous snapshot untouched.
func TestUpdateRateFromHeadersMalformed(t *testing.T) {
	resetRate(t)
	h := http.Header{}
	h.Set("X-RateLimit-Limit", "abc")
	h.Set("X-RateLimit-Remaining", "xyz")
	h.Set("X-RateLimit-Reset", "notunix")

	updateRateFromHeaders(h)

	if got := Rate(); got.Known {
		t.Errorf("Known = true after malformed headers, want false: %+v", got)
	}
}

// TestShouldReplaceRate pins the precedence rule between rate observations:
// more usage (lower/equal Remaining) wins, a Limit change wins, anything wins
// over an unknown snapshot or an expired window; a same-window snapshot
// claiming fewer used requests (the /rate_limit staleness lie) is dropped.
func TestShouldReplaceRate(t *testing.T) {
	now := time.Now()
	future := now.Add(30 * time.Minute)
	past := now.Add(-time.Minute)
	informed := RateLimit{Limit: 5000, Remaining: 4935, Reset: future, Known: true}

	tests := []struct {
		name string
		cur  RateLimit
		snap RateLimit
		want bool
	}{
		{"unknown snap never replaces", informed, RateLimit{}, false},
		{"anything replaces unknown cur", RateLimit{}, informed, true},
		{"more usage replaces", informed, RateLimit{Limit: 5000, Remaining: 4900, Reset: future, Known: true}, true},
		{"equal usage replaces (refreshes reset)", informed, RateLimit{Limit: 5000, Remaining: 4935, Reset: future, Known: true}, true},
		{"less usage in same window is dropped", informed, RateLimit{Limit: 5000, Remaining: 5000, Reset: future, Known: true}, false},
		{"less usage after window expiry replaces", RateLimit{Limit: 5000, Remaining: 4935, Reset: past, Known: true}, RateLimit{Limit: 5000, Remaining: 5000, Reset: future, Known: true}, true},
		{"limit change replaces (token added)", RateLimit{Limit: 60, Remaining: 10, Reset: future, Known: true}, RateLimit{Limit: 5000, Remaining: 5000, Reset: future, Known: true}, true},
		{"limit change replaces (token removed)", informed, RateLimit{Limit: 60, Remaining: 60, Reset: future, Known: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldReplaceRate(tt.cur, tt.snap, now); got != tt.want {
				t.Errorf("shouldReplaceRate(%+v, %+v) = %v, want %v", tt.cur, tt.snap, got, tt.want)
			}
		})
	}
}

// TestUpdateRateFromHeadersKeepsMoreInformed verifies a same-window header
// observation with fewer used requests does not clobber the snapshot.
func TestUpdateRateFromHeadersKeepsMoreInformed(t *testing.T) {
	resetRate(t)
	future := time.Now().Add(30 * time.Minute)
	rlMu.Lock()
	rl = RateLimit{Limit: 5000, Remaining: 4935, Reset: future, Known: true}
	rlMu.Unlock()

	h := http.Header{}
	h.Set("X-RateLimit-Limit", "5000")
	h.Set("X-RateLimit-Remaining", "5000")
	h.Set("X-RateLimit-Reset", strconv.FormatInt(future.Unix(), 10))
	updateRateFromHeaders(h)

	if got := Rate(); got.Remaining != 4935 {
		t.Errorf("Remaining = %d, want informed 4935 kept", got.Remaining)
	}
}

// rateLimitServer returns an httptest server answering GET /rate_limit with the
// given core numbers in both the body and the response's own X-RateLimit-*
// headers (mimicking the live endpoint, which lies in both places the same way).
func rateLimitServer(t *testing.T, limit, remaining int, reset time.Time) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		w.Header().Set("Content-Type", "application/json")
		body := `{"resources":{"core":{"limit":` + strconv.Itoa(limit) +
			`,"remaining":` + strconv.Itoa(remaining) +
			`,"reset":` + strconv.FormatInt(reset.Unix(), 10) + `}}}`
		_, _ = w.Write([]byte(body))
	}))
	origAPIBase := testAPIBase
	testAPIBase = srv.URL
	t.Cleanup(func() {
		srv.Close()
		testAPIBase = origAPIBase
	})
	return srv
}

// TestFetchRateDoesNotClobberInformedSnapshot reproduces the [L]-overlay bug:
// after real requests counted usage (gauge 65/5000), GET /rate_limit reports a
// pristine 0/5000. FetchRate must return the informed snapshot and leave the
// shared one untouched, so the gauge never zeroes out on overlay open.
func TestFetchRateDoesNotClobberInformedSnapshot(t *testing.T) {
	resetRate(t)
	future := time.Now().Add(30 * time.Minute)
	rlMu.Lock()
	rl = RateLimit{Limit: 5000, Remaining: 4935, Reset: future, Known: true}
	rlMu.Unlock()

	rateLimitServer(t, 5000, 5000, future)

	got, err := FetchRate()
	if err != nil {
		t.Fatalf("FetchRate: %v", err)
	}
	if got.Remaining != 4935 {
		t.Errorf("FetchRate Remaining = %d, want informed 4935", got.Remaining)
	}
	if shared := Rate(); shared.Remaining != 4935 {
		t.Errorf("shared Remaining = %d, want informed 4935 kept", shared.Remaining)
	}
}

// TestFetchRateSeedsUnknownSnapshot verifies the startup path: with no prior
// observation the /rate_limit numbers are accepted as the initial seed.
func TestFetchRateSeedsUnknownSnapshot(t *testing.T) {
	resetRate(t)
	future := time.Now().Add(30 * time.Minute)
	rateLimitServer(t, 5000, 4990, future)

	got, err := FetchRate()
	if err != nil {
		t.Fatalf("FetchRate: %v", err)
	}
	if !got.Known || got.Limit != 5000 || got.Remaining != 4990 {
		t.Errorf("FetchRate = %+v, want seeded 4990/5000", got)
	}
}

// TestFetchRateAcceptsResetWindow verifies a fresh counter is accepted once the
// current snapshot's window has expired — a legitimate hourly reset must not be
// mistaken for the staleness lie.
func TestFetchRateAcceptsResetWindow(t *testing.T) {
	resetRate(t)
	past := time.Now().Add(-time.Minute)
	rlMu.Lock()
	rl = RateLimit{Limit: 5000, Remaining: 4935, Reset: past, Known: true}
	rlMu.Unlock()

	rateLimitServer(t, 5000, 5000, time.Now().Add(time.Hour))

	got, err := FetchRate()
	if err != nil {
		t.Fatalf("FetchRate: %v", err)
	}
	if got.Remaining != 5000 {
		t.Errorf("Remaining = %d, want fresh 5000 accepted after window expiry", got.Remaining)
	}
}

// TestDoGHSendsAuthorizationWhenTokenSet verifies doGH attaches a Bearer header
// when a token resolves, and that updateRateFromHeaders runs on the response.
func TestDoGHSendsAuthorizationWhenTokenSet(t *testing.T) {
	resetRate(t)
	t.Setenv("GITHUB_TOKEN", "secret-tok")

	var gotAuth, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Unix(), 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := doGH(req)
	if err != nil {
		t.Fatalf("doGH: %v", err)
	}
	resp.Body.Close()

	if gotAuth != "Bearer secret-tok" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer secret-tok")
	}
	if gotAccept != "application/vnd.github.v3+json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if r := Rate(); !r.Known || r.Limit != 5000 || r.Remaining != 4999 {
		t.Errorf("rate not accounted from response headers: %+v", r)
	}
}

// TestDoGHOmitsAuthorizationWhenEmpty verifies doGH sends no Authorization header
// when no token resolves.
func TestDoGHOmitsAuthorizationWhenEmpty(t *testing.T) {
	resetRate(t)
	t.Setenv("GITHUB_TOKEN", "")
	// Ensure the config-file token is empty by pointing at an empty temp dir and
	// resetting the lazy-load once.
	origTokenDir := testTokenDir
	testTokenDir = t.TempDir()
	loadTokenOnce = sync.Once{}
	tokenMu.Lock()
	tokenMem = ""
	tokenMu.Unlock()
	t.Cleanup(func() {
		testTokenDir = origTokenDir
		loadTokenOnce = sync.Once{}
		tokenMu.Lock()
		tokenMem = ""
		tokenMu.Unlock()
	})

	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := doGH(req)
	if err != nil {
		t.Fatalf("doGH: %v", err)
	}
	resp.Body.Close()

	if hadAuth {
		t.Error("Authorization header sent with no token, want none")
	}
}

// TestFetchersParseViaDoGH confirms the three fetchers still parse correctly now
// that they route through doGH, using the shared test server.
func TestFetchersParseViaDoGH(t *testing.T) {
	resetRate(t)
	githubTestServer(t)

	info, err := fetchRelease("owner/repo")
	if err != nil {
		t.Fatalf("fetchRelease: %v", err)
	}
	if info.Tag != "v2.3.4" {
		t.Errorf("fetchRelease Tag = %q, want v2.3.4", info.Tag)
	}

	status, about, stars, err := fetchRepoInfo("owner/repo")
	if err != nil {
		t.Fatalf("fetchRepoInfo: %v", err)
	}
	if status != "active" || about != "test tool" || stars != 42 {
		t.Errorf("fetchRepoInfo = (%q, %q, %d), want (active, test tool, 42)", status, about, stars)
	}

	langs, err := fetchLanguages("owner/repo")
	if err != nil {
		t.Fatalf("fetchLanguages: %v", err)
	}
	if langs["Go"] != 1000 {
		t.Errorf("fetchLanguages = %v, want Go:1000", langs)
	}
}
