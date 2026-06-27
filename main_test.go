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
