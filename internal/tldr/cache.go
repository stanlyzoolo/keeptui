package tldr

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const cacheTTL = 7 * 24 * time.Hour

var platforms = []string{"common", "osx", "linux"}

func cacheDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "keys", "tldr-cache")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// Fetch returns the tldr Markdown for tool, using a local cache (TTL 7 days).
func Fetch(tool string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	cachePath := filepath.Join(dir, tool+".md")

	if info, err := os.Stat(cachePath); err == nil {
		if time.Since(info.ModTime()) < cacheTTL {
			data, err := os.ReadFile(cachePath)
			if err == nil {
				return string(data), nil
			}
		}
	}

	content, err := fetchRemote(tool)
	if err != nil {
		return "", err
	}

	_ = os.WriteFile(cachePath, []byte(content), 0644)
	return content, nil
}

func fetchRemote(tool string) (string, error) {
	for _, platform := range platforms {
		url := fmt.Sprintf(
			"https://raw.githubusercontent.com/tldr-pages/tldr/main/pages/%s/%s.md",
			platform, tool,
		)
		resp, err := http.Get(url) //nolint:gosec,noctx
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			continue
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}
		return string(data), nil
	}
	return "", fmt.Errorf("tldr page not found for %q (tried: common, osx, linux)", tool)
}
