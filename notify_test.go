package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
	"time"
)

// wt builds a worktree with an open PR for the notifier tests.
func wt(path, branch string, review PRReview, check CheckState, checks ...Check) Worktree {
	return Worktree{
		Path:     path,
		Branch:   branch,
		HasPR:    true,
		PRReview: review,
		Check:    check,
		Checks:   checks,
	}
}

// proj wraps worktrees in a single named project.
func proj(name string, wts ...Worktree) []Project {
	return []Project{{Name: name, Worktrees: wts}}
}

// bodies extracts the notification bodies for terse assertions.
func bodies(notes []notification) []string {
	out := make([]string, len(notes))
	for i, n := range notes {
		out[i] = n.body
	}
	return out
}

func TestNotifierDisabledIsNoOp(t *testing.T) {
	n := newNotifier(false)
	// Even a textbook transition produces nothing when disabled.
	n.diff(proj("snag", wt("/a", "feat", ReviewNone, StateSuccess)))
	got := n.diff(proj("snag", wt("/a", "feat", ReviewApproved, StateSuccess)))
	if len(got) != 0 {
		t.Fatalf("disabled notifier emitted %v", bodies(got))
	}
}

func TestFirstObservationSuppressed(t *testing.T) {
	n := newNotifier(true)
	// Launching onto an already-changes-requested, already-failing PR must be
	// silent: the first sight only establishes the baseline.
	got := n.diff(proj("snag", wt("/a", "feat", ReviewChangesRequested, StateFailure)))
	if len(got) != 0 {
		t.Fatalf("first observation emitted %v", bodies(got))
	}
}

func TestReviewTransitions(t *testing.T) {
	tests := []struct {
		name string
		to   PRReview
		want string
	}{
		{"approved", ReviewApproved, "snag/feat — approved"},
		{"changes requested", ReviewChangesRequested, "snag/feat — changes requested"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := newNotifier(true)
			n.diff(proj("snag", wt("/a", "feat", ReviewNone, StateSuccess))) // baseline
			got := n.diff(proj("snag", wt("/a", "feat", tc.to, StateSuccess)))
			if want := []string{tc.want}; !equalStrings(bodies(got), want) {
				t.Fatalf("got %v, want %v", bodies(got), want)
			}
			// Staying in the state must not re-notify.
			if again := n.diff(proj("snag", wt("/a", "feat", tc.to, StateSuccess))); len(again) != 0 {
				t.Fatalf("steady state re-notified %v", bodies(again))
			}
		})
	}
}

func TestBranchSwitchRebaselines(t *testing.T) {
	n := newNotifier(true)
	n.diff(proj("snag", wt("/a", "feat", ReviewNone, StateSuccess))) // baseline on feat
	// Same worktree path, different branch (a different PR): even though review
	// moved to changes-requested, the branch switch re-baselines silently.
	if got := n.diff(proj("snag", wt("/a", "feat2", ReviewChangesRequested, StateSuccess))); len(got) != 0 {
		t.Fatalf("branch switch notified %v", bodies(got))
	}
	// A genuine transition on the new branch now notifies.
	got := n.diff(proj("snag", wt("/a", "feat2", ReviewApproved, StateSuccess)))
	if want := []string{"snag/feat2 — approved"}; !equalStrings(bodies(got), want) {
		t.Fatalf("got %v, want %v", bodies(got), want)
	}
}

func TestCISettleGating(t *testing.T) {
	n := newNotifier(true)
	n.diff(proj("snag", wt("/a", "feat", ReviewNone, StateSuccess))) // baseline: CI fine

	// Rollup folds to failure while a check is still pending: not settled, no ping.
	failing := wt("/a", "feat", ReviewNone, StateFailure,
		Check{Name: "test", State: StateFailure}, Check{Name: "lint", State: StatePending})
	if got := n.diff(proj("snag", failing)); len(got) != 0 {
		t.Fatalf("early failure (still pending) notified %v", bodies(got))
	}

	// Run settles to failure (nothing pending): one ping.
	settled := wt("/a", "feat", ReviewNone, StateFailure,
		Check{Name: "test", State: StateFailure}, Check{Name: "lint", State: StateSuccess})
	got := n.diff(proj("snag", settled))
	if want := []string{"snag/feat — CI failed"}; !equalStrings(bodies(got), want) {
		t.Fatalf("got %v, want %v", bodies(got), want)
	}

	// Still failing on the next refresh: no transition, no repeat.
	if again := n.diff(proj("snag", settled)); len(again) != 0 {
		t.Fatalf("steady failure re-notified %v", bodies(again))
	}
}

func TestCooldownSuppressesFlap(t *testing.T) {
	n := newNotifier(true)
	now := time.Now()
	n.now = func() time.Time { return now }

	other := proj("snag", wt("/a", "feat", ReviewNone, StateSuccess))
	failed := proj("snag", wt("/a", "feat", ReviewNone, StateFailure, Check{Name: "test", State: StateFailure}))

	n.diff(other)                             // baseline
	if got := n.diff(failed); len(got) != 1 { // first failure fires
		t.Fatalf("first failure: got %v, want 1", bodies(got))
	}
	n.diff(other) // back to green: clears the failed baseline

	now = now.Add(1 * time.Minute) // within cooldown
	if got := n.diff(failed); len(got) != 0 {
		t.Fatalf("flap within cooldown notified %v", bodies(got))
	}
	n.diff(other)

	now = now.Add(5 * time.Minute) // past cooldown
	if got := n.diff(failed); len(got) != 1 {
		t.Fatalf("failure past cooldown: got %v, want 1", bodies(got))
	}
}

func TestSweepForgetsDepartedWorktree(t *testing.T) {
	n := newNotifier(true)
	n.diff(proj("snag", wt("/a", "feat", ReviewApproved, StateSuccess))) // baseline, in view
	n.diff(proj("snag"))                                                 // /a leaves view (e.g. filter narrowed)
	// /a returns, still approved: treated as a fresh first observation, so silent
	// — not replayed as an approval.
	if got := n.diff(proj("snag", wt("/a", "feat", ReviewApproved, StateSuccess))); len(got) != 0 {
		t.Fatalf("returning worktree replayed %v", bodies(got))
	}
}

func TestOSC9Format(t *testing.T) {
	t.Setenv("TMUX", "") // ensure no passthrough wrapping
	got := osc9("snag/feat — approved")
	want := "\033]9;snag/feat — approved\033\\"
	if got != want {
		t.Fatalf("osc9 = %q, want %q", got, want)
	}
}

func TestWrapPassthroughTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	got := wrapPassthrough("\033]9;hi\033\\")
	if !strings.HasPrefix(got, "\033Ptmux;") {
		t.Fatalf("missing tmux passthrough prefix: %q", got)
	}
	if !strings.HasSuffix(got, "\033\\") {
		t.Fatalf("missing ST terminator: %q", got)
	}
	// The inner sequence must have every ESC doubled so tmux passes it through.
	if !strings.Contains(got, "\033\033]9;hi\033\033\\") {
		t.Fatalf("inner ESC not doubled: %q", got)
	}
}

func TestWrapPassthroughNoTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	seq := "\033]9;hi\033\\"
	if got := wrapPassthrough(seq); got != seq {
		t.Fatalf("wrapped outside tmux: %q", got)
	}
}

func TestRaiseNotificationsWritesOSC9(t *testing.T) {
	t.Setenv("TMUX", "")
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	raiseNotifications(w, []notification{{body: "snag/feat — CI failed"}})
	w.Flush()
	want := "\033]9;snag/feat — CI failed\033\\"
	if buf.String() != want {
		t.Fatalf("raiseNotifications wrote %q, want %q", buf.String(), want)
	}
}

func TestNotifyBodyFallsBackToRef(t *testing.T) {
	// A detached worktree has no branch; the body falls back to its Ref().
	w := Worktree{Path: "/a", Detached: true, HasPR: true}
	if got := notifyBody("snag", w, "CI failed"); got != "snag/(detached) — CI failed" {
		t.Fatalf("notifyBody = %q", got)
	}
}

func TestNotifyBodySanitizesControlRunes(t *testing.T) {
	// A branch (or project) name carrying an escape byte must be scrubbed before
	// it reaches the OSC 9 sequence — otherwise it injects into the terminal.
	w := Worktree{Path: "/a", Branch: "feat\033]9;pwn\033\\evil", HasPR: true}
	got := notifyBody("snag", w, "approved")
	if strings.ContainsRune(got, '\033') {
		t.Fatalf("notifyBody left an ESC in the body: %q", got)
	}
}

// equalStrings reports whether two string slices are equal.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
