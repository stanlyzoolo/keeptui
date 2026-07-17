package version

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/logx"
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

	// Accumulate reasons only for anomalous failures — a binary that exists but
	// won't answer --version/-V (non-zero exit, timeout, or no parseable
	// version). A plain "not on PATH" (LookPath miss) is the normal
	// "not installed" state the card renders as "installed: not found", not a
	// malfunction, so it is left out: otherwise every tracked-but-uninstalled
	// tool would create a session log on every startup, defeating logx's
	// "a log file means something went wrong" signal. We also do not log inside
	// the loop — a --version miss followed by a -V success must stay silent.
	var reasons []string
	for _, args := range candidates {
		if _, err := exec.LookPath(args[0]); err != nil {
			continue // not installed — benign, never logged
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		// Detached from the controlling terminal: a tool that ignores
		// --version/-V and starts a TUI must not reach keys' screen.
		proc.DetachTTY(cmd)
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			reasons = append(reasons, strings.Join(args, " ")+": "+err.Error())
			continue
		}
		if m := versionRe.FindString(string(out)); m != "" {
			return m
		}
		reasons = append(reasons, strings.Join(args, " ")+": no version string in output")
	}
	// Only an installed-but-unresponsive binary reaches here with reasons; a
	// tool simply absent from PATH leaves reasons empty and logs nothing.
	if len(reasons) > 0 {
		logx.Errorf("version.InstalledVersion: %s: %s", t.Name, strings.Join(reasons, "; "))
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
