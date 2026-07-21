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
	code := m.Run()
	restore()
	_ = os.RemoveAll(dir)
	_ = os.RemoveAll(brewDir)
	os.Exit(code)
}
