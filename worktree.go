package main

import (
	"os/exec"
	"sort"
	"strings"
)

// listWorktrees returns every worktree of the repo that `dir` belongs to,
// parsed from `git worktree list --porcelain`.
func listWorktrees(dir string) []Worktree {
	out, err := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	return parseWorktrees(out)
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
