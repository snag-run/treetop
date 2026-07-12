package main

import (
	"path/filepath"
	"testing"
	"time"
)

// TestMarkInUseTiers drives markInUse through the three activity tiers using only
// the scan signals (no marker files), asserting that anchoring (Rooted, from the
// cwd scan) and work (Activity, from the open-file scan) stay independent and
// decay on their own clocks.
func TestMarkInUseTiers(t *testing.T) {
	clock := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	trs := newTrackers()
	trs.root.now = func() time.Time { return clock }
	trs.work.now = func() time.Time { return clock }

	realDir := func() string {
		d := t.TempDir()
		if r, err := filepath.EvalSymlinks(d); err == nil {
			d = r
		}
		return d
	}
	rooted := realDir() // a session anchored here (its cwd)
	worked := realDir() // a subagent touching files here, but not cwd'd here

	projects := func() []Project {
		return []Project{{Name: "p", Worktrees: []Worktree{{Path: rooted}, {Path: worked}}}}
	}
	at := func(ps []Project) (Worktree, Worktree) { return ps[0].Worktrees[0], ps[0].Worktrees[1] }

	// Pass 1 — both signals live now.
	live := sessionScan{
		supported: true,
		cwds:      []string{rooted},
		openFiles: []string{filepath.Join(worked, "main.go")},
	}
	ps := projects()
	live.markInUse(trs, ps)
	r, w := at(ps)
	if !r.Rooted || r.Activity != ActIdle {
		t.Errorf("anchored-only: got Rooted=%v Activity=%v, want Rooted=true Activity=Idle", r.Rooted, r.Activity)
	}
	if w.Rooted || w.Activity != ActActive {
		t.Errorf("worked-not-rooted (drift): got Rooted=%v Activity=%v, want Rooted=false Activity=Active", w.Rooted, w.Activity)
	}

	// Pass 2 — signals gone, 31s later: past inUseDecay, so the anchor clears and
	// work drops from Active to Recent.
	clock = clock.Add(inUseDecay + time.Second)
	gone := sessionScan{supported: true}
	ps = projects()
	gone.markInUse(trs, ps)
	r, w = at(ps)
	if r.Rooted {
		t.Error("anchor should clear once inUseDecay lapses")
	}
	if w.Activity != ActRecent {
		t.Errorf("work should decay to Recent after inUseDecay, got %v", w.Activity)
	}

	// Pass 3 — past recentTTL: fully idle.
	clock = clock.Add(recentTTL)
	ps = projects()
	gone.markInUse(trs, ps)
	_, w = at(ps)
	if w.Activity != ActIdle {
		t.Errorf("work should decay to Idle past recentTTL, got %v", w.Activity)
	}
}
