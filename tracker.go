package main

import "time"

// tracker keeps a short memory of which worktrees recently showed a live
// session signal, so transient signals (an open file descriptor that is only
// held for the duration of a read/write) don't make the in-use marker flicker.
//
// A worktree is considered active for `decay` after it was last observed. In
// snapshot mode the tracker is created fresh, so decay is a no-op and a signal
// shows immediately; in watch mode one tracker lives across refreshes and
// smooths the marker over the decay window.
type tracker struct {
	decay    time.Duration
	lastSeen map[string]time.Time
	now      func() time.Time // injectable for tests
}

func newTracker(decay time.Duration) *tracker {
	return &tracker{
		decay:    decay,
		lastSeen: map[string]time.Time{},
		now:      time.Now,
	}
}

// observe records that a live session signal was seen at path right now.
func (t *tracker) observe(path string) {
	t.lastSeen[path] = t.now()
}

// active reports whether path was observed within the decay window.
func (t *tracker) active(path string) bool {
	seen, ok := t.lastSeen[path]
	if !ok {
		return false
	}
	return t.now().Sub(seen) <= t.decay
}
