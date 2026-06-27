package loader

import "strings"

// NormalizeRepo turns a GitHub URL or path into a bare "owner/repo" string.
// It strips an optional scheme and "github.com/" prefix, then keeps the first
// two path segments. It returns "" when the input cannot yield owner and repo.
func NormalizeRepo(s string) string {
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "github.com/")
	s = strings.Trim(s, "/")
	parts := strings.Split(s, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// ParseToolRef classifies a `keys track` argument. When arg refers to a GitHub
// repository it returns a short tool name (the repo segment, without a trailing
// ".git") and a normalized "github.com/owner/repo" string. Otherwise it returns
// the argument unchanged as a plain name with isGitHub=false.
func ParseToolRef(arg string) (name, github string, isGitHub bool) {
	looksGitHub := strings.Contains(arg, "github.com") ||
		strings.HasPrefix(arg, "http://") ||
		strings.HasPrefix(arg, "https://")
	if !looksGitHub {
		return arg, "", false
	}

	repo := NormalizeRepo(arg)
	if repo == "" {
		// Looks GitHub-ish but isn't parseable — never lose the input.
		return arg, "", false
	}

	parts := strings.SplitN(repo, "/", 2)
	owner, name := parts[0], strings.TrimSuffix(parts[1], ".git")
	return name, "github.com/" + owner + "/" + name, true
}
