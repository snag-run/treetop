package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// notification is a single desktop notification to surface: a body identifying
// the worktree and what changed (e.g. "snag/feat-x — changes requested").
type notification struct {
	body string
}

// notifyState is the last-seen salient PR state of one worktree, the baseline
// each refresh diffs against. Only the fields that drive a notification are
// kept: the review decision, whether CI is a *settled* failure, and the branch
// (a branch switch is a different PR, so it re-baselines rather than notifies).
type notifyState struct {
	branch   string
	review   PRReview
	ciFailed bool
}

// notifier turns successive dashboard snapshots into desktop notifications for
// the transitions that need a human: a PR newly approved or sent back for
// changes, and CI newly failed. It keeps a per-worktree baseline across
// refreshes and fires only on an actual change into one of those states.
//
// Enabled by --notify (watch mode only). When disabled, diff is a no-op so the
// refresh path pays nothing.
type notifier struct {
	enabled bool
	seen    map[string]notifyState // by worktree path
}

func newNotifier(enabled bool) *notifier {
	return &notifier{
		enabled: enabled,
		seen:    map[string]notifyState{},
	}
}

// diff compares the worktrees in projects against the last-seen baseline and
// returns the notifications to surface this refresh, updating the baseline in
// place. It considers only worktrees with an open PR (HasPR) — the same set the
// --pr column is fetched for — so notifications inherit --pr's gating.
//
// The first time a worktree is seen (or after its branch changes) it is recorded
// silently: launching treetop onto an already-red or already-changes-requested
// PR must not fire a storm; only post-baseline transitions notify.
func (n *notifier) diff(projects []Project) []notification {
	if !n.enabled {
		return nil
	}
	var out []notification
	present := make(map[string]bool)
	for _, p := range projects {
		for i := range p.Worktrees {
			w := p.Worktrees[i]
			if !w.HasPR {
				continue
			}
			present[w.Path] = true
			cur := notifyState{
				branch:   w.Branch,
				review:   w.PRReview,
				ciFailed: ciSettledFailure(w),
			}
			prev, ok := n.seen[w.Path]
			n.seen[w.Path] = cur
			if !ok || prev.branch != cur.branch {
				continue // first observation / branch switch: baseline only
			}
			// Reviews are instantaneous human events: notify on the transition.
			if cur.review != prev.review {
				switch cur.review {
				case ReviewApproved:
					out = fire(out, p.Name, w, "approved")
				case ReviewChangesRequested:
					out = fire(out, p.Name, w, "changes requested")
				}
			}
			// CI: rollup-keyed and settle-gated, so this is one ping per run when
			// the rollup lands on a terminal failure — never per check, never the
			// early flake while other checks are still running. A re-pushed run
			// that fails again is a fresh other→failed transition, so it notifies
			// again — which is intended (it's a new run, not a duplicate).
			if cur.ciFailed && !prev.ciFailed {
				out = fire(out, p.Name, w, "CI failed")
			}
		}
	}
	n.sweep(present)
	return out
}

// fire appends a notification for a worktree transition. No de-duplication is
// needed here: diff only calls it on an actual state change (edge-triggered), and
// CI is settle-gated so a single failing run yields exactly one other→failed
// transition. Repeats therefore only happen across genuinely distinct events — a
// re-pushed run, a re-requested review, a branch switch — all of which should
// notify again.
func fire(out []notification, project string, w Worktree, what string) []notification {
	return append(out, notification{body: notifyBody(project, w, what)})
}

// notifyBody identifies the worktree and what changed, e.g.
// "snag/feat-renderer-host-brand — changes requested". The project and branch
// come straight from the filesystem and git, so they're scrubbed of control
// runes the same way the rendered table is (see sanitizeDisplay): otherwise a
// branch named with an embedded ESC/ST would inject into the OSC 777 sequence.
func notifyBody(project string, w Worktree, what string) string {
	ref := w.Branch
	if ref == "" {
		ref = w.Ref()
	}
	return sanitizeDisplay(project) + "/" + sanitizeDisplay(ref) + " — " + what
}

// sweep forgets worktrees no longer in view so a later reappearance re-baselines
// (silently) and the map doesn't grow across a long session.
func (n *notifier) sweep(present map[string]bool) {
	for path := range n.seen {
		if !present[path] {
			delete(n.seen, path)
		}
	}
}

// ciSettledFailure reports whether a worktree's CI rollup is a *settled* failure:
// the worst-wins state is a failure and no check is still pending. Gating on "no
// pending" is what makes CI notify once, when the run finishes failing, rather
// than the instant the first check goes red while others are still running.
func ciSettledFailure(w Worktree) bool {
	if w.Check != StateFailure {
		return false
	}
	for _, c := range w.Checks {
		if c.State == StatePending {
			return false
		}
	}
	return true
}

// notifyTitle is the fixed title on every notification. OSC 777 carries an
// explicit title, so treetop names itself rather than letting the terminal fall
// back to the window title — which the shell tends to set to the running command
// line, leaking "treetop -w --checks --pr --notify" into the notification.
const notifyTitle = "treetop"

// raiseNotifications writes each pending notification to the terminal as an
// OSC 777 desktop notification. It must be called from the main loop, which owns
// out, so the writes never interleave with the refresh goroutine. The frame's own
// flush (in render) pushes these out; an empty slice is a no-op.
func raiseNotifications(out *bufio.Writer, notes []notification) {
	for _, nt := range notes {
		// Best-effort: the buffered write surfaces any error at the frame's
		// flush in render, so discard it here.
		_, _ = fmt.Fprint(out, osc777(notifyTitle, nt.body))
	}
}

// osc777 builds an OSC 777 desktop-notification escape sequence with an explicit
// title and body, wrapped for tmux passthrough when running inside tmux. OSC 777
// (ESC ] 777 ; notify ; <title> ; <body> ST) is rendered as a system notification
// by Ghostty (the target), WezTerm, kitty, and urxvt; terminals that don't
// understand it swallow the OSC string and show nothing. Unlike OSC 9 it carries
// a title, so the notification never inherits the shell's window title. The
// terminator is ST (ESC \), not BEL, so we never ring the bell.
func osc777(title, body string) string {
	const esc = "\033"
	return wrapPassthrough(esc + "]777;notify;" + title + ";" + body + esc + `\`)
}

// wrapPassthrough wraps an escape sequence so it survives tmux, which otherwise
// swallows OSC it doesn't recognise. Outside tmux the sequence is returned
// unchanged. tmux passthrough is ESC P tmux ; <payload, every ESC doubled> ESC \,
// and needs `set -g allow-passthrough on` (the default since tmux 3.3).
func wrapPassthrough(seq string) string {
	if os.Getenv("TMUX") == "" {
		return seq
	}
	const esc = "\033"
	return esc + "Ptmux;" + strings.ReplaceAll(seq, esc, esc+esc) + esc + `\`
}
