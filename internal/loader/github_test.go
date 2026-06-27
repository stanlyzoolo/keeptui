package loader

import "testing"

func TestParseToolRef(t *testing.T) {
	tests := []struct {
		name       string
		arg        string
		wantName   string
		wantGitHub string
		wantIsGH   bool
	}{
		{
			name:       "https url",
			arg:        "https://github.com/neovim/neovim",
			wantName:   "neovim",
			wantGitHub: "github.com/neovim/neovim",
			wantIsGH:   true,
		},
		{
			name:       "bare github.com path",
			arg:        "github.com/junegunn/fzf",
			wantName:   "fzf",
			wantGitHub: "github.com/junegunn/fzf",
			wantIsGH:   true,
		},
		{
			name:       "url with .git suffix",
			arg:        "https://github.com/sharkdp/bat.git",
			wantName:   "bat",
			wantGitHub: "github.com/sharkdp/bat",
			wantIsGH:   true,
		},
		{
			name:       "url with extra path tail",
			arg:        "https://github.com/owner/repo/tree/main",
			wantName:   "repo",
			wantGitHub: "github.com/owner/repo",
			wantIsGH:   true,
		},
		{
			name:       "plain name git",
			arg:        "git",
			wantName:   "git",
			wantGitHub: "",
			wantIsGH:   false,
		},
		{
			name:       "plain name tmux",
			arg:        "tmux",
			wantName:   "tmux",
			wantGitHub: "",
			wantIsGH:   false,
		},
		{
			name:       "malformed github-ish input falls back to plain",
			arg:        "https://github.com/onlyowner",
			wantName:   "https://github.com/onlyowner",
			wantGitHub: "",
			wantIsGH:   false,
		},
		{
			name:       "non-github url falls back to plain",
			arg:        "https://gitlab.com/owner/repo",
			wantName:   "https://gitlab.com/owner/repo",
			wantGitHub: "",
			wantIsGH:   false,
		},
		{
			name:       "ssh scp-style ref",
			arg:        "git@github.com:owner/repo.git",
			wantName:   "repo",
			wantGitHub: "github.com/owner/repo",
			wantIsGH:   true,
		},
		{
			name:       "empty repo segment after .git strip falls back to plain",
			arg:        "github.com/owner/.git",
			wantName:   "github.com/owner/.git",
			wantGitHub: "",
			wantIsGH:   false,
		},
		{
			name:       "trailing slash",
			arg:        "https://github.com/owner/repo/",
			wantName:   "repo",
			wantGitHub: "github.com/owner/repo",
			wantIsGH:   true,
		},
		{
			name:       "spoofed host containing github.com falls back to plain",
			arg:        "https://github.com.evil.com/owner/repo",
			wantName:   "https://github.com.evil.com/owner/repo",
			wantGitHub: "",
			wantIsGH:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotGitHub, gotIsGH := ParseToolRef(tt.arg)
			if gotName != tt.wantName {
				t.Errorf("name = %q, want %q", gotName, tt.wantName)
			}
			if gotGitHub != tt.wantGitHub {
				t.Errorf("github = %q, want %q", gotGitHub, tt.wantGitHub)
			}
			if gotIsGH != tt.wantIsGH {
				t.Errorf("isGitHub = %v, want %v", gotIsGH, tt.wantIsGH)
			}
		})
	}
}

func TestNormalizeRepo(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"https://github.com/neovim/neovim", "neovim/neovim"},
		{"github.com/junegunn/fzf", "junegunn/fzf"},
		{"http://github.com/owner/repo/tree/main", "owner/repo"},
		{"owner/repo", "owner/repo"},
		{"onlyowner", ""},
		{"", ""},
		{"github.com/onlyowner", ""},
		{"github.com/sharkdp/bat.git", "sharkdp/bat"},
		{"git@github.com:owner/repo.git", "owner/repo"},
		{"github.com/owner/.git", ""},
		{"https://github.com.evil.com/owner/repo", ""},
		{"mygithub.com/owner/repo", ""},
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			if got := NormalizeRepo(tt.arg); got != tt.want {
				t.Errorf("NormalizeRepo(%q) = %q, want %q", tt.arg, got, tt.want)
			}
		})
	}
}
