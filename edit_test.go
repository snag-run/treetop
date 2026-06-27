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
	for i := 0; i < 50; i++ {
		touch(t, filepath.Join(dir, "f"+strconv.Itoa(i)+".txt"), base)
	}

	// An already-expired deadline forces the bound to trip on the first entry.
	got, ok := walkNewestBounded(dir, time.Now().Add(-time.Hour), walkMaxEntries)
	if !ok {
		t.Fatal("expected a partial-but-valid result (found==true) after hitting the budget")
	}
	if !got.Equal(base) {
		t.Errorf("walkNewestBounded = %v, want %v", got, base)
	}

	// A tight max-entries cap is the other bound; it must also return found==true.
	if _, ok := walkNewestBounded(dir, time.Now().Add(time.Hour), 1); !ok {
		t.Error("expected found==true after hitting the max-entries cap")
	}
}
