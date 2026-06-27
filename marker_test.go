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

func TestMarkerActive(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		if markerActive(t.TempDir()) {
			t.Error("no marker file should not be active")
		}
	})

	t.Run("live PID", func(t *testing.T) {
		dir := t.TempDir()
		writeMarker(t, dir, strconv.Itoa(os.Getpid())+"\n")
		if !markerActive(dir) {
			t.Error("marker with our own (live) PID should be active")
		}
	})

	t.Run("dead PID", func(t *testing.T) {
		dir := t.TempDir()
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
