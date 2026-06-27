package main

import (
	"testing"
	"time"
)

func TestTrackerDecay(t *testing.T) {
	clock := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	tr := newTracker(30 * time.Second)
	tr.now = func() time.Time { return clock }

	if tr.active("/wt") {
		t.Fatal("unobserved path should not be active")
	}

	tr.observe("/wt")
	if !tr.active("/wt") {
		t.Fatal("just-observed path should be active")
	}

	clock = clock.Add(29 * time.Second)
	if !tr.active("/wt") {
		t.Error("path within decay window should still be active")
	}

	clock = clock.Add(2 * time.Second) // now 31s since observe
	if tr.active("/wt") {
		t.Error("path past decay window should no longer be active")
	}

	// A fresh observation re-arms the window.
	tr.observe("/wt")
	if !tr.active("/wt") {
		t.Error("re-observed path should be active again")
	}
}
