package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo creates a real git repo at dir so discovery (gitCommonDir +
// listWorktrees) has something to find.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", dir, err, out)
	}
}

// An unreadable/nonexistent root must not be fatal: discovery continues and the
// bad root is reported in badRoots so the one-shot path can warn about it.
func TestDiscoverProjectsReportsBadRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	projects, badRoots, err := discoverProjects([]string{missing}, 1, nil)
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

// --depth controls how many levels below a root are scanned: a repo two levels
// down is missed at depth 1 but found at depth 2, while the top-level repo is
// always found.
func TestDiscoverProjectsDepth(t *testing.T) {
	root := t.TempDir()
	initRepo(t, filepath.Join(root, "shallow"))     // one level down
	initRepo(t, filepath.Join(root, "org", "deep")) // two levels down

	names := func(depth int) map[string]bool {
		projects, _, err := discoverProjects([]string{root}, depth, nil)
		if err != nil {
			t.Fatalf("discoverProjects(depth=%d): %v", depth, err)
		}
		got := map[string]bool{}
		for _, p := range projects {
			got[p.Name] = true
		}
		return got
	}

	d1 := names(1)
	if !d1["shallow"] {
		t.Error("depth 1 should find the top-level repo")
	}
	if d1["deep"] {
		t.Error("depth 1 should not find a repo nested two levels deep")
	}

	d2 := names(2)
	if !d2["shallow"] || !d2["deep"] {
		t.Errorf("depth 2 should find both repos, got %v", d2)
	}
}
