package main

import (
	"testing"
	"time"
)

func TestTrackerWithin(t *testing.T) {
	clock := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	tr := newTracker()
	tr.now = func() time.Time { return clock }

	if tr.within("/wt", 30*time.Second) {
		t.Fatal("unobserved path should not be within any window")
	}

	tr.observe("/wt")
	if !tr.within("/wt", 30*time.Second) {
		t.Fatal("just-observed path should be within the window")
	}

	clock = clock.Add(29 * time.Second)
	if !tr.within("/wt", 30*time.Second) {
		t.Error("path inside the window should still count")
	}

	clock = clock.Add(2 * time.Second) // now 31s since observe
	if tr.within("/wt", 30*time.Second) {
		t.Error("path past the window should no longer count")
	}

	// The same last-seen time answers a longer window differently — this is what
	// lets one observation feed both the active and recent tiers.
	if !tr.within("/wt", 15*time.Minute) {
		t.Error("31s-old sighting should still be within a 15m window")
	}

	// A fresh observation re-arms the short window.
	tr.observe("/wt")
	if !tr.within("/wt", 30*time.Second) {
		t.Error("re-observed path should count again")
	}
}
