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
