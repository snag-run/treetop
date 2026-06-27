package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// discoverProjects scans the given roots (one level deep) for git worktrees,
// groups them by repository, and returns one Project per repo with all of its
// worktrees. A bare repo is discovered via any of its linked worktrees, so it
// need not live under a root itself.
func discoverProjects(roots []string) ([]Project, error) {
	// Map from a repo's common git dir -> a known worktree path we can query.
	seen := map[string]string{}

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue // a missing/unreadable root is not fatal
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(root, e.Name())
			if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
				continue // not a worktree (we reach bare repos via their worktrees)
			}
			common := gitCommonDir(dir)
			if common == "" {
				continue
			}
			if _, ok := seen[common]; !ok {
				seen[common] = dir
			}
		}
	}

	var projects []Project
	for common, anyWorktree := range seen {
		wts := listWorktrees(anyWorktree)
		if len(wts) == 0 {
			continue
		}
		projects = append(projects, Project{
			Name:      projectName(common),
			Worktrees: wts,
		})
	}

	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, nil
}

// gitCommonDir returns the absolute path to a repo's shared git directory,
// which uniquely identifies the repository across all of its worktrees.
func gitCommonDir(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
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
