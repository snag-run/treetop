package main

import (
	"os"
	"path/filepath"
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
