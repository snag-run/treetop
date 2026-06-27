package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func touch(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestWalkNewest(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	touch(t, filepath.Join(dir, "old.txt"), base)
	newest := base.Add(time.Hour)
	touch(t, filepath.Join(dir, "src", "new.txt"), newest)

	// A much newer file inside .git must be ignored (it isn't a working-tree edit).
	touch(t, filepath.Join(dir, ".git", "index"), base.Add(48*time.Hour))

	got, ok := walkNewest(dir)
	if !ok {
		t.Fatal("expected to find a newest mtime")
	}
	if !got.Equal(newest) {
		t.Errorf("walkNewest = %v, want %v (.git must be pruned)", got, newest)
	}
}

func TestWalkNewestEmpty(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, ".git", "HEAD"), time.Now())
	if _, ok := walkNewest(dir); ok {
		t.Error("a worktree with only .git contents should report no edit time")
	}
}

// A walk that exceeds its budget must stop early and return a partial-but-valid
// result (found==true) rather than running to completion.
func TestWalkNewestBoundedExceedsBudget(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	newest := base.Add(time.Hour)

	// Lay out the tree so the deadline branch must fire before the walk finishes:
	// the deadline is only checked every walkDeadlineCheckEvery entries, so create
	// more than that many "old" files (all mtime=base), plus one newest-mtime file
	// that sorts last. filepath.WalkDir visits in lexical order, so with an
	// already-expired deadline the walk stops at the first check (after
	// walkDeadlineCheckEvery old files) and never reaches the newest file. If the
	// bound did NOT fire, got would equal `newest`.
	for i := 0; i < walkDeadlineCheckEvery+64; i++ {
		touch(t, filepath.Join(dir, "a"+strconv.Itoa(i)+".txt"), base)
	}
	touch(t, filepath.Join(dir, "zzz_newest.txt"), newest)

	got, ok := walkNewestBounded(dir, time.Now().Add(-time.Hour), walkMaxEntries)
	if !ok {
		t.Fatal("expected a partial-but-valid result (found==true) after hitting the deadline")
	}
	if got.Equal(newest) {
		t.Fatal("expired deadline did not stop the walk early: it reached the newest file")
	}
	if !got.Equal(base) {
		t.Errorf("walkNewestBounded = %v, want %v (newest of the visited files)", got, base)
	}

	// A tight max-entries cap is the other bound; it must also return found==true.
	if _, ok := walkNewestBounded(dir, time.Now().Add(time.Hour), 1); !ok {
		t.Error("expected found==true after hitting the max-entries cap")
	}
}
