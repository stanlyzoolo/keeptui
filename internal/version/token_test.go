package version

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// resetTokenState clears the in-memory token and the sync.Once so each test
// starts from a clean slate and re-reads the (test-overridden) token file.
func resetTokenState(t *testing.T, dir string) {
	t.Helper()
	origDir := testTokenDir
	testTokenDir = dir
	tokenMu.Lock()
	tokenMem = ""
	tokenMu.Unlock()
	loadTokenOnce = sync.Once{}
	t.Cleanup(func() {
		testTokenDir = origDir
		tokenMu.Lock()
		tokenMem = ""
		tokenMu.Unlock()
		loadTokenOnce = sync.Once{}
	})
}

func TestResolveTokenEnvOverConfig(t *testing.T) {
	dir := t.TempDir()
	resetTokenState(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("config-token"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_TOKEN", "env-token")

	if got := resolveToken(); got != "env-token" {
		t.Errorf("resolveToken() = %q, want env-token", got)
	}
	if got := TokenSource(); got != "env" {
		t.Errorf("TokenSource() = %q, want env", got)
	}
}

func TestResolveTokenFromConfig(t *testing.T) {
	dir := t.TempDir()
	resetTokenState(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("  config-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_TOKEN", "")

	if got := resolveToken(); got != "config-token" {
		t.Errorf("resolveToken() = %q, want config-token (trimmed)", got)
	}
	if got := TokenSource(); got != "config" {
		t.Errorf("TokenSource() = %q, want config", got)
	}
}

func TestResolveTokenEmpty(t *testing.T) {
	dir := t.TempDir()
	resetTokenState(t, dir)
	t.Setenv("GITHUB_TOKEN", "")

	if got := resolveToken(); got != "" {
		t.Errorf("resolveToken() = %q, want empty", got)
	}
	if got := TokenSource(); got != "none" {
		t.Errorf("TokenSource() = %q, want none", got)
	}
}

func TestSetTokenWritesFile0600(t *testing.T) {
	dir := t.TempDir()
	resetTokenState(t, dir)
	t.Setenv("GITHUB_TOKEN", "")

	if err := SetToken("my-token"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "token")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("token file mode = %v, want 0600", perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "my-token" {
		t.Errorf("token file content = %q, want my-token", string(data))
	}
	if got := resolveToken(); got != "my-token" {
		t.Errorf("resolveToken() after SetToken = %q, want my-token", got)
	}
	if got := TokenSource(); got != "config" {
		t.Errorf("TokenSource() after SetToken = %q, want config", got)
	}
}

func TestSetTokenCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	resetTokenState(t, dir)
	t.Setenv("GITHUB_TOKEN", "")

	if err := SetToken("tok"); err != nil {
		t.Fatalf("SetToken should MkdirAll: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "token")); err != nil {
		t.Errorf("token file not created: %v", err)
	}
}

func TestClearToken(t *testing.T) {
	dir := t.TempDir()
	resetTokenState(t, dir)
	t.Setenv("GITHUB_TOKEN", "")

	if err := SetToken("tok"); err != nil {
		t.Fatal(err)
	}
	if err := ClearToken(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "token")); !os.IsNotExist(err) {
		t.Errorf("token file still present after ClearToken: %v", err)
	}
	if got := resolveToken(); got != "" {
		t.Errorf("resolveToken() after ClearToken = %q, want empty", got)
	}
	if got := TokenSource(); got != "none" {
		t.Errorf("TokenSource() after ClearToken = %q, want none", got)
	}
}

func TestClearTokenNoFile(t *testing.T) {
	dir := t.TempDir()
	resetTokenState(t, dir)
	t.Setenv("GITHUB_TOKEN", "")

	if err := ClearToken(); err != nil {
		t.Errorf("ClearToken on missing file should be nil, got %v", err)
	}
}
