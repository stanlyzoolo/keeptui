package version

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestConcurrentFetch verifies that parallel FetchAndCache calls for multiple
// repos all end up in the cache (no write is lost due to a race condition).
func TestConcurrentFetch(t *testing.T) {
	repos := []string{"owner/toolA", "owner/toolB", "owner/toolC"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case len(r.URL.Path) > 0 && r.URL.Path[len(r.URL.Path)-1] == 's' &&
			len(r.URL.Path) > 10 && r.URL.Path[len(r.URL.Path)-10:] == "/languages":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int{"Go": 1000})
		case len(r.URL.Path) > 7 && r.URL.Path[len(r.URL.Path)-7:] == "/latest":
			w.Header().Set("Content-Type", "application/json")
			// Extract repo name from path for unique tag
			json.NewEncoder(w).Encode(map[string]string{
				"tag_name":     "v1.0.0",
				"body":         "release notes",
				"html_url":     "https://github.com" + r.URL.Path,
				"published_at": "2025-01-01T00:00:00Z",
			})
		default:
			// Repo info endpoint
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"archived":        false,
				"description":     "test tool",
				"stargazers_count": 42,
			})
		}
	}))
	defer srv.Close()

	dir := t.TempDir()

	// Override package-level vars for this test (restored via defer).
	origAPIBase := testAPIBase
	origCacheDir := testCacheDir
	defer func() {
		testAPIBase = origAPIBase
		testCacheDir = origCacheDir
	}()
	testAPIBase = srv.URL
	testCacheDir = dir

	var wg sync.WaitGroup
	for _, repo := range repos {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			FetchAndCache("github.com/" + r) //nolint:errcheck
		}(repo)
	}
	wg.Wait()

	cache := LoadCache()
	for _, repo := range repos {
		if _, ok := cache[repo]; !ok {
			t.Errorf("cache missing entry for %q after concurrent FetchAndCache", repo)
		}
	}
}

// TestUpdateCacheEntryConcurrentRepos verifies that parallel updateCacheEntry
// calls for distinct repos all persist — no write is lost to a lost-update race.
func TestUpdateCacheEntryConcurrentRepos(t *testing.T) {
	dir := t.TempDir()
	origCacheDir := testCacheDir
	defer func() { testCacheDir = origCacheDir }()
	testCacheDir = dir

	const m = 20
	repos := make([]string, m)
	for i := 0; i < m; i++ {
		repos[i] = "owner/tool" + string(rune('A'+i))
	}

	var wg sync.WaitGroup
	for _, repo := range repos {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			updateCacheEntry(r, func(existing CacheEntry) CacheEntry {
				existing.Latest = r
				return existing
			})
		}(repo)
	}
	wg.Wait()

	cache := LoadCache()
	if len(cache) != m {
		t.Fatalf("cache has %d entries, want %d", len(cache), m)
	}
	for _, repo := range repos {
		entry, ok := cache[repo]
		if !ok {
			t.Errorf("cache missing entry for %q", repo)
			continue
		}
		if entry.Latest != repo {
			t.Errorf("entry %q Latest = %q, want %q", repo, entry.Latest, repo)
		}
	}
}

// TestUpdateCacheEntryConcurrentSameRepo verifies that concurrent updates to a
// single repo are serialized: the final count reflects every increment, so no
// read-modify-write is lost.
func TestUpdateCacheEntryConcurrentSameRepo(t *testing.T) {
	dir := t.TempDir()
	origCacheDir := testCacheDir
	defer func() { testCacheDir = origCacheDir }()
	testCacheDir = dir

	const n = 50
	const repo = "owner/tool"

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			updateCacheEntry(repo, func(existing CacheEntry) CacheEntry {
				existing.Stars++
				return existing
			})
		}()
	}
	wg.Wait()

	cache := LoadCache()
	if got := cache[repo].Stars; got != n {
		t.Errorf("Stars = %d after %d concurrent increments, want %d", got, n, n)
	}
}

// githubTestServer returns an httptest server that answers the release, repo
// info and languages endpoints, plus a cleanup that restores the package vars.
func githubTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case len(r.URL.Path) >= 10 && r.URL.Path[len(r.URL.Path)-10:] == "/languages":
			json.NewEncoder(w).Encode(map[string]int{"Go": 1000})
		case len(r.URL.Path) >= 7 && r.URL.Path[len(r.URL.Path)-7:] == "/latest":
			json.NewEncoder(w).Encode(map[string]string{
				"tag_name":     "v2.3.4",
				"body":         "release notes",
				"html_url":     "https://github.com" + r.URL.Path,
				"published_at": "2025-01-01T00:00:00Z",
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{
				"archived":         false,
				"description":      "test tool",
				"stargazers_count": 42,
			})
		}
	}))
	origAPIBase := testAPIBase
	origCacheDir := testCacheDir
	testAPIBase = srv.URL
	testCacheDir = t.TempDir()
	t.Cleanup(func() {
		srv.Close()
		testAPIBase = origAPIBase
		testCacheDir = origCacheDir
	})
	return srv
}

// TestGetLatestReadAfterWrite verifies GetLatest persists its result through
// updateCacheEntry so a subsequent read within TTL is served from cache.
func TestGetLatestReadAfterWrite(t *testing.T) {
	githubTestServer(t)

	if got := GetLatest("github.com/owner/repo"); got != "v2.3.4" {
		t.Fatalf("GetLatest = %q, want v2.3.4", got)
	}
	entry, ok := LoadCache()["owner/repo"]
	if !ok {
		t.Fatal("cache missing entry after GetLatest")
	}
	if entry.Latest != "v2.3.4" {
		t.Errorf("cached Latest = %q, want v2.3.4", entry.Latest)
	}
	if entry.RepoStatus != "active" {
		t.Errorf("cached RepoStatus = %q, want active", entry.RepoStatus)
	}
}

// TestGetRepoCardReadAfterWrite verifies GetRepoCard persists languages and
// preserves the Latest tag already stored by an earlier GetLatest.
func TestGetRepoCardReadAfterWrite(t *testing.T) {
	githubTestServer(t)

	GetLatest("github.com/owner/repo")
	card := GetRepoCard("github.com/owner/repo")
	if card.Languages["Go"] != 1000 {
		t.Errorf("card languages = %v, want Go:1000", card.Languages)
	}
	if card.Latest != "v2.3.4" {
		t.Errorf("card Latest = %q, want v2.3.4 (preserved from GetLatest)", card.Latest)
	}
	entry := LoadCache()["owner/repo"]
	if entry.Languages == nil {
		t.Error("cached Languages nil after GetRepoCard")
	}
	if entry.Latest != "v2.3.4" {
		t.Errorf("cached Latest = %q, want v2.3.4 preserved", entry.Latest)
	}
}

// TestGetChangelogReadAfterWrite verifies GetChangelog persists body and
// preserves languages already populated by GetRepoCard.
func TestGetChangelogReadAfterWrite(t *testing.T) {
	githubTestServer(t)

	GetRepoCard("github.com/owner/repo")
	info, err := GetChangelog("github.com/owner/repo")
	if err != nil {
		t.Fatalf("GetChangelog: %v", err)
	}
	if info.Body != "release notes" {
		t.Errorf("changelog Body = %q, want release notes", info.Body)
	}
	entry := LoadCache()["owner/repo"]
	if entry.Body != "release notes" {
		t.Errorf("cached Body = %q, want release notes", entry.Body)
	}
	if entry.Languages["Go"] != 1000 {
		t.Errorf("cached Languages = %v, want Go:1000 preserved from GetRepoCard", entry.Languages)
	}
}

// TestConcurrentInitLikeFetch mimics startup: for several repos GetLatest and
// GetRepoCard run in parallel; every repo must end up with both release and
// card fields intact — no lost write between the two paths.
func TestConcurrentInitLikeFetch(t *testing.T) {
	githubTestServer(t)

	repos := []string{"owner/toolA", "owner/toolB", "owner/toolC", "owner/toolD"}
	var wg sync.WaitGroup
	for _, repo := range repos {
		field := "github.com/" + repo
		wg.Add(1)
		go func() {
			defer wg.Done()
			GetLatest(field)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			GetRepoCard(field)
		}()
	}
	wg.Wait()

	cache := LoadCache()
	for _, repo := range repos {
		entry, ok := cache[repo]
		if !ok {
			t.Errorf("cache missing entry for %q", repo)
			continue
		}
		if entry.Latest != "v2.3.4" {
			t.Errorf("%q Latest = %q, want v2.3.4", repo, entry.Latest)
		}
		if entry.Languages["Go"] != 1000 {
			t.Errorf("%q Languages = %v, want Go:1000", repo, entry.Languages)
		}
	}
}

// TestGetRepoDataSinglePass verifies GetRepoData returns latest+status+about+
// stars+languages from one pass and that a second call within TTL is served
// entirely from cache (no further network requests).
func TestGetRepoDataSinglePass(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case len(r.URL.Path) >= 10 && r.URL.Path[len(r.URL.Path)-10:] == "/languages":
			json.NewEncoder(w).Encode(map[string]int{"Go": 1000})
		case len(r.URL.Path) >= 7 && r.URL.Path[len(r.URL.Path)-7:] == "/latest":
			json.NewEncoder(w).Encode(map[string]string{
				"tag_name":     "v3.1.4",
				"body":         "release notes",
				"html_url":     "https://github.com" + r.URL.Path,
				"published_at": "2025-01-01T00:00:00Z",
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{
				"archived":         false,
				"description":      "test tool",
				"stargazers_count": 42,
			})
		}
	}))
	origAPIBase := testAPIBase
	origCacheDir := testCacheDir
	testAPIBase = srv.URL
	testCacheDir = t.TempDir()
	defer func() {
		srv.Close()
		testAPIBase = origAPIBase
		testCacheDir = origCacheDir
	}()

	d := GetRepoData("github.com/owner/repo")
	if d.Latest != "v3.1.4" {
		t.Errorf("Latest = %q, want v3.1.4", d.Latest)
	}
	if d.RepoStatus != "active" {
		t.Errorf("RepoStatus = %q, want active", d.RepoStatus)
	}
	if d.About != "test tool" {
		t.Errorf("About = %q, want test tool", d.About)
	}
	if d.Stars != 42 {
		t.Errorf("Stars = %d, want 42", d.Stars)
	}
	if d.Languages["Go"] != 1000 {
		t.Errorf("Languages = %v, want Go:1000", d.Languages)
	}

	mu.Lock()
	afterFirst := requests
	mu.Unlock()
	if afterFirst == 0 {
		t.Fatal("expected network requests on first GetRepoData")
	}

	d2 := GetRepoData("github.com/owner/repo")
	mu.Lock()
	afterSecond := requests
	mu.Unlock()
	if afterSecond != afterFirst {
		t.Errorf("second GetRepoData made %d extra requests, want 0 (cache hit)", afterSecond-afterFirst)
	}
	if d2.Latest != "v3.1.4" || d2.Languages["Go"] != 1000 {
		t.Errorf("cache-hit RepoData = %+v, want same as first call", d2)
	}
}

// TestGetRepoDataLanguagesFailureStillCaches verifies that when the languages
// endpoint fails (leaving Languages nil) but the other endpoints succeed, a
// second call within TTL is still served from cache and does not trigger a full
// three-endpoint re-fetch. This guards the regression where the cache-hit gate
// required Languages != nil, forcing a network pass on every start for any repo
// whose languages endpoint was flaky or rate-limited.
func TestGetRepoDataLanguagesFailureStillCaches(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		switch {
		case len(r.URL.Path) >= 10 && r.URL.Path[len(r.URL.Path)-10:] == "/languages":
			w.WriteHeader(http.StatusInternalServerError)
		case len(r.URL.Path) >= 7 && r.URL.Path[len(r.URL.Path)-7:] == "/latest":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"tag_name":     "v1.0.0",
				"body":         "release notes",
				"html_url":     "https://github.com" + r.URL.Path,
				"published_at": "2025-01-01T00:00:00Z",
			})
		default:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"archived":         false,
				"description":      "test tool",
				"stargazers_count": 7,
			})
		}
	}))
	origAPIBase := testAPIBase
	origCacheDir := testCacheDir
	testAPIBase = srv.URL
	testCacheDir = t.TempDir()
	defer func() {
		srv.Close()
		testAPIBase = origAPIBase
		testCacheDir = origCacheDir
	}()

	d := GetRepoData("github.com/owner/repo")
	if d.Latest != "v1.0.0" {
		t.Errorf("Latest = %q, want v1.0.0", d.Latest)
	}
	if d.Languages != nil {
		t.Errorf("Languages = %v, want nil after languages fetch failure", d.Languages)
	}

	mu.Lock()
	afterFirst := requests
	mu.Unlock()
	if afterFirst == 0 {
		t.Fatal("expected network requests on first GetRepoData")
	}

	d2 := GetRepoData("github.com/owner/repo")
	mu.Lock()
	afterSecond := requests
	mu.Unlock()
	if afterSecond != afterFirst {
		t.Errorf("second GetRepoData made %d extra requests, want 0 (cache hit despite nil Languages)", afterSecond-afterFirst)
	}
	if d2.Latest != "v1.0.0" {
		t.Errorf("cache-hit Latest = %q, want v1.0.0", d2.Latest)
	}
}

// TestGetRepoDataTotalFailureDoesNotPoisonCache verifies that when both the
// release and repo-info endpoints fail on a cold cache, GetRepoData does NOT
// write a fresh-but-empty entry. A subsequent call must retry over the network
// (not be served an empty cache hit) once the endpoints recover.
func TestGetRepoDataTotalFailureDoesNotPoisonCache(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		failing := fail
		mu.Unlock()
		if failing {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case len(r.URL.Path) >= 10 && r.URL.Path[len(r.URL.Path)-10:] == "/languages":
			json.NewEncoder(w).Encode(map[string]int{"Go": 1000})
		case len(r.URL.Path) >= 7 && r.URL.Path[len(r.URL.Path)-7:] == "/latest":
			json.NewEncoder(w).Encode(map[string]string{"tag_name": "v2.0.0"})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{
				"archived": false, "description": "test tool", "stargazers_count": 9,
			})
		}
	}))
	origAPIBase := testAPIBase
	origCacheDir := testCacheDir
	testAPIBase = srv.URL
	testCacheDir = t.TempDir()
	defer func() {
		srv.Close()
		testAPIBase = origAPIBase
		testCacheDir = origCacheDir
	}()

	d := GetRepoData("github.com/owner/repo")
	if d.Latest != "" || d.RepoStatus != "" {
		t.Errorf("first (failing) GetRepoData = %+v, want zero RepoData", d)
	}

	// Endpoints recover; a second call must retry (no poisoned cache hit).
	mu.Lock()
	fail = false
	afterFirst := requests
	mu.Unlock()

	d2 := GetRepoData("github.com/owner/repo")
	mu.Lock()
	afterSecond := requests
	mu.Unlock()
	if afterSecond == afterFirst {
		t.Fatal("second GetRepoData made no requests, cache was poisoned by total failure")
	}
	if d2.Latest != "v2.0.0" {
		t.Errorf("recovered Latest = %q, want v2.0.0", d2.Latest)
	}
}

// TestGetRepoDataStaleFallbackOnReleaseFailure verifies that when only the
// release endpoint fails on a re-fetch (repo info still succeeds), the failed
// field keeps its stale cached value while the successful fields refresh. This
// exercises the field-by-field stale fallback inside the updateCacheEntry mutate.
func TestGetRepoDataStaleFallbackOnReleaseFailure(t *testing.T) {
	var mu sync.Mutex
	failRelease := false
	stars := 11
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fr := failRelease
		curStars := stars
		mu.Unlock()
		switch {
		case len(r.URL.Path) >= 10 && r.URL.Path[len(r.URL.Path)-10:] == "/languages":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int{"Go": 500})
		case len(r.URL.Path) >= 7 && r.URL.Path[len(r.URL.Path)-7:] == "/latest":
			if fr {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"tag_name": "v1.2.3"})
		default:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"archived": false, "description": "good tool", "stargazers_count": curStars,
			})
		}
	}))
	origAPIBase := testAPIBase
	origCacheDir := testCacheDir
	testAPIBase = srv.URL
	testCacheDir = t.TempDir()
	defer func() {
		srv.Close()
		testAPIBase = origAPIBase
		testCacheDir = origCacheDir
	}()

	// Populate a good entry.
	if d := GetRepoData("github.com/owner/repo"); d.Latest != "v1.2.3" {
		t.Fatalf("setup: Latest = %q, want v1.2.3", d.Latest)
	}

	// Expire the cached entry so the next call re-fetches, fail only the release
	// endpoint, and change stars so we can confirm the repo-info fields refreshed.
	updateCacheEntry("owner/repo", func(e CacheEntry) CacheEntry {
		e.CheckedAt = time.Now().Add(-2 * cacheTTL)
		return e
	})
	mu.Lock()
	failRelease = true
	stars = 22
	mu.Unlock()

	d := GetRepoData("github.com/owner/repo")
	if d.Latest != "v1.2.3" {
		t.Errorf("stale fallback Latest = %q, want v1.2.3 preserved", d.Latest)
	}
	if d.Stars != 22 {
		t.Errorf("refreshed Stars = %d, want 22 (repo info succeeded)", d.Stars)
	}
}

// TestGetRepoDataInvalidField verifies empty and unparseable github fields
// return a zero RepoData without panicking or hitting the network.
func TestGetRepoDataInvalidField(t *testing.T) {
	for _, field := range []string{"", "onlyowner"} {
		got := GetRepoData(field)
		if got.Latest != "" || got.RepoStatus != "" || got.About != "" ||
			got.Stars != 0 || got.Languages != nil || got.Body != "" ||
			got.HtmlUrl != "" || got.PublishedAt != "" {
			t.Errorf("GetRepoData(%q) = %+v, want zero RepoData", field, got)
		}
	}
}

// TestClassifyStatusRateLimited verifies a 403 whose own header reads
// X-RateLimit-Remaining==0 classifies as ErrRateLimited, while a 403 with
// remaining>0 (genuine access denial) returns a generic error. The check reads
// the response header, not the global rl snapshot.
func TestClassifyStatusRateLimited(t *testing.T) {
	// Restore the shared snapshot afterwards so this test's dirty write doesn't
	// leak into order-dependent tests reading Rate().
	resetRate(t)
	// Seed global rl with a "healthy" remaining to prove classifyStatus ignores it.
	rlMu.Lock()
	rl = RateLimit{Limit: 5000, Remaining: 5000, Known: true}
	rlMu.Unlock()

	exhausted := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
	exhausted.Header.Set("X-RateLimit-Remaining", "0")
	if err := classifyStatus(exhausted); !errors.Is(err, ErrRateLimited) {
		t.Errorf("classifyStatus(403, remaining=0) = %v, want ErrRateLimited", err)
	}

	tooMany := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}}
	tooMany.Header.Set("X-RateLimit-Remaining", "0")
	if err := classifyStatus(tooMany); !errors.Is(err, ErrRateLimited) {
		t.Errorf("classifyStatus(429, remaining=0) = %v, want ErrRateLimited", err)
	}

	denied := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
	denied.Header.Set("X-RateLimit-Remaining", "37")
	if err := classifyStatus(denied); err == nil || errors.Is(err, ErrRateLimited) {
		t.Errorf("classifyStatus(403, remaining=37) = %v, want generic HTTP error", err)
	}

	// No rate-limit header at all → generic error even though status is 403.
	bare := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
	if err := classifyStatus(bare); err == nil || errors.Is(err, ErrRateLimited) {
		t.Errorf("classifyStatus(403, no header) = %v, want generic HTTP error", err)
	}
}

// TestFetchRateParsesCore verifies FetchRate decodes resources.core from the
// /rate_limit endpoint and updates the shared snapshot.
func TestFetchRateParsesCore(t *testing.T) {
	reset := time.Now().Add(30 * time.Minute).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rate_limit" {
			t.Errorf("unexpected path %q, want /rate_limit", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"resources": map[string]interface{}{
				"core": map[string]interface{}{
					"limit":     5000,
					"remaining": 4321,
					"reset":     reset,
				},
			},
		})
	}))
	origAPIBase := testAPIBase
	testAPIBase = srv.URL
	defer func() {
		srv.Close()
		testAPIBase = origAPIBase
	}()

	snap, err := FetchRate()
	if err != nil {
		t.Fatalf("FetchRate: %v", err)
	}
	if snap.Limit != 5000 || snap.Remaining != 4321 || !snap.Known {
		t.Errorf("FetchRate snapshot = %+v, want Limit=5000 Remaining=4321 Known=true", snap)
	}
	if snap.Reset.Unix() != reset {
		t.Errorf("FetchRate Reset = %d, want %d", snap.Reset.Unix(), reset)
	}
	if got := Rate(); got.Remaining != 4321 {
		t.Errorf("global Rate() Remaining = %d, want 4321", got.Remaining)
	}
}

// TestFetchRateWithTokenValid verifies FetchRateWithToken sends the candidate
// token and does not mutate global tokenMem.
func TestFetchRateWithTokenValid(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"resources": map[string]interface{}{
				"core": map[string]interface{}{"limit": 5000, "remaining": 5000, "reset": 0},
			},
		})
	}))
	origAPIBase := testAPIBase
	testAPIBase = srv.URL
	defer func() {
		srv.Close()
		testAPIBase = origAPIBase
	}()

	tokenMu.RLock()
	before := tokenMem
	tokenMu.RUnlock()

	snap, err := FetchRateWithToken("ghp_candidate123")
	if err != nil {
		t.Fatalf("FetchRateWithToken: %v", err)
	}
	if snap.Limit != 5000 {
		t.Errorf("snapshot Limit = %d, want 5000", snap.Limit)
	}
	if gotAuth != "Bearer ghp_candidate123" {
		t.Errorf("Authorization = %q, want Bearer ghp_candidate123", gotAuth)
	}
	tokenMu.RLock()
	after := tokenMem
	tokenMu.RUnlock()
	if after != before {
		t.Errorf("tokenMem mutated by FetchRateWithToken: %q -> %q", before, after)
	}
}

// TestFetchRateWithTokenInvalid verifies a 401 returns ErrTokenInvalid and does
// not mutate global tokenMem.
func TestFetchRateWithTokenInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	origAPIBase := testAPIBase
	testAPIBase = srv.URL
	defer func() {
		srv.Close()
		testAPIBase = origAPIBase
	}()

	tokenMu.RLock()
	before := tokenMem
	tokenMu.RUnlock()

	_, err := FetchRateWithToken("bad-token")
	if !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("FetchRateWithToken(bad) = %v, want ErrTokenInvalid", err)
	}
	tokenMu.RLock()
	after := tokenMem
	tokenMu.RUnlock()
	if after != before {
		t.Errorf("tokenMem mutated on invalid token: %q -> %q", before, after)
	}
}

// TestFetchReleaseRateLimited verifies fetchRelease surfaces ErrRateLimited when
// the endpoint returns 403 with X-RateLimit-Remaining==0.
func TestFetchReleaseRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	origAPIBase := testAPIBase
	testAPIBase = srv.URL
	defer func() {
		srv.Close()
		testAPIBase = origAPIBase
	}()

	_, err := fetchRelease("owner/repo")
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("fetchRelease on 403/remaining=0 = %v, want ErrRateLimited", err)
	}
}

// TestExtractRepo guards the delegation to loader.NormalizeRepo so a stored
// github field is normalized to "owner/repo" before it reaches the GitHub API.
func TestExtractRepo(t *testing.T) {
	tests := []struct {
		field string
		want  string
	}{
		{"github.com/owner/repo", "owner/repo"},
		{"https://github.com/owner/repo/tree/main", "owner/repo"},
		{"github.com/owner/repo.git", "owner/repo"},
		{"onlyowner", ""},
	}
	for _, tt := range tests {
		if got := extractRepo(tt.field); got != tt.want {
			t.Errorf("extractRepo(%q) = %q, want %q", tt.field, got, tt.want)
		}
	}
}
