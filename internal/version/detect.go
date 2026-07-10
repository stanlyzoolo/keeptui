package version

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/proc"
)

var versionRe = regexp.MustCompile(`v?(\d+\.\d+[\d.]*)`)

func InstalledVersion(t loader.Tool) string {
	var candidates [][]string

	if t.VersionCmd != "" {
		parts := strings.Fields(t.VersionCmd)
		candidates = [][]string{parts}
	} else {
		candidates = [][]string{
			{t.Name, "--version"},
			{t.Name, "-V"},
		}
	}

	for _, args := range candidates {
		if _, err := exec.LookPath(args[0]); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		// Detached from the controlling terminal: a tool that ignores
		// --version/-V and starts a TUI must not reach keys' screen.
		proc.DetachTTY(cmd)
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			continue
		}
		if m := versionRe.FindString(string(out)); m != "" {
			return m
		}
	}
	return ""
}

// IsNewer reports whether latest is a newer version than installed.
// Comparison is semver (pre-releases order below their release); either side
// failing to canonicalize — after CalVer/4-segment normalization — means "not
// newer", matching the old empty-string behavior.
func IsNewer(installed, latest string) bool {
	a := canonSemver(installed)
	b := canonSemver(latest)
	if a == "" || b == "" {
		return false
	}
	return semver.Compare(b, a) > 0
}

// canonSemver normalizes a detected version or GitHub tag into a form
// semver.Compare accepts, or "" when it cannot. Beyond the "v" prefix it
// handles two shapes strict semver rejects but real tools emit: zero-padded
// numeric segments (CalVer, "2024.01.15") and a 4th segment ("1.2.3.4",
// truncated — the first three decide newer-ness). Build metadata is dropped;
// a pre-release suffix is kept so rc/beta order below the release.
func canonSemver(v string) string {
	v = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(v), "v"), "V")
	v, _, _ = strings.Cut(v, "+")
	if v == "" {
		return ""
	}
	base, pre, hasPre := strings.Cut(v, "-")

	segs := strings.Split(base, ".")
	if len(segs) > 3 {
		segs = segs[:3]
	}
	for i, s := range segs {
		s = strings.TrimLeft(s, "0")
		if s == "" {
			s = "0"
		}
		segs[i] = s
	}

	out := "v" + strings.Join(segs, ".")
	if hasPre {
		out += "-" + pre
	}
	if !semver.IsValid(out) {
		return ""
	}
	return out
}
