package version

import (
	"os"
	"testing"

	"github.com/stanlyzoolo/keeptui/internal/logx"
)

// TestMain redirects logx to a throwaway directory for the whole package test
// binary, so tests that exercise the logging error paths (classifyStatus, doGH,
// LoadCache/SaveCache, InstalledVersion) never write keeptui-*.log into the real
// user config dir. Individual tests that assert logger output still call
// logx.SetDirForTesting with their own temp dir; its restore reverts to this
// fallback (not the real dir), keeping every test off the real config.
//
// It also pins testBrewPrefix to an empty throwaway prefix, so no test ever
// consults the developer machine's real Homebrew tree (a real formula named
// like a test fixture would otherwise leak a version into InstalledVersion
// results). Brew-specific tests install their own layout via makeBrewLayout.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "keeptui-version-logs")
	if err != nil {
		panic(err)
	}
	restore := logx.SetDirForTesting(dir)
	brewDir, err := os.MkdirTemp("", "keeptui-version-brew")
	if err != nil {
		panic(err)
	}
	testBrewPrefix = brewDir
	// Same blanket protection for the two files this package writes: cache.json
	// and the token. Per-test overrides nest inside it and restore back to it.
	cfgDir, err := os.MkdirTemp("", "keeptui-version-config")
	if err != nil {
		panic(err)
	}
	restoreCfg := SetConfigDirForTesting(cfgDir)
	code := m.Run()
	restoreCfg()
	restore()
	_ = os.RemoveAll(dir)
	_ = os.RemoveAll(brewDir)
	_ = os.RemoveAll(cfgDir)
	os.Exit(code)
}

// TestConfigDirIsolated fails if the package-wide isolation above is ever
// removed: without it a test that writes the cache or saves a token rewrites the
// real user config.
func TestConfigDirIsolated(t *testing.T) {
	cacheDir, tokenDir := ConfigDirOverrides()
	if cacheDir == "" || tokenDir == "" {
		t.Fatalf("cache/token dir overrides = %q/%q, want a temp dir — tests can reach the real config", cacheDir, tokenDir)
	}
}
