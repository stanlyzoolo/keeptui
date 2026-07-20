package main

import (
	"strings"
	"testing"
)

func TestHandleCLI(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantDone bool
		wantOut  string // substring expected on stdout
		wantErr  string // substring expected on stderr
	}{
		{name: "no args launches TUI", args: nil, wantCode: 0, wantDone: false},
		{name: "--version", args: []string{"--version"}, wantCode: 0, wantDone: true, wantOut: "keeptui "},
		{name: "-V", args: []string{"-V"}, wantCode: 0, wantDone: true, wantOut: "keeptui "},
		{name: "-v", args: []string{"-v"}, wantCode: 0, wantDone: true, wantOut: "keeptui "},
		{name: "version word", args: []string{"version"}, wantCode: 0, wantDone: true, wantOut: "keeptui "},
		{name: "--help", args: []string{"--help"}, wantCode: 0, wantDone: true, wantOut: "Usage:"},
		{name: "-h", args: []string{"-h"}, wantCode: 0, wantDone: true, wantOut: "Usage:"},
		{name: "help word", args: []string{"help"}, wantCode: 0, wantDone: true, wantOut: "Usage:"},
		{name: "unknown flag", args: []string{"--bogus"}, wantCode: 2, wantDone: true, wantErr: `unknown argument "--bogus"`},
		{name: "unknown word", args: []string{"frobnicate"}, wantCode: 2, wantDone: true, wantErr: `unknown argument "frobnicate"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut strings.Builder
			code, done := handleCLI(tt.args, &out, &errOut)
			if code != tt.wantCode || done != tt.wantDone {
				t.Fatalf("handleCLI(%v) = (%d, %v), want (%d, %v)", tt.args, code, done, tt.wantCode, tt.wantDone)
			}
			if tt.wantOut != "" && !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want substring %q", out.String(), tt.wantOut)
			}
			if tt.wantOut == "" && out.String() != "" {
				t.Errorf("stdout = %q, want empty", out.String())
			}
			if tt.wantErr != "" && !strings.Contains(errOut.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want substring %q", errOut.String(), tt.wantErr)
			}
		})
	}
}

// The --version line must be parseable by the tool's own installed-version
// detection (version.versionRe wants a dotted numeric), so a release build of
// keeptui tracked inside keeptui shows up as installed.
func TestVersionOutputParseable(t *testing.T) {
	var out strings.Builder
	oldVersion := version
	version = "v0.5.1"
	defer func() { version = oldVersion }()

	if code, done := handleCLI([]string{"--version"}, &out, &strings.Builder{}); code != 0 || !done {
		t.Fatalf("handleCLI --version = (%d, %v), want (0, true)", code, done)
	}
	if got, want := out.String(), "keeptui v0.5.1\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		ldflag, mod, want string
	}{
		{"v1.2.3", "v0.0.9", "v1.2.3"}, // release ldflag always wins
		{"dev", "v0.5.0", "v0.5.0"},    // go install stamps the module version
		{"dev", "(devel)", "dev"},      // plain go build from a checkout
		{"dev", "", "dev"},
		{"", "", "dev"},
	}
	for _, tt := range tests {
		if got := resolveVersion(tt.ldflag, tt.mod); got != tt.want {
			t.Errorf("resolveVersion(%q, %q) = %q, want %q", tt.ldflag, tt.mod, got, tt.want)
		}
	}
}
