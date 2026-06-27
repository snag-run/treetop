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
		newRenderer(&b, false, compact).render(projects, true)
		if strings.ContainsRune(b.String(), '\x1b') {
			t.Errorf("compact=%v: rendered output contains a raw ESC byte:\n%q", compact, b.String())
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
	newRenderer(&b, false, false).render(projects, true)
	out := b.String()

	if strings.HasPrefix(out, "\n") {
		t.Errorf("first project should not be preceded by a blank line:\n%q", out)
	}
	if !strings.Contains(out, "\n\ntreetop\n") {
		t.Errorf("expected a blank line before the second project:\n%q", out)
	}
}
