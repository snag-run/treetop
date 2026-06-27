package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// listWorktrees returns every worktree of the repo that `dir` belongs to,
// parsed from `git worktree list --porcelain` and enriched with last-activity.
func listWorktrees(dir string) []Worktree {
	out, err := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	wts := parseWorktrees(out)
	for i := range wts {
		wts[i].Changed, wts[i].HasTime = lastActivity(wts[i].Path)
	}
	return wts
}

// lastActivity returns the most recent git-activity time for a worktree, read
// from the mtimes of its git metadata (index, HEAD, reflog). This reflects
// commits, checkouts, and staging without walking the working tree.
func lastActivity(worktreePath string) (time.Time, bool) {
	gitDir := resolveGitDir(worktreePath)
	if gitDir == "" {
		return time.Time{}, false
	}
	var latest time.Time
	found := false
	for _, name := range []string{"index", "HEAD", filepath.Join("logs", "HEAD")} {
		if fi, err := os.Stat(filepath.Join(gitDir, name)); err == nil {
			if mt := fi.ModTime(); mt.After(latest) {
				latest, found = mt, true
			}
		}
	}
	return latest, found
}

// resolveGitDir returns the git directory backing a worktree. For a linked
// worktree, `.git` is a file pointing at the real dir; for the main worktree
// it's a directory; for a bare repo the worktree path is the git dir itself.
func resolveGitDir(worktreePath string) string {
	dotGit := filepath.Join(worktreePath, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return worktreePath // bare repo: the path is the git dir
	}
	if info.IsDir() {
		return dotGit
	}
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
}

// parseWorktrees parses the output of `git worktree list --porcelain`.
func parseWorktrees(out []byte) []Worktree {
	var wts []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			wts = append(wts, *cur)
			cur = nil
		}
	}

	for line := range strings.SplitSeq(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		case cur == nil:
			// ignore stray lines
		case strings.HasPrefix(line, "branch refs/heads/"):
			cur.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "detached":
			cur.Detached = true
		case line == "bare":
			cur.Bare = true
		}
	}
	flush()

	sort.Slice(wts, func(i, j int) bool { return wts[i].Path < wts[j].Path })
	return wts
}
