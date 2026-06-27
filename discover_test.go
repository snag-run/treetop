package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// An unreadable/nonexistent root must not be fatal: discovery continues and the
// bad root is reported in badRoots so the one-shot path can warn about it.
func TestDiscoverProjectsReportsBadRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	projects, badRoots, err := discoverProjects([]string{missing}, nil)
	if err != nil {
		t.Fatalf("discoverProjects returned a fatal error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected no projects from a missing root, got %d", len(projects))
	}
	if len(badRoots) != 1 {
		t.Fatalf("expected 1 bad root, got %d: %v", len(badRoots), badRoots)
	}
	if !strings.Contains(badRoots[0], missing) {
		t.Errorf("bad-root entry %q does not mention the root %q", badRoots[0], missing)
	}
}
