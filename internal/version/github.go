package version

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const cacheTTL = 24 * time.Hour

type CacheEntry struct {
	Latest      string    `json:"latest"`
	CheckedAt   time.Time `json:"checked_at"`
	Body        string    `json:"body,omitempty"`
	HtmlUrl     string    `json:"html_url,omitempty"`
	PublishedAt string    `json:"published_at,omitempty"`
	RepoStatus  string    `json:"repo_status,omitempty"` // "active" or "archived"
}

// ReleaseInfo holds full release metadata from GitHub.
type ReleaseInfo struct {
	Tag         string
	Body        string
	HtmlUrl     string
	PublishedAt string
}

type Cache map[string]CacheEntry

// GetLatest returns the latest release tag for a tool's github field.
// Uses cache when fresh; fetches from GitHub API when stale.
// Falls back to stale cache value on network error.
func GetLatest(githubField string) string {
	if githubField == "" {
		return ""
	}
	repo := extractRepo(githubField)
	if repo == "" {
		return ""
	}

	cache := LoadCache()
	entry, cached := cache[repo]

	if cached && time.Since(entry.CheckedAt) < cacheTTL {
		return entry.Latest
	}

	info, err := fetchRelease(repo)
	if err != nil {
		return entry.Latest // stale value or ""
	}

	repoStatus, _ := fetchRepoInfo(repo)
	newEntry := CacheEntry{
		Latest:      info.Tag,
		Body:        info.Body,
		HtmlUrl:     info.HtmlUrl,
		PublishedAt: info.PublishedAt,
		CheckedAt:   time.Now(),
		RepoStatus:  repoStatus,
	}
	if repoStatus == "" && cached {
		newEntry.RepoStatus = entry.RepoStatus
	}
	cache[repo] = newEntry
	SaveCache(cache)
	return info.Tag
}

// GetCachedRepoStatus returns the repository status from cache without making a network request.
// Returns "active", "archived", or "" if not yet known.
func GetCachedRepoStatus(githubField string) string {
	if githubField == "" {
		return ""
	}
	repo := extractRepo(githubField)
	if repo == "" {
		return ""
	}
	cache := LoadCache()
	return cache[repo].RepoStatus
}

// FetchAndCache force-fetches the latest release, bypassing the cache TTL.
func FetchAndCache(githubField string) (string, error) {
	repo := extractRepo(githubField)
	if repo == "" {
		return "", fmt.Errorf("cannot parse github field: %q", githubField)
	}
	info, err := fetchRelease(repo)
	if err != nil {
		return "", err
	}
	repoStatus, _ := fetchRepoInfo(repo)
	cache := LoadCache()
	existing := cache[repo]
	newEntry := CacheEntry{
		Latest:      info.Tag,
		Body:        info.Body,
		HtmlUrl:     info.HtmlUrl,
		PublishedAt: info.PublishedAt,
		CheckedAt:   time.Now(),
		RepoStatus:  repoStatus,
	}
	if repoStatus == "" {
		newEntry.RepoStatus = existing.RepoStatus
	}
	cache[repo] = newEntry
	SaveCache(cache)
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

	cache[repo] = CacheEntry{Latest: info.Tag, Body: info.Body, HtmlUrl: info.HtmlUrl, PublishedAt: info.PublishedAt, CheckedAt: time.Now()}
	SaveCache(cache)
	return info, nil
}

func fetchRepoInfo(repo string) (string, error) {
	url := "https://api.github.com/repos/" + repo
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var info struct {
		Archived bool `json:"archived"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return "", err
	}
	if info.Archived {
		return "archived", nil
	}
	return "active", nil
}

func fetchRelease(repo string) (ReleaseInfo, error) {
	url := "https://api.github.com/repos/" + repo + "/releases/latest"

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

func extractRepo(githubField string) string {
	s := strings.TrimPrefix(githubField, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "github.com/")
	parts := strings.Split(s, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

func cacheFilePath() (string, error) {
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
