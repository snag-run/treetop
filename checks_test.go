package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// setGHHealthNote overrides the cached gh-health note for a test and restores it
// afterward, so prHeaderNote assertions don't depend on whether gh happens to be
// installed/authenticated in the test environment.
func setGHHealthNote(t *testing.T, note string) {
	t.Helper()
	ghHealthMu.Lock()
	prev := ghHealthNote
	ghHealthNote = note
	ghHealthMu.Unlock()
	t.Cleanup(func() {
		ghHealthMu.Lock()
		ghHealthNote = prev
		ghHealthMu.Unlock()
	})
}

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
		{"actions startup failure", ghRollupEntry{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "STARTUP_FAILURE"}, StateFailure},
		{"actions stale stays neutral", ghRollupEntry{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "STALE"}, StateNeutral},
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

func TestCheckName(t *testing.T) {
	cases := []struct {
		name string
		e    ghRollupEntry
		want string
	}{
		{"checkrun job name", ghRollupEntry{Typename: "CheckRun", Name: "build (ubuntu)", WorkflowName: "ci"}, "build (ubuntu)"},
		{"status context", ghRollupEntry{Typename: "StatusContext", Context: "ci/circleci"}, "ci/circleci"},
		{"falls back to workflow", ghRollupEntry{Typename: "CheckRun", WorkflowName: "ci"}, "ci"},
		{"generic fallback", ghRollupEntry{Typename: "CheckRun"}, "check"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := checkName(c.e); got != c.want {
				t.Errorf("checkName(%+v) = %q, want %q", c.e, got, c.want)
			}
		})
	}
}

func TestRollupChecks(t *testing.T) {
	entries := []ghRollupEntry{
		{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS", Name: "build"},
		{Typename: "StatusContext", State: "FAILURE", Context: "lint"},
		{Typename: "CheckRun", Status: "IN_PROGRESS", Name: "test"},
		{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS", Name: "audit"},
	}
	got := rollupChecks(entries)

	// Worst-first, then alphabetical: the failure leads, the pending follows, then
	// the two successes in name order.
	want := []Check{
		{Name: "lint", State: StateFailure},
		{Name: "test", State: StatePending},
		{Name: "audit", State: StateSuccess},
		{Name: "build", State: StateSuccess},
	}
	if len(got) != len(want) {
		t.Fatalf("rollupChecks returned %d checks, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("check[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// An empty rollup expands to nothing (a PR with no configured checks).
	if got := rollupChecks(nil); got != nil {
		t.Errorf("empty rollup should expand to nil, got %+v", got)
	}
}

// GitHub's statusCheckRollup can return two CheckRuns with the same name — most
// visibly after a PR is closed and reopened, which leaves a stale run beside the
// fresh one. rollupChecks must collapse them into a single row, folding to the
// worst state so the row matches the check's contribution to the glyph.
func TestRollupChecksDeduplicates(t *testing.T) {
	entries := []ghRollupEntry{
		{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS", Name: "changes"},
		{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS", Name: "changes"},
		{Typename: "CheckRun", Status: "IN_PROGRESS", Name: "ci"},
		{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS", Name: "ci"},
	}
	got := rollupChecks(entries)

	// "changes" collapses to one success; "ci" folds the pending and success into
	// one pending row (worst wins), which leads.
	want := []Check{
		{Name: "ci", State: StatePending},
		{Name: "changes", State: StateSuccess},
	}
	if len(got) != len(want) {
		t.Fatalf("rollupChecks returned %d checks, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("check[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestRollupChecksCapturesURL asserts each per-check row carries its link: a
// CheckRun's detailsUrl (its Actions run page) and a StatusContext's targetUrl
// (the external CI's page). When the same name folds twice, the first non-empty
// link is kept so a later URL-less stale run can't blank an established link.
func TestRollupChecksCapturesURL(t *testing.T) {
	const runURL = "https://github.com/o/r/actions/runs/1"
	const extURL = "https://ci.example.com/build/9"
	entries := []ghRollupEntry{
		{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS", Name: "build", DetailsURL: runURL},
		{Typename: "StatusContext", State: "SUCCESS", Context: "lint", TargetURL: extURL},
		{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS", Name: "build"}, // dup, no URL
	}
	got := rollupChecks(entries)

	urls := map[string]string{}
	for _, c := range got {
		urls[c.Name] = c.URL
	}
	if urls["build"] != runURL {
		t.Errorf("build URL = %q, want %q (first link kept across the dup)", urls["build"], runURL)
	}
	if urls["lint"] != extURL {
		t.Errorf("lint URL = %q, want %q", urls["lint"], extURL)
	}
}

// TestGHPRJSONUnmarshal asserts the PR-level url and the per-entry detailsUrl /
// targetUrl fields we added to the gh query actually decode, so the link data
// reaches the renderer.
func TestGHPRJSONUnmarshal(t *testing.T) {
	const blob = `[{"number":7,"url":"https://github.com/o/r/pull/7","headRefName":"feat",
	  "statusCheckRollup":[
	    {"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":"https://github.com/o/r/actions/runs/1"},
	    {"__typename":"StatusContext","context":"lint","state":"SUCCESS","targetUrl":"https://ci.example.com/9"}
	  ]}]`
	var prs []ghPR
	if err := json.Unmarshal([]byte(blob), &prs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(prs))
	}
	if prs[0].URL != "https://github.com/o/r/pull/7" {
		t.Errorf("PR url = %q", prs[0].URL)
	}
	if prs[0].Rollup[0].DetailsURL == "" || prs[0].Rollup[1].TargetURL == "" {
		t.Errorf("rollup link fields did not decode: %+v", prs[0].Rollup)
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
	setGHHealthNote(t, "") // assume gh is healthy for the base cases

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

func TestPRHeaderNoteGHProblem(t *testing.T) {
	pat := []*regexp.Regexp{regexp.MustCompile("x")}
	setGHHealthNote(t, "PR checks: gh not authenticated — run `gh auth login`")

	// Active + filtered: the gh problem surfaces, and takes precedence over the
	// over-cap truncation note (a dead gh matters more than how many matched).
	var many []Project
	for i := 0; i < maxPRPollProjects+1; i++ {
		many = append(many, Project{})
	}
	note := prHeaderNote(options{pr: true, patterns: pat}, many, false)
	if !strings.Contains(note, "authenticated") {
		t.Errorf("active polling with unusable gh should report it, got %q", note)
	}

	// Dormant (no filter): still the "filter to enable" message — we don't nag
	// about gh auth until polling is actually attempted.
	if note := prHeaderNote(options{pr: true}, nil, false); !strings.Contains(note, "filter") {
		t.Errorf("dormant note should ask to filter, not mention gh, got %q", note)
	}

	// Flag off: nothing, regardless of gh health.
	if note := prHeaderNote(options{}, many, false); note != "" {
		t.Errorf("--pr off should suppress the gh note, got %q", note)
	}
}

func TestPRReviewOf(t *testing.T) {
	for _, c := range []struct {
		name string
		pr   ghPR
		want PRReview
	}{
		{"approved", ghPR{ReviewDecision: "APPROVED"}, ReviewApproved},
		{"changes", ghPR{ReviewDecision: "CHANGES_REQUESTED"}, ReviewChangesRequested},
		{"required", ghPR{ReviewDecision: "REVIEW_REQUIRED"}, ReviewRequired},
		{"none", ghPR{ReviewDecision: ""}, ReviewNone},
		{"draft", ghPR{IsDraft: true}, ReviewDraft},
		// Draft wins over any decision: a draft isn't up for review.
		{"draft beats decision", ghPR{IsDraft: true, ReviewDecision: "APPROVED"}, ReviewDraft},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := prReviewOf(c.pr); got != c.want {
				t.Errorf("prReviewOf(%+v) = %v, want %v", c.pr, got, c.want)
			}
		})
	}
}
