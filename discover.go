package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// discoverProjects scans the given roots up to depth levels deep for git
// worktrees, groups them by repository, and returns one Project per repo with
// all of its worktrees. A bare repo is discovered via any of its linked
// worktrees, so it need not live under a root itself.
//
// depth is how many directory levels below each root to look for repos: 1 scans
// a root's immediate children (the original behaviour), 2 also scans their
// children, and so on — useful for nested layouts like ~/src/<host>/<org>/<repo>.
// A repo is never descended into, so a depth larger than the layout costs only
// the directory stats. depth < 1 is treated as 1.
//
// keep filters projects by name *before* the expensive per-worktree enrichment
// (git queries + working-tree walk in listWorktrees), so projects the caller has
// filtered out cost nothing beyond cheap name discovery. A nil keep enriches
// every project.
//
// A missing/unreadable root is not fatal: scanning continues with the other
// roots and the bad root is returned (as a "<root>: <err>" string) in badRoots
// so the caller can warn about it. Surfacing rather than printing keeps the
// watch-mode refresh path quiet (it would corrupt the live TUI every tick). Only
// the explicitly-passed roots are reported; an unreadable nested subdirectory is
// skipped silently.
func discoverProjects(roots []string, depth int, keep func(name string) bool) (projects []Project, badRoots []string, err error) {
	if depth < 1 {
		depth = 1
	}
	// Map from a repo's common git dir -> a known worktree path we can query.
	seen := map[string]string{}

	for _, root := range roots {
		if err := collectRepos(root, depth, seen); err != nil {
			badRoots = append(badRoots, fmt.Sprintf("%s: %v", root, err))
		}
	}

	for common, anyWorktree := range seen {
		name := projectName(common)
		if keep != nil && !keep(name) {
			continue // filtered out: skip the costly worktree enrichment
		}
		wts := listWorktrees(anyWorktree)
		if len(wts) == 0 {
			continue
		}
		projects = append(projects, Project{
			Name:      name,
			Worktrees: wts,
		})
	}

	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, badRoots, nil
}

// collectRepos records every git repo found in dir, recursing up to depth
// levels. A directory containing a .git entry is a repo (or linked worktree):
// it's recorded and not descended into. Other directories are recursed when
// depth allows. It returns the error from reading dir itself; nested read errors
// are skipped silently, so only the top-level root surfaces as a bad root.
func collectRepos(dir string, depth int, seen map[string]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(child, ".git")); err == nil {
			// A repo/worktree: record it and don't descend (its own worktrees
			// and submodules aren't separate projects to scan).
			if common := gitCommonDir(child); common != "" {
				if _, ok := seen[common]; !ok {
					seen[common] = child
				}
			}
			continue
		}
		if depth > 1 {
			_ = collectRepos(child, depth-1, seen) // nested read errors: skip
		}
	}
	return nil
}

// gitCommonDir returns the absolute path to a repo's shared git directory,
// which uniquely identifies the repository across all of its worktrees.
func gitCommonDir(dir string) string {
	// Bound the call: this runs on the dashboard refresh path, and a hung git or
	// wedged filesystem must not stall it.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := gitCommandContext(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// projectName derives a friendly repo name from its common git dir.
//
//	/home/me/snag         (bare)      -> "snag"
//	/home/me/athanor/.git (non-bare)  -> "athanor"
//	/home/me/repo.git     (bare)      -> "repo"
func projectName(common string) string {
	base := filepath.Base(common)
	if base == ".git" {
		base = filepath.Base(filepath.Dir(common))
	}
	return strings.TrimSuffix(base, ".git")
}
