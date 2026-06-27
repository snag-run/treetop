package main

import (
	"regexp"
	"testing"
)

func TestParseFlagsPatterns(t *testing.T) {
	// -e flags and positional args both feed the pattern set.
	opts, err := parseFlags([]string{"-e", "snag", "athanor"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if got := len(opts.patterns); got != 2 {
		t.Fatalf("len(patterns) = %d, want 2", got)
	}
}

func TestParseFlagsInvalidPattern(t *testing.T) {
	if _, err := parseFlags([]string{"["}); err == nil {
		t.Fatal("expected error for invalid regexp, got nil")
	}
}

func TestParseFlagsNoRootNoHome(t *testing.T) {
	// An empty HOME makes os.UserHomeDir() fail on Linux; with no --root the
	// scan root is unresolvable and parseFlags must error rather than scan
	// nothing silently.
	t.Setenv("HOME", "")
	if _, err := parseFlags(nil); err == nil {
		t.Fatal("expected error when HOME is unset and no --root given, got nil")
	}
}

func TestParseFlagsRootNoHome(t *testing.T) {
	// An explicit --root resolves the scan root even when HOME is unset.
	t.Setenv("HOME", "")
	if _, err := parseFlags([]string{"--root", "/some/dir"}); err != nil {
		t.Fatalf("parseFlags with --root: %v", err)
	}
}

func TestParseFlagsVersionNoHome(t *testing.T) {
	// -V returns before root resolution, so it succeeds regardless of HOME.
	t.Setenv("HOME", "")
	opts, err := parseFlags([]string{"-V"})
	if err != nil {
		t.Fatalf("parseFlags -V: %v", err)
	}
	if !opts.showVersion {
		t.Fatal("expected showVersion to be true for -V")
	}
}

func TestMatchesName(t *testing.T) {
	mustCompile := func(raw ...string) []*regexp.Regexp {
		p, err := compilePatterns(raw)
		if err != nil {
			t.Fatalf("compilePatterns(%v): %v", raw, err)
		}
		return p
	}

	cases := []struct {
		name     string
		patterns []*regexp.Regexp
		input    string
		want     bool
	}{
		{"no patterns matches all", nil, "anything", true},
		{"substring", mustCompile("nag"), "snag", true},
		{"case-insensitive", mustCompile("SNAG"), "snag", true},
		{"alternation hit", mustCompile("snag|athanor"), "athanor", true},
		{"alternation via two patterns", mustCompile("snag", "athanor"), "athanor", true},
		{"no match", mustCompile("snag", "athanor"), "treetop", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesName(tc.patterns, tc.input); got != tc.want {
				t.Errorf("matchesName(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
