package main

import "time"

// tracker keeps a short memory of when each worktree last showed a given session
// signal, so a transient signal (an open file descriptor held only for the
// duration of a read/write) doesn't make an indicator flicker, and so a longer
// window can report "recently active" after the signal has gone.
//
// It records last-seen times only; callers ask about a specific window with
// within. In snapshot mode a fresh tracker has no history, so only signals seen
// in this pass count; in watch mode one tracker lives across refreshes and
// remembers earlier sightings.
type tracker struct {
	lastSeen map[string]time.Time
	now      func() time.Time // injectable for tests
}

func newTracker() *tracker {
	return &tracker{
		lastSeen: map[string]time.Time{},
		now:      time.Now,
	}
}

// observe records that the signal was seen at path right now.
func (t *tracker) observe(path string) {
	t.lastSeen[path] = t.now()
}

// within reports whether path was observed within the last d.
func (t *tracker) within(path string, d time.Duration) bool {
	seen, ok := t.lastSeen[path]
	if !ok {
		return false
	}
	age := t.now().Sub(seen)
	return age >= 0 && age <= d
}

// trackers pairs the two session-scan signals, kept apart so a worktree that is
// only a session's anchor (cwd) and one that is only being worked in (open
// files) don't smear into each other's recency.
type trackers struct {
	root *tracker // cwd sightings — where a session is anchored
	work *tracker // open-file sightings — where work is touching files
}

func newTrackers() *trackers {
	return &trackers{root: newTracker(), work: newTracker()}
}
