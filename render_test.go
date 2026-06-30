package main

import (
	"strings"
	"testing"
	"time"
)

func TestSanitizeDisplay(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "feature/login", "feature/login"},
		{"space kept", "my repo", "my repo"},
		{"unicode kept", "café-α", "café-α"},
		{"esc replaced", "repo\x1b[31mX\x1b[0m", "repo?[31mX?[0m"},
		{"c0 controls", "a\x00\a\b\tb", "a????b"},
		{"newline", "a\nb", "a?b"},
		{"del", "a\x7fb", "a?b"},
	}
	for _, tt := range tests {
		if got := sanitizeDisplay(tt.in); got != tt.want {
			t.Errorf("%s: sanitizeDisplay(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

// TestRenderStripsControlBytes asserts no ESC byte from an attacker-controlled
// project/branch/path name survives into the rendered output.
func TestRenderStripsControlBytes(t *testing.T) {
	now := time.Now()
	projects := []Project{{
		Name: "proj\x1b[31mINJECT",
		Worktrees: []Worktree{{
			Path:    "/tmp/wt\x1b]0;title\a",
			Branch:  "main\x1b[2J",
			Changed: now, HasTime: true,
			Edited: now, HasEdit: true,
		}},
	}}

	for _, compact := range []bool{false, true} {
		var b strings.Builder
		newRenderer(&b, false, compact, false).render(projects, true)
		if strings.ContainsRune(b.String(), '\x1b') {
			t.Errorf("compact=%v: rendered output contains a raw ESC byte:\n%q", compact, b.String())
		}
	}
}

// TestRenderPRGlyphColumn asserts the --pr column renders the right coloured
// glyph per state, a blank cell for a PR-less worktree, and nothing at all when
// the column is off.
func TestRenderPRGlyphColumn(t *testing.T) {
	now := time.Now()
	mk := func(branch string, hasPR bool, s CheckState) Worktree {
		return Worktree{Path: "/" + branch, Branch: branch, HasPR: hasPR, Check: s,
			Changed: now, HasTime: true, Edited: now, HasEdit: true}
	}
	projects := []Project{{Name: "snag", Worktrees: []Worktree{
		mk("pass", true, StateSuccess),
		mk("fail", true, StateFailure),
		mk("run", true, StatePending),
		mk("none", true, StateNeutral),
		mk("nopr", false, StateNeutral),
	}}}

	// Column off: no glyphs, regardless of the worktrees' PR data.
	var off strings.Builder
	newRenderer(&off, false, false, false).render(projects, true)
	for _, g := range []string{"✓", "✗", "○"} {
		if strings.Contains(off.String(), g) {
			t.Errorf("PR column off should not render glyph %q:\n%s", g, off.String())
		}
	}

	// Column on, with colour: each state maps to its coloured glyph.
	var on strings.Builder
	newRenderer(&on, true, false, true).render(projects, true)
	out := on.String()
	for _, want := range []struct {
		glyph, color string
	}{
		{"✓", colGreen},
		{"✗", colRed},
		{"●", colYellow}, // pending; the in-use marker also uses ● but is green
		{"○", colDim},
	} {
		if !strings.Contains(out, want.color+want.glyph+colReset) {
			t.Errorf("expected coloured glyph %q (%q):\n%s", want.glyph, want.color, out)
		}
	}

	// Compact (--projects) view: the project rolls up worst-wins, so the mix above
	// (which includes a failure) must render the red ✗ on the single project line.
	var comp strings.Builder
	newRenderer(&comp, true, true, true).render(projects, true)
	if cs := comp.String(); !strings.Contains(cs, colRed+"✗"+colReset) {
		t.Errorf("compact view should roll up to the worst (✗) glyph:\n%s", cs)
	}

	// Compact with no PRs anywhere: a blank cell, no glyph.
	noPR := []Project{{Name: "x", Worktrees: []Worktree{mk("a", false, StateNeutral)}}}
	var compNone strings.Builder
	newRenderer(&compNone, false, true, true).render(noPR, true)
	for _, g := range []string{"✓", "✗", "○", "●"} {
		if strings.Contains(compNone.String(), g) {
			t.Errorf("compact view with no PRs should render no glyph %q:\n%s", g, compNone.String())
		}
	}
}

// TestRenderPRNumber asserts the open PR's number renders in its own column when
// --pr is on, is absent for a PR-less worktree, and never shows when the column
// is off — and that the layout stays aligned across both.
func TestRenderPRNumber(t *testing.T) {
	now := time.Now()
	mk := func(branch string, hasPR bool, n int) Worktree {
		return Worktree{Path: "/" + branch, Branch: branch, HasPR: hasPR, PRNumber: n,
			Check: StateSuccess, Changed: now, HasTime: true, Edited: now, HasEdit: true}
	}
	projects := []Project{{Name: "snag", Worktrees: []Worktree{
		mk("withpr", true, 56),
		mk("nopr", false, 0),
	}}}

	// --pr on: the PR number renders in its own column; the PR-less worktree gets
	// a blank cell. This worktree has no review decision (ReviewNone), so the cell
	// is plain — colouring by review state is covered by TestRenderPRNumberColor.
	var on strings.Builder
	newRenderer(&on, true, false, true).render(projects, true)
	out := on.String()
	if !strings.Contains(out, "#56") {
		t.Errorf("expected PR-number cell #56:\n%s", out)
	}
	if strings.Count(out, "#") != 1 {
		t.Errorf("only the worktree with a PR should show a number:\n%s", out)
	}

	// --pr off: no number even though the data carries one.
	var off strings.Builder
	newRenderer(&off, false, false, false).render(projects, true)
	if strings.Contains(off.String(), "#56") {
		t.Errorf("PR number should not render with --pr off:\n%s", off.String())
	}
}

// TestRenderPRNumberColor asserts the PR-number suffix is coloured by review
// state — green approved, red changes-requested, yellow awaiting review, dim
// draft — while an open PR with no decision stays plain.
func TestRenderPRNumberColor(t *testing.T) {
	now := time.Now()
	mk := func(branch string, n int, rv PRReview) Worktree {
		return Worktree{Path: "/" + branch, Branch: branch, HasPR: true, PRNumber: n,
			PRReview: rv, Check: StateSuccess, Changed: now, HasTime: true, Edited: now, HasEdit: true}
	}
	projects := []Project{{Name: "snag", Worktrees: []Worktree{
		mk("approved", 1, ReviewApproved),
		mk("changes", 2, ReviewChangesRequested),
		mk("pending", 3, ReviewRequired),
		mk("draft", 4, ReviewDraft),
		mk("open", 5, ReviewNone),
	}}}

	var b strings.Builder
	newRenderer(&b, true, false, true).render(projects, true)
	out := b.String()

	for _, want := range []struct {
		num, color string
	}{
		{"#1", colGreen},
		{"#2", colRed},
		{"#3", colYellow},
		{"#4", colDim},
	} {
		if !strings.Contains(out, want.color+want.num+colReset) {
			t.Errorf("expected %q coloured %q:\n%s", want.num, want.color, out)
		}
	}
	// The no-decision PR renders plain: present, but not wrapped in any colour.
	for _, c := range []string{colGreen, colRed, colYellow, colDim} {
		if strings.Contains(out, c+"#5"+colReset) {
			t.Errorf("open/no-decision PR #5 should be plain, found colour %q:\n%s", c, out)
		}
	}
	if !strings.Contains(out, "#5") {
		t.Errorf("expected plain PR-number cell #5:\n%s", out)
	}
}

// TestRenderCheckRows asserts that --checks expands one coloured row per CI check
// beneath a worktree, that the rows are off without the flag, and that a worktree
// with no PR contributes no rows.
func TestRenderCheckRows(t *testing.T) {
	now := time.Now()
	withPR := Worktree{
		Path: "/wt", Branch: "feat", HasPR: true, Check: StateFailure,
		Checks: []Check{
			{Name: "lint", State: StateFailure},
			{Name: "build", State: StateSuccess},
		},
		Changed: now, HasTime: true, Edited: now, HasEdit: true,
	}
	noPR := Worktree{
		Path: "/clean", Branch: "main",
		Changed: now, HasTime: true, Edited: now, HasEdit: true,
	}
	projects := []Project{{Name: "snag", Worktrees: []Worktree{withPR, noPR}}}

	// Flag off: the rollup glyph may show, but no per-check names appear.
	var off strings.Builder
	r := newRenderer(&off, false, false, true)
	r.render(projects, true)
	for _, name := range []string{"lint", "build"} {
		if strings.Contains(off.String(), name) {
			t.Errorf("without --checks, check name %q should not render:\n%s", name, off.String())
		}
	}

	// Flag on (with colour): each check renders as a coloured glyph + its name.
	var on strings.Builder
	r = newRenderer(&on, true, false, true)
	r.checks = checksAll
	r.render(projects, true)
	out := on.String()
	for _, want := range []struct{ color, glyph, name string }{
		{colRed, "✗", "lint"},
		{colGreen, "✓", "build"},
	} {
		if !strings.Contains(out, want.color+want.glyph+colReset+" "+want.name) {
			t.Errorf("expected check row %q %q:\n%s", want.glyph, want.name, out)
		}
	}

	// Failures sort ahead of successes: lint's row precedes build's.
	if strings.Index(out, "lint") > strings.Index(out, "build") {
		t.Errorf("failing check should render before passing one:\n%s", out)
	}
}

// TestRenderCheckRowsSkippedFilter asserts the three-way check view: checksAll
// shows every per-check row (including skipped), checksRan drops skipped rows,
// and checksCollapsed shows none.
func TestRenderCheckRowsSkippedFilter(t *testing.T) {
	now := time.Now()
	wt := Worktree{
		Path: "/wt", Branch: "feat", HasPR: true, Check: StateFailure,
		Checks: []Check{
			{Name: "lint", State: StateFailure},
			{Name: "noop", State: StateNeutral, Skipped: true},
			{Name: "build", State: StateSuccess},
		},
		Changed: now, HasTime: true, Edited: now, HasEdit: true,
	}
	projects := []Project{{Name: "snag", Worktrees: []Worktree{wt}}}

	out := func(v checkView) string {
		var b strings.Builder
		r := newRenderer(&b, false, false, true)
		r.checks = v
		r.render(projects, true)
		return b.String()
	}

	all := out(checksAll)
	for _, name := range []string{"lint", "noop", "build"} {
		if !strings.Contains(all, name) {
			t.Errorf("checksAll should show %q:\n%s", name, all)
		}
	}

	ran := out(checksRan)
	if strings.Contains(ran, "noop") {
		t.Errorf("checksRan should hide the skipped check:\n%s", ran)
	}
	for _, name := range []string{"lint", "build"} {
		if !strings.Contains(ran, name) {
			t.Errorf("checksRan should still show ran check %q:\n%s", name, ran)
		}
	}

	collapsed := out(checksCollapsed)
	for _, name := range []string{"lint", "noop", "build"} {
		if strings.Contains(collapsed, name) {
			t.Errorf("checksCollapsed should show no per-check rows, found %q:\n%s", name, collapsed)
		}
	}
}

// TestCheckViewCycle asserts the 'c' key cycle order and the next-state footer
// labels: collapsed -> all checks -> checks -> collapse.
func TestCheckViewCycle(t *testing.T) {
	cases := []struct {
		from      checkView
		wantNext  checkView
		wantLabel string
	}{
		{checksCollapsed, checksAll, "all checks"},
		{checksAll, checksRan, "checks"},
		{checksRan, checksCollapsed, "collapse"},
	}
	for _, c := range cases {
		if got := c.from.next(); got != c.wantNext {
			t.Errorf("%d.next() = %d, want %d", c.from, got, c.wantNext)
		}
		if got := c.wantNext.label(); got != c.wantLabel {
			t.Errorf("%d.label() = %q, want %q", c.wantNext, got, c.wantLabel)
		}
	}
}

// TestRenderCheckRowsGatedByProjectCount asserts that --checks expansion is
// suppressed once more projects are shown than the poll cap — beyond it the CI
// data is only partially populated, so a half-expanded wall of rows is worse than
// the rollup glyph alone.
func TestRenderCheckRowsGatedByProjectCount(t *testing.T) {
	now := time.Now()
	mkProject := func(name string) Project {
		return Project{Name: name, Worktrees: []Worktree{{
			Path: "/" + name, Branch: "feat", HasPR: true, Check: StateFailure,
			Checks:  []Check{{Name: "lint-" + name, State: StateFailure}},
			Changed: now, HasTime: true, Edited: now, HasEdit: true,
		}}}
	}

	// Within the cap: the per-check row renders.
	var within strings.Builder
	r := newRenderer(&within, false, false, true)
	r.checks = checksAll
	r.render([]Project{mkProject("a")}, true)
	if !strings.Contains(within.String(), "lint-a") {
		t.Errorf("within the cap, check rows should render:\n%s", within.String())
	}

	// Over the cap: no check rows, even with --checks on.
	var over strings.Builder
	overProjects := make([]Project, 0, maxPRPollProjects+1)
	for i := 0; i <= maxPRPollProjects; i++ {
		overProjects = append(overProjects, mkProject(string(rune('a'+i))))
	}
	r = newRenderer(&over, false, false, true)
	r.checks = checksAll
	r.render(overProjects, true)
	if strings.Contains(over.String(), "lint-") {
		t.Errorf("over the cap (%d projects), check rows should be suppressed:\n%s",
			len(overProjects), over.String())
	}
}

// TestRenderBlankLineBetweenProjects asserts adjacent project groups are
// separated by a blank line, but the first project gets no leading blank.
func TestRenderBlankLineBetweenProjects(t *testing.T) {
	now := time.Now()
	wt := []Worktree{{Path: "/p", Changed: now, HasTime: true, Edited: now, HasEdit: true}}
	projects := []Project{
		{Name: "snag", Worktrees: wt},
		{Name: "treetop", Worktrees: wt},
	}

	var b strings.Builder
	newRenderer(&b, false, false, false).render(projects, true)
	out := b.String()

	if strings.HasPrefix(out, "\n") {
		t.Errorf("first project should not be preceded by a blank line:\n%q", out)
	}
	if !strings.Contains(out, "\n\ntreetop\n") {
		t.Errorf("expected a blank line before the second project:\n%q", out)
	}
}

// osc8 builds the OSC 8 hyperlink opener for url, the substring a linked cell
// must contain: ESC ]8;; <url> ST. Tests assert on this rather than the full
// open+close pair so they stay readable.
func osc8(url string) string { return "\033]8;;" + url + "\033\\" }

// TestSafeURL asserts only control-free http(s) URLs are allowed into an OSC 8
// escape; anything else (other schemes, an embedded ESC or string-terminator)
// is rejected so a hostile URL can't break out of the sequence.
func TestSafeURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
		want bool
	}{
		{"https", "https://github.com/o/r/pull/1", true},
		{"http", "http://example.com", true},
		{"empty", "", false},
		{"no scheme", "github.com/o/r/pull/1", false},
		{"other scheme", "file:///etc/passwd", false},
		{"javascript", "javascript:alert(1)", false},
		{"embedded esc", "https://x/\033]8;;evil\033\\", false},
		{"embedded newline", "https://x/\n", false},
	} {
		if got := safeURL(tc.url); got != tc.want {
			t.Errorf("safeURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

// TestHyperlink asserts hyperlink wraps text in a full OSC 8 open/close pair when
// colour is on and the URL is safe, and otherwise returns text untouched — for
// colour off (non-interactive output) or an unsafe URL.
func TestHyperlink(t *testing.T) {
	url := "https://github.com/o/r/pull/7"
	on := newRenderer(nil, true, false, true)
	got := on.hyperlink(url, "#7")
	want := osc8(url) + "#7" + "\033]8;;\033\\"
	if got != want {
		t.Errorf("hyperlink with colour on = %q, want %q", got, want)
	}

	off := newRenderer(nil, false, false, true)
	if got := off.hyperlink(url, "#7"); got != "#7" {
		t.Errorf("hyperlink with colour off should be plain, got %q", got)
	}
	if got := on.hyperlink("ftp://nope", "#7"); got != "#7" {
		t.Errorf("hyperlink with unsafe URL should be plain, got %q", got)
	}
}

// TestRenderHyperlinks asserts the PR number, the rollup glyph, and each --checks
// row are emitted as OSC 8 links to their respective URLs, and that none of those
// escapes appear when colour is off (so piped output stays clean).
func TestRenderHyperlinks(t *testing.T) {
	now := time.Now()
	prURL := "https://github.com/snag-run/treetop/pull/42"
	runURL := "https://github.com/snag-run/treetop/actions/runs/123"
	wt := Worktree{
		Path: "/wt", Branch: "feat", HasPR: true, PRNumber: 42, PRURL: prURL,
		Check: StateFailure,
		Checks: []Check{
			{Name: "lint", State: StateFailure, URL: runURL},
			{Name: "build", State: StateSuccess}, // no URL: row renders unlinked
		},
		Changed: now, HasTime: true, Edited: now, HasEdit: true,
	}
	projects := []Project{{Name: "snag", Worktrees: []Worktree{wt}}}

	var on strings.Builder
	r := newRenderer(&on, true, false, true)
	r.checks = checksAll
	r.render(projects, true)
	out := on.String()

	if !strings.Contains(out, osc8(prURL)) {
		t.Errorf("expected an OSC 8 link to the PR URL:\n%q", out)
	}
	// The PR URL links both the "#42" cell and the rollup glyph, so it opens twice.
	if n := strings.Count(out, osc8(prURL)); n != 2 {
		t.Errorf("expected PR URL linked twice (number + glyph), got %d:\n%q", n, out)
	}
	if !strings.Contains(out, osc8(runURL)+"") {
		t.Errorf("expected an OSC 8 link to the check run URL:\n%q", out)
	}
	// A check with no URL still renders its name, just without a link wrapper.
	if !strings.Contains(out, "build") {
		t.Errorf("expected the unlinked check to still render:\n%q", out)
	}

	// Colour off: no OSC 8 escapes at all.
	var off strings.Builder
	ro := newRenderer(&off, false, false, true)
	ro.checks = checksAll
	ro.render(projects, true)
	if strings.Contains(off.String(), "\033]8;;") {
		t.Errorf("no OSC 8 escapes should appear with colour off:\n%q", off.String())
	}
}
