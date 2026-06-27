package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
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
