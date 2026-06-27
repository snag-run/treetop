package main

import (
	"regexp"
	"testing"
)

func TestCheckStateOf(t *testing.T) {
	cases := []struct {
		name string
		e    ghRollupEntry
		want CheckState
	}{
		{"actions in progress", ghRollupEntry{Typename: "CheckRun", Status: "IN_PROGRESS"}, StatePending},
		{"actions queued", ghRollupEntry{Typename: "CheckRun", Status: "QUEUED"}, StatePending},
		{"actions success", ghRollupEntry{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"}, StateSuccess},
		{"actions failure", ghRollupEntry{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "FAILURE"}, StateFailure},
		{"actions timed out", ghRollupEntry{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "TIMED_OUT"}, StateFailure},
		{"actions action required", ghRollupEntry{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "ACTION_REQUIRED"}, StateFailure},
		{"actions skipped", ghRollupEntry{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SKIPPED"}, StateNeutral},
		{"actions neutral", ghRollupEntry{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "NEUTRAL"}, StateNeutral},
		{"status context success", ghRollupEntry{Typename: "StatusContext", State: "SUCCESS"}, StateSuccess},
		{"status context pending", ghRollupEntry{Typename: "StatusContext", State: "PENDING"}, StatePending},
		{"status context expected", ghRollupEntry{Typename: "StatusContext", State: "EXPECTED"}, StatePending},
		{"status context error", ghRollupEntry{Typename: "StatusContext", State: "ERROR"}, StateFailure},
		{"status context failure", ghRollupEntry{Typename: "StatusContext", State: "FAILURE"}, StateFailure},
		{"unknown", ghRollupEntry{Typename: "StatusContext", State: "WAT"}, StateNeutral},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := checkStateOf(c.e); got != c.want {
				t.Errorf("checkStateOf(%+v) = %v, want %v", c.e, got, c.want)
			}
		})
	}
}

func TestRollupCheckState(t *testing.T) {
	// Worst-wins across a mix of CheckRun and StatusContext entries.
	mixed := []ghRollupEntry{
		{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Typename: "StatusContext", State: "FAILURE"},
		{Typename: "CheckRun", Status: "IN_PROGRESS"},
	}
	if got := rollupCheckState(mixed); got != StateFailure {
		t.Errorf("one failure among many should be failure, got %v", got)
	}

	pendingWins := []ghRollupEntry{
		{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Typename: "CheckRun", Status: "QUEUED"},
	}
	if got := rollupCheckState(pendingWins); got != StatePending {
		t.Errorf("a queued check should outrank successes, got %v", got)
	}

	allGreen := []ghRollupEntry{
		{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Typename: "StatusContext", State: "SUCCESS"},
	}
	if got := rollupCheckState(allGreen); got != StateSuccess {
		t.Errorf("all-success should be success, got %v", got)
	}

	// The load-bearing edge case: an empty rollup is neutral, never success.
	if got := rollupCheckState(nil); got != StateNeutral {
		t.Errorf("empty rollup must be neutral (not success), got %v", got)
	}

	onlySkipped := []ghRollupEntry{
		{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SKIPPED"},
	}
	if got := rollupCheckState(onlySkipped); got != StateNeutral {
		t.Errorf("only-skipped should be neutral, got %v", got)
	}
}

func TestShouldPollPR(t *testing.T) {
	pat := []*regexp.Regexp{regexp.MustCompile("x")}
	cases := []struct {
		name string
		opts options
		live []*regexp.Regexp
		want bool
	}{
		{"flag off", options{}, nil, false},
		{"flag off even with filter", options{patterns: pat}, nil, false},
		{"flag on but no filter", options{pr: true}, nil, false},
		{"flag on with cli pattern", options{pr: true, patterns: pat}, nil, true},
		{"flag on with live filter", options{pr: true}, pat, true},
		{"flag on with in-use", options{pr: true, onlyInUse: true}, nil, true},
		{"flag on with open", options{pr: true, onlyOpen: true}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shouldPollPR(c.opts, c.live); got != c.want {
				t.Errorf("shouldPollPR = %v, want %v", got, c.want)
			}
		})
	}
}

func TestProjectWorstCheck(t *testing.T) {
	p := Project{Worktrees: []Worktree{
		{HasPR: true, Check: StateSuccess},
		{}, // no PR: ignored
		{HasPR: true, Check: StateFailure},
		{HasPR: true, Check: StatePending},
	}}
	got, ok := projectWorstCheck(p)
	if !ok || got != StateFailure {
		t.Errorf("projectWorstCheck = (%v, %v), want (Failure, true)", got, ok)
	}

	none := Project{Worktrees: []Worktree{{}, {Check: StateSuccess}}}
	if _, ok := projectWorstCheck(none); ok {
		t.Error("a project with no PR worktrees should report ok=false")
	}
}

func TestEnrichPRChecksCapsProjects(t *testing.T) {
	// More projects than the cap: enrichPRChecks reports it only polled the cap.
	// (gh almost certainly isn't resolvable for these synthetic paths, so no PR
	// data is stamped; we're asserting the bound, not network behaviour.)
	var projects []Project
	for i := 0; i < maxPRPollProjects+3; i++ {
		projects = append(projects, Project{
			Name:      "p",
			Worktrees: []Worktree{{Path: t.TempDir(), Branch: "main"}},
		})
	}
	if polled := enrichPRChecks(projects); polled != maxPRPollProjects {
		t.Errorf("polled = %d, want %d (the cap)", polled, maxPRPollProjects)
	}
}

func TestPRHeaderNote(t *testing.T) {
	pat := []*regexp.Regexp{regexp.MustCompile("x")}

	if note := prHeaderNote(options{}, nil, false); note != "" {
		t.Errorf("note with --pr off should be empty, got %q", note)
	}

	dormant := prHeaderNote(options{pr: true}, nil, false)
	if dormant == "" {
		t.Error("note should explain that --pr is dormant without a filter")
	}

	// Filtered within the cap: no note (the glyphs speak).
	few := []Project{{}, {}}
	if note := prHeaderNote(options{pr: true, patterns: pat}, few, false); note != "" {
		t.Errorf("filtered within cap should have no note, got %q", note)
	}

	// More matches than the cap: a truncation note.
	var many []Project
	for i := 0; i < maxPRPollProjects+1; i++ {
		many = append(many, Project{})
	}
	if note := prHeaderNote(options{pr: true, patterns: pat}, many, false); note == "" {
		t.Error("more projects than the cap should produce a truncation note")
	}

	// The live "/" filter alone is enough to leave dormancy.
	if note := prHeaderNote(options{pr: true}, few, true); note != "" {
		t.Errorf("live filter within cap should have no note, got %q", note)
	}
}
