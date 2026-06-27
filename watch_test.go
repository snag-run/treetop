package main

import (
	"regexp"
	"strings"
	"testing"
	"time"
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

func TestKeepNameANDsFilters(t *testing.T) {
	mustCompile := func(raw ...string) []*regexp.Regexp {
		p, err := compilePatterns(raw)
		if err != nil {
			t.Fatalf("compilePatterns(%v): %v", raw, err)
		}
		return p
	}

	cases := []struct {
		name      string
		cli, live []*regexp.Regexp
		input     string
		want      bool
	}{
		{"no filters keeps all", nil, nil, "snag", true},
		{"cli only", mustCompile("snag"), nil, "snag", true},
		{"cli only excludes", mustCompile("snag"), nil, "athanor", false},
		{"live narrows within cli", mustCompile("snag|athanor"), mustCompile("nag"), "snag", true},
		{"live excludes a cli match", mustCompile("snag|athanor"), mustCompile("nag"), "athanor", false},
		{"live only", nil, mustCompile("tree"), "treetop", true},
		{"live only excludes", nil, mustCompile("tree"), "snag", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := keepName(tc.cli, tc.live, tc.input); got != tc.want {
				t.Errorf("keepName(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
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

	// The "/ filter" hint shows only when the live filter box is available
	// (i.e. no CLI grep flags pinned the filter at launch).
	withBox := watchFooter(r, footerState{total: 3, viewport: 10, filterable: true})
	if !strings.Contains(withBox, "/ filter") {
		t.Errorf("filterable footer should advertise the filter key: %q", withBox)
	}
	noBox := watchFooter(r, footerState{total: 3, viewport: 10, filterable: false})
	if strings.Contains(noBox, "/ filter") {
		t.Errorf("CLI-filtered footer should hide the filter key: %q", noBox)
	}
}

func TestHeaderLinesStaleness(t *testing.T) {
	r := newRenderer(nil, false, false)
	opts := options{interval: 2} // stale threshold is two intervals = 4s

	fresh := headerLines(r, opts, nil, true, time.Second)
	if strings.Contains(fresh[0], "stale") {
		t.Errorf("data within one interval should not be flagged stale: %q", fresh[0])
	}

	stale := headerLines(r, opts, nil, true, 5*time.Second)
	if !strings.Contains(stale[0], "stale") {
		t.Errorf("data older than two intervals should be flagged stale: %q", stale[0])
	}
}

func names(ps []Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}
