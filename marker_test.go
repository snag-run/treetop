package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func writeMarker(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, markerName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// stubPIDIsAgent overrides the marker's PID identity check with a fixed result
// for the duration of the test, so the PID code paths are deterministic without
// depending on what the real process table looks like.
func stubPIDIsAgent(t *testing.T, result bool) {
	t.Helper()
	prev := pidIsAgentFunc
	pidIsAgentFunc = func(int) bool { return result }
	t.Cleanup(func() { pidIsAgentFunc = prev })
}

func TestMarkerActive(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		if markerActive(t.TempDir()) {
			t.Error("no marker file should not be active")
		}
	})

	t.Run("live PID, verified agent", func(t *testing.T) {
		dir := t.TempDir()
		stubPIDIsAgent(t, true)
		writeMarker(t, dir, strconv.Itoa(os.Getpid())+"\n")
		if !markerActive(dir) {
			t.Error("marker with a live, agent-verified PID should be active")
		}
	})

	t.Run("live PID, not an agent", func(t *testing.T) {
		dir := t.TempDir()
		// A recycled PID points at a live but unrelated process: the identity
		// check must reject it so the worktree isn't pinned in-use forever.
		stubPIDIsAgent(t, false)
		writeMarker(t, dir, strconv.Itoa(os.Getpid())+"\n")
		if markerActive(dir) {
			t.Error("marker whose live PID isn't an agent process should not be active")
		}
	})

	t.Run("dead PID", func(t *testing.T) {
		dir := t.TempDir()
		// pidAlive short-circuits before the identity check; assert it stays
		// inactive even when identity verification would pass.
		stubPIDIsAgent(t, true)
		writeMarker(t, dir, "999999\n")
		if markerActive(dir) {
			t.Error("marker with a dead PID should not be active")
		}
	})

	t.Run("no PID, fresh mtime", func(t *testing.T) {
		dir := t.TempDir()
		writeMarker(t, dir, "")
		if !markerActive(dir) {
			t.Error("empty marker with a fresh mtime should be active")
		}
	})

	t.Run("no PID, stale mtime", func(t *testing.T) {
		dir := t.TempDir()
		writeMarker(t, dir, "")
		old := time.Now().Add(-2 * markerTTL)
		if err := os.Chtimes(filepath.Join(dir, markerName), old, old); err != nil {
			t.Fatal(err)
		}
		if markerActive(dir) {
			t.Error("empty marker older than markerTTL should not be active")
		}
	})

	t.Run("no PID, future mtime", func(t *testing.T) {
		dir := t.TempDir()
		writeMarker(t, dir, "")
		future := time.Now().Add(time.Hour)
		if err := os.Chtimes(filepath.Join(dir, markerName), future, future); err != nil {
			t.Fatal(err)
		}
		if markerActive(dir) {
			t.Error("marker with a future mtime should not be active")
		}
	})

	t.Run("symlink is not followed", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "real")
		// A live-PID payload that WOULD pass if the symlink were followed.
		if err := os.WriteFile(target, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, markerName)); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}
		if markerActive(dir) {
			t.Error("a symlink marker should be rejected, not followed")
		}
	})
}

func TestMarkerActiveWithin(t *testing.T) {
	// A timestamp marker aged between the two windows: fresh for the recent tier,
	// stale for the active tier. This is what separates ActActive from ActRecent.
	dir := t.TempDir()
	writeMarker(t, dir, "treetop in-use since whenever\n")
	age := (activeTTL + recentTTL) / 2
	stamp := time.Now().Add(-age)
	if err := os.Chtimes(filepath.Join(dir, markerName), stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if markerActiveWithin(dir, activeTTL) {
		t.Errorf("marker aged %s should be stale for the active window (%s)", age, activeTTL)
	}
	if !markerActiveWithin(dir, recentTTL) {
		t.Errorf("marker aged %s should be fresh for the recent window (%s)", age, recentTTL)
	}

	// A live PID overrides the window entirely (legacy back-compat).
	pdir := t.TempDir()
	stubPIDIsAgent(t, true)
	writeMarker(t, pdir, strconv.Itoa(os.Getpid())+"\n")
	old := time.Now().Add(-2 * recentTTL)
	if err := os.Chtimes(filepath.Join(pdir, markerName), old, old); err != nil {
		t.Fatal(err)
	}
	if !markerActiveWithin(pdir, activeTTL) {
		t.Error("a live-PID legacy marker should be active regardless of mtime")
	}
}

func TestFirstLinePID(t *testing.T) {
	tests := []struct {
		in      string
		wantPID int
		wantOK  bool
	}{
		{"1234\n", 1234, true},
		{"1234", 1234, true},
		{"  1234  \nextra", 1234, true},
		{"", 0, false},
		{"\n", 0, false},
		{"notapid", 0, false},
		{"0", 0, false},
		{"-5", 0, false},
	}
	for _, tt := range tests {
		pid, ok := firstLinePID([]byte(tt.in))
		if ok != tt.wantOK || pid != tt.wantPID {
			t.Errorf("firstLinePID(%q) = (%d, %v), want (%d, %v)", tt.in, pid, ok, tt.wantPID, tt.wantOK)
		}
	}
}
