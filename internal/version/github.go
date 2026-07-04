package version

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lepeshko/keys/internal/loader"
)

const cacheTTL = 24 * time.Hour

var cacheMu sync.Mutex

// testCacheDir overrides the cache directory in tests.
var testCacheDir string

// testAPIBase overrides the GitHub API base URL in tests.
var testAPIBase string

type CacheEntry struct {
	Latest      string         `json:"latest"`
	CheckedAt   time.Time      `json:"checked_at"`
	Body        string         `json:"body,omitempty"`
	HtmlUrl     string         `json:"html_url,omitempty"`
	PublishedAt string         `json:"published_at,omitempty"`
	RepoStatus  string         `json:"repo_status,omitempty"` // "active" or "archived"
	About       string         `json:"about,omitempty"`
	Stars       int            `json:"stars,omitempty"`
	Languages   map[string]int `json:"languages,omitempty"`
}

// RepoCard holds full repository metadata for display in the TUI.
type RepoCard struct {
	About       string
	Stars       int
	Languages   map[string]int
	Latest      string
	PublishedAt string
	HtmlUrl     string
	Body        string
	RepoStatus  string
}

// ReleaseInfo holds full release metadata from GitHub.
type ReleaseInfo struct {
	Tag         string
	Body        string
	HtmlUrl     string
	PublishedAt string
}

// RepoData is the combined result of a single network pass over a repository:
// release + repo info + languages. It carries everything the TUI needs for the
// version line and the repo card, so version and card no longer each fetch the
// repo info separately.
type RepoData struct {
	Latest      string
	RepoStatus  string
	About       string
	Stars       int
	Languages   map[string]int
	Body        string
	HtmlUrl     string
	PublishedAt string
}

func repoDataFromEntry(e CacheEntry) RepoData {
	return RepoData{
		Latest:      e.Latest,
		RepoStatus:  e.RepoStatus,
		About:       e.About,
		Stars:       e.Stars,
		Languages:   e.Languages,
		Body:        e.Body,
		HtmlUrl:     e.HtmlUrl,
		PublishedAt: e.PublishedAt,
	}
}

type Cache map[string]CacheEntry

// GetRepoData fetches release, repo info and languages for a tool's github
// field in a single pass and returns the combined data. A cache entry within
// TTL is served without any network call. On a miss it makes one call to each
// endpoint and persists the result atomically via updateCacheEntry; fields
// whose fetch fails are kept from the existing entry (stale fallback), so a
// rate-limited release does not wipe a previously known tag or card. Freshness
// is decided on CheckedAt alone: a failed languages fetch leaves Languages nil
// but must not force a full three-endpoint re-fetch on every subsequent start.
// A total failure (both release and repo info fail) is not written at all, so a
// cold-cache outage does not poison the entry as fresh-but-empty for the TTL.
func GetRepoData(githubField string) RepoData {
	if githubField == "" {
		return RepoData{}
	}
	repo := extractRepo(githubField)
	if repo == "" {
		return RepoData{}
	}

	cache := LoadCache()
	entry, cached := cache[repo]
	if cached && time.Since(entry.CheckedAt) < cacheTTL {
		return repoDataFromEntry(entry)
	}

	info, relErr := fetchRelease(repo)
	repoStatus, about, stars, infoErr := fetchRepoInfo(repo)
	langs, _ := fetchLanguages(repo)

	// Total fetch failure (offline / rate-limited): both the release and the repo
	// info endpoints failed. Do not write a fresh-but-empty entry — since freshness
	// is decided on CheckedAt alone, that would read back as a valid cache hit and
	// suppress any retry for the full TTL. Return the stale entry (zero value if the
	// cache was cold) so the next start retries.
	if relErr != nil && infoErr != nil {
		return repoDataFromEntry(entry)
	}

	var stored CacheEntry
	updateCacheEntry(repo, func(existing CacheEntry) CacheEntry {
		e := existing
		e.CheckedAt = time.Now()
		if relErr == nil {
			e.Latest = info.Tag
			e.Body = info.Body
			e.HtmlUrl = info.HtmlUrl
			e.PublishedAt = info.PublishedAt
		}
		if infoErr == nil {
			e.RepoStatus = repoStatus
			e.About = about
			e.Stars = stars
		}
		if langs != nil {
			e.Languages = langs
		}
		stored = e
		return e
	})
	return repoDataFromEntry(stored)
}

// GetLatest returns the latest release tag for a tool's github field.
// Thin wrapper over GetRepoData: uses cache when fresh, falls back to the stale
// value on network error.
func GetLatest(githubField string) string {
	return GetRepoData(githubField).Latest
}

// FetchAndCache force-fetches the latest release, bypassing the cache TTL.
// Network requests happen without holding the mutex; only the read-modify-write
// of cache.json is serialized so concurrent goroutines don't overwrite each other.
func FetchAndCache(githubField string) (string, error) {
	repo := extractRepo(githubField)
	if repo == "" {
		return "", fmt.Errorf("cannot parse github field: %q", githubField)
	}
	info, err := fetchRelease(repo)
	if err != nil {
		return "", err
	}
	repoStatus, about, stars, _ := fetchRepoInfo(repo)

	updateCacheEntry(repo, func(existing CacheEntry) CacheEntry {
		newEntry := CacheEntry{
			Latest:      info.Tag,
			Body:        info.Body,
			HtmlUrl:     info.HtmlUrl,
			PublishedAt: info.PublishedAt,
			CheckedAt:   time.Now(),
			RepoStatus:  repoStatus,
			About:       about,
			Stars:       stars,
			Languages:   existing.Languages,
		}
		if repoStatus == "" {
			newEntry.RepoStatus = existing.RepoStatus
		}
		return newEntry
	})
	return info.Tag, nil
}

// GetChangelog returns full release info for a tool's github field.
// Uses cache when fresh and body is present; fetches otherwise.
func GetChangelog(githubField string) (ReleaseInfo, error) {
	if githubField == "" {
		return ReleaseInfo{}, fmt.Errorf("no github field")
	}
	repo := extractRepo(githubField)
	if repo == "" {
		return ReleaseInfo{}, fmt.Errorf("cannot parse github field: %q", githubField)
	}

	cache := LoadCache()
	entry, cached := cache[repo]

	if cached && time.Since(entry.CheckedAt) < cacheTTL && entry.Body != "" {
		return ReleaseInfo{Tag: entry.Latest, Body: entry.Body, HtmlUrl: entry.HtmlUrl, PublishedAt: entry.PublishedAt}, nil
	}

	info, err := fetchRelease(repo)
	if err != nil {
		if cached {
			return ReleaseInfo{Tag: entry.Latest, Body: entry.Body, HtmlUrl: entry.HtmlUrl, PublishedAt: entry.PublishedAt}, nil
		}
		return ReleaseInfo{}, err
	}

	updateCacheEntry(repo, func(existing CacheEntry) CacheEntry {
		return CacheEntry{
			Latest:      info.Tag,
			Body:        info.Body,
			HtmlUrl:     info.HtmlUrl,
			PublishedAt: info.PublishedAt,
			CheckedAt:   time.Now(),
			RepoStatus:  existing.RepoStatus,
			About:       existing.About,
			Stars:       existing.Stars,
			Languages:   existing.Languages,
		}
	})
	return info, nil
}

func apiBase() string {
	if testAPIBase != "" {
		return testAPIBase
	}
	return "https://api.github.com"
}

func fetchRepoInfo(repo string) (status, about string, stars int, err error) {
	url := apiBase() + "/repos/" + repo
	req, reqErr := http.NewRequest("GET", url, nil)
	if reqErr != nil {
		return "", "", 0, reqErr
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, doErr := client.Do(req)
	if doErr != nil {
		return "", "", 0, doErr
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", 0, fmt.Errorf("GitHub API: HTTP %d", resp.StatusCode)
	}
	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", "", 0, readErr
	}
	var info struct {
		Archived        bool   `json:"archived"`
		Description     string `json:"description"`
		StargazersCount int    `json:"stargazers_count"`
	}
	if unmarshalErr := json.Unmarshal(data, &info); unmarshalErr != nil {
		return "", "", 0, unmarshalErr
	}
	if info.Archived {
		return "archived", info.Description, info.StargazersCount, nil
	}
	return "active", info.Description, info.StargazersCount, nil
}

func fetchLanguages(repo string) (map[string]int, error) {
	url := apiBase() + "/repos/" + repo + "/languages"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var langs map[string]int
	if err := json.Unmarshal(data, &langs); err != nil {
		return nil, err
	}
	return langs, nil
}

func fetchRelease(repo string) (ReleaseInfo, error) {
	url := apiBase() + "/repos/" + repo + "/releases/latest"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ReleaseInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ReleaseInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return ReleaseInfo{}, fmt.Errorf("GitHub API rate limit exceeded (set GITHUB_TOKEN to increase quota)")
	}
	if resp.StatusCode == http.StatusNotFound {
		return ReleaseInfo{}, fmt.Errorf("no releases found")
	}
	if resp.StatusCode != http.StatusOK {
		return ReleaseInfo{}, fmt.Errorf("GitHub API: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return ReleaseInfo{}, err
	}

	var release struct {
		TagName     string `json:"tag_name"`
		Body        string `json:"body"`
		HtmlUrl     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return ReleaseInfo{}, err
	}
	return ReleaseInfo{
		Tag:         release.TagName,
		Body:        release.Body,
		HtmlUrl:     release.HtmlUrl,
		PublishedAt: release.PublishedAt,
	}, nil
}

// GetRepoCard returns repository metadata for display. Thin wrapper over
// GetRepoData: reads from cache when fresh and languages are populated,
// otherwise makes one network pass shared with the version lookup.
func GetRepoCard(githubField string) RepoCard {
	d := GetRepoData(githubField)
	return RepoCard{
		About:       d.About,
		Stars:       d.Stars,
		Languages:   d.Languages,
		Latest:      d.Latest,
		PublishedAt: d.PublishedAt,
		HtmlUrl:     d.HtmlUrl,
		Body:        d.Body,
		RepoStatus:  d.RepoStatus,
	}
}

func extractRepo(githubField string) string {
	return loader.NormalizeRepo(githubField)
}

func cacheFilePath() (string, error) {
	if testCacheDir != "" {
		return filepath.Join(testCacheDir, "cache.json"), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "keys", "cache.json"), nil
}

func LoadCache() Cache {
	path, err := cacheFilePath()
	if err != nil {
		return Cache{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Cache{}
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return Cache{}
	}
	return c
}

// updateCacheEntry atomically applies mutate to a single repo's cache entry.
// It holds cacheMu across a fresh LoadCache → mutate → SaveCache cycle so that
// concurrent goroutines never overwrite each other's writes: mutate receives the
// current on-disk entry (or a zero value if absent) and must return the entry to
// store, pulling any fields it doesn't set from existing.
func updateCacheEntry(repo string, mutate func(existing CacheEntry) CacheEntry) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cache := LoadCache()
	cache[repo] = mutate(cache[repo])
	SaveCache(cache)
}

func SaveCache(c Cache) {
	path, err := cacheFilePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(path, data, 0o644) //nolint:errcheck
}
