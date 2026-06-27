package main

import (
	"strings"
	"testing"
)

func projectsNamed(names ...string) []Project {
	ps := make([]Project, len(names))
	for i, n := range names {
		ps[i] = Project{Name: n, Worktrees: []Worktree{{Path: "/" + n}}}
	}
	return ps
}

func TestFilterByNameNarrows(t *testing.T) {
	projects := projectsNamed("snag", "athanor", "treetop")

	pats, err := compilePatterns([]string{"sn"})
	if err != nil {
		t.Fatalf("compilePatterns: %v", err)
	}
	got := filterByName(projects, pats)
	if len(got) != 1 || got[0].Name != "snag" {
		t.Fatalf("filterByName = %v, want [snag]", names(got))
	}

	// Alternation works just like the CLI pattern arg.
	pats, _ = compilePatterns([]string{"snag|treetop"})
	if got := filterByName(projects, pats); len(got) != 2 {
		t.Fatalf("alternation = %v, want 2 projects", names(got))
	}
}

func TestFilterByNameEmptyKeepsAll(t *testing.T) {
	projects := projectsNamed("snag", "athanor")
	// An empty query compiles to zero patterns -> everything is kept.
	pats, err := compilePatterns([]string{""})
	if err != nil {
		t.Fatalf("compilePatterns: %v", err)
	}
	if got := filterByName(projects, pats); len(got) != 2 {
		t.Fatalf("empty query = %v, want all", names(got))
	}
}

func TestInvalidQueryDoesNotPanic(t *testing.T) {
	// A partial/invalid regex (lone "(") must surface as an error, never panic,
	// so the live filter can fall back to the unfiltered set while typing.
	if _, err := compilePatterns([]string{"("}); err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
}

func TestTrimLastRune(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"a", ""},
		{"snag", "sna"},
		{"é", ""},   // multi-byte rune removed whole
		{"a世", "a"}, // trims the trailing multi-byte rune, keeps the ASCII
	}
	for _, tc := range cases {
		if got := trimLastRune(tc.in); got != tc.want {
			t.Errorf("trimLastRune(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWatchFooterFilterMode(t *testing.T) {
	r := newRenderer(nil, false, false)

	editing := watchFooter(r, footerState{filtering: true, query: "sn", validQuery: true})
	if !strings.Contains(editing, "/sn") {
		t.Errorf("editing footer missing query: %q", editing)
	}

	invalid := watchFooter(r, footerState{filtering: true, query: "(", validQuery: false})
	if !strings.Contains(invalid, "invalid") {
		t.Errorf("invalid-regex footer should warn: %q", invalid)
	}

	applied := watchFooter(r, footerState{total: 3, viewport: 10, query: "sn", validQuery: true})
	if !strings.Contains(applied, "[/sn]") {
		t.Errorf("applied filter should show in scroll footer: %q", applied)
	}

	noMatch := watchFooter(r, footerState{total: 0, query: "zzz", validQuery: true})
	if !strings.Contains(noMatch, "no matches") {
		t.Errorf("zero results with a query should say no matches: %q", noMatch)
	}
}

func names(ps []Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}
