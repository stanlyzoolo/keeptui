package version

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/lepeshko/keys/internal/loader"
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
		out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
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

// IsNewer reports whether b is a newer version than a.
func IsNewer(installed, latest string) bool {
	if installed == "" || latest == "" {
		return false
	}
	ma, na, pa := parseVersion(installed)
	mb, nb, pb := parseVersion(latest)
	if mb != ma {
		return mb > ma
	}
	if nb != na {
		return nb > na
	}
	return pb > pa
}

func parseVersion(v string) (int, int, int) {
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	parts := strings.Split(v, ".")
	nums := [3]int{}
	for i := 0; i < 3 && i < len(parts); i++ {
		n := 0
		for _, c := range parts[i] {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2]
}
