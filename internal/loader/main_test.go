package loader

import (
	"os"
	"testing"

	"github.com/lepeshko/keys/internal/logx"
)

// TestMain redirects logx to a throwaway directory for the whole package test
// binary, so tests that exercise SaveMeta's error paths never write
// keeptui-*.log into the real user config dir. Individual tests that assert
// logger output still call logx.SetDirForTesting with their own temp dir; its
// restore reverts to this fallback (not the real dir).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "keeptui-loader-logs")
	if err != nil {
		panic(err)
	}
	restore := logx.SetDirForTesting(dir)
	code := m.Run()
	restore()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
