package main

import (
	"bufio"
	"errors"
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

type failingWriter struct{}

var errFailingWriter = errors.New("write failed")

func (failingWriter) Write([]byte) (int, error) {
	return 0, errFailingWriter
}

func TestWriteWatchSetup(t *testing.T) {
	var plain strings.Builder
	if err := writeWatchSetup(bufio.NewWriter(&plain), false); err != nil {
		t.Fatalf("writeWatchSetup without mouse reporting: %v", err)
	}
	if got := plain.String(); got != altScreenOn+cursorHide {
		t.Errorf("writeWatchSetup without mouse reporting = %q, want %q", got, altScreenOn+cursorHide)
	}

	var mouse strings.Builder
	if err := writeWatchSetup(bufio.NewWriter(&mouse), true); err != nil {
		t.Fatalf("writeWatchSetup with mouse reporting: %v", err)
	}
	if got := mouse.String(); got != altScreenOn+cursorHide+mouseOn {
		t.Errorf("writeWatchSetup with mouse reporting = %q, want %q", got, altScreenOn+cursorHide+mouseOn)
	}

	if err := writeWatchSetup(bufio.NewWriter(failingWriter{}), true); !errors.Is(err, errFailingWriter) {
		t.Fatalf("writeWatchSetup failure = %v, want %v", err, errFailingWriter)
	}
}

func TestWatchMouseReporting(t *testing.T) {
	cases := []struct {
		name string
		opts options
		want bool
	}{
		{name: "plain watch", opts: options{}, want: true},
		{name: "PR links", opts: options{pr: true}, want: false},
		{name: "check links", opts: options{checks: true}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := watchMouseReporting(tc.opts); got != tc.want {
				t.Errorf("watchMouseReporting() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWatchFooterFilterMode(t *testing.T) {
	r := newRenderer(nil, false, false, false)

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

	// The check-row toggle hint appears only when expansion is available, and
	// names the next mode in the collapse -> all -> checks cycle.
	noToggle := watchFooter(r, footerState{total: 3, viewport: 10, prExpandable: false})
	if strings.Contains(noToggle, "checks") || strings.Contains(noToggle, "collapse") {
		t.Errorf("toggle hint should be hidden when expansion is unavailable: %q", noToggle)
	}
	collapsed := watchFooter(r, footerState{total: 3, viewport: 10, prExpandable: true, checksExpanded: checksCollapsed})
	if !strings.Contains(collapsed, "c all checks") {
		t.Errorf("collapsed state should advertise expanding to all checks: %q", collapsed)
	}
	all := watchFooter(r, footerState{total: 3, viewport: 10, prExpandable: true, checksExpanded: checksAll})
	if !strings.Contains(all, "c checks") || strings.Contains(all, "c all checks") {
		t.Errorf("all-checks state should advertise the filtered checks view: %q", all)
	}
	ran := watchFooter(r, footerState{total: 3, viewport: 10, prExpandable: true, checksExpanded: checksRan})
	if !strings.Contains(ran, "c collapse") {
		t.Errorf("checks state should advertise collapsing: %q", ran)
	}
}

func TestHeaderLinesStaleness(t *testing.T) {
	r := newRenderer(nil, false, false, false)
	opts := options{interval: 2} // stale threshold is two intervals = 4s

	fresh := headerLines(r, opts, nil, true, time.Second, false)
	if strings.Contains(fresh[0], "stale") {
		t.Errorf("data within one interval should not be flagged stale: %q", fresh[0])
	}

	stale := headerLines(r, opts, nil, true, 5*time.Second, false)
	if !strings.Contains(stale[0], "stale") {
		t.Errorf("data older than two intervals should be flagged stale: %q", stale[0])
	}

	// Boundary: the threshold is >= two intervals (4s here), so 3s is still
	// fresh and exactly 4s flips to stale.
	if h := headerLines(r, opts, nil, true, 3*time.Second, false); strings.Contains(h[0], "stale") {
		t.Errorf("just under the threshold should stay fresh: %q", h[0])
	}
	if h := headerLines(r, opts, nil, true, 4*time.Second, false); !strings.Contains(h[0], "stale") {
		t.Errorf("exactly at the threshold should be stale: %q", h[0])
	}
}

func names(ps []Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}
