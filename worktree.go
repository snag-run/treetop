package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// listWorktrees returns every worktree of the repo that `dir` belongs to,
// parsed from `git worktree list --porcelain` and enriched with last-activity.
func listWorktrees(dir string) []Worktree {
	// Bound the call: this runs on the dashboard refresh path, and a hung git or
	// wedged filesystem must not stall it.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := gitCommandContext(ctx, dir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	wts := parseWorktrees(out)
	for i := range wts {
		wts[i].Changed, wts[i].HasTime = lastActivity(wts[i].Path)
		wts[i].Edited, wts[i].HasEdit = newestEdit(wts[i].Path)
	}
	return wts
}

// editCacheTTL bounds how often newestEdit re-walks a worktree. The dashboard
// refreshes every couple of seconds; a few seconds of staleness on the "edited"
// column is invisible but spares a full directory walk on every tick.
const editCacheTTL = 4 * time.Second

// editCacheStale bounds how long an entry survives without being recomputed.
// A present worktree refreshes its entry every editCacheTTL, so anything older
// than several TTLs belongs to a worktree that has vanished; sweeping those out
// keeps editCache roughly the size of the active worktree set rather than
// growing monotonically across a long watch session.
const editCacheStale = 10 * editCacheTTL

type editEntry struct {
	at      time.Time // when this result was computed
	mtime   time.Time
	hasTime bool
}

var (
	editCacheMu sync.Mutex
	editCache   = map[string]editEntry{}
)

// newestEdit returns the most recent modification time of any file in the
// worktree, ignoring the .git directory and anything git ignores (build output,
// vendored deps, etc.). This reflects actual file edits — including unstaged
// ones — which lastActivity (git metadata only) cannot see. Results are cached
// per worktree for editCacheTTL.
func newestEdit(worktreePath string) (time.Time, bool) {
	now := time.Now()

	editCacheMu.Lock()
	if e, ok := editCache[worktreePath]; ok && now.Sub(e.at) < editCacheTTL {
		editCacheMu.Unlock()
		return e.mtime, e.hasTime
	}
	editCacheMu.Unlock()

	mtime, hasTime := walkNewest(worktreePath)

	editCacheMu.Lock()
	editCache[worktreePath] = editEntry{at: now, mtime: mtime, hasTime: hasTime}
	// Evict entries for worktrees that have vanished: they stop being refreshed
	// and would otherwise leak. The map tracks the active worktree set, so this
	// sweep is cheap.
	for path, e := range editCache {
		if now.Sub(e.at) > editCacheStale {
			delete(editCache, path)
		}
	}
	editCacheMu.Unlock()

	return mtime, hasTime
}

// Defense-in-depth bounds for walkNewest. A pathologically large or deep
// worktree must not stall the dashboard refresh, so the walk gives up after
// walkBudget of wall-clock time or walkMaxEntries entries visited (whichever
// comes first) and returns the newest mtime found so far. The "edited" column
// is approximate by design, so a partial result is acceptable.
const (
	walkBudget     = 2 * time.Second
	walkMaxEntries = 200_000
	// walkDeadlineCheckEvery throttles time.Now() calls: checking the clock on
	// every entry is a measurable cost on large trees, so check every N entries.
	walkDeadlineCheckEvery = 1024
)

// walkNewest walks the worktree, pruning .git and git-ignored directories, and
// returns the newest file mtime found. The walk is bounded by walkBudget and
// walkMaxEntries; see walkNewestBounded.
func walkNewest(worktreePath string) (time.Time, bool) {
	return walkNewestBounded(worktreePath, time.Now().Add(walkBudget), walkMaxEntries)
}

// walkNewestBounded is the core of walkNewest, with the wall-clock deadline and
// the max-entries cap injected so tests can exercise tiny budgets. On hitting a
// bound it stops via filepath.SkipAll and returns the newest mtime found so far
// (a partial-but-valid result).
func walkNewestBounded(worktreePath string, deadline time.Time, maxEntries int) (time.Time, bool) {
	ignored := ignoredDirs(worktreePath)

	var latest time.Time
	found := false
	entries := 0
	filepath.WalkDir(worktreePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry: skip it, keep walking
		}
		if d.IsDir() {
			if path != worktreePath && (d.Name() == ".git" || ignored[path]) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == markerName {
			return nil // our own in-use marker isn't a working-tree edit
		}
		if fi, err := d.Info(); err == nil {
			if mt := fi.ModTime(); mt.After(latest) {
				latest, found = mt, true
			}
		}
		// Bound the walk after recording this file's mtime, so the entry that
		// trips the bound still contributes: a partial newest-mtime is fine, but
		// the walk must return regardless of worktree size or depth.
		entries++
		if maxEntries > 0 && entries >= maxEntries {
			return filepath.SkipAll
		}
		// time.Now() per entry is a measurable cost on large trees, so only
		// check the deadline every walkDeadlineCheckEvery files.
		if entries%walkDeadlineCheckEvery == 0 && time.Now().After(deadline) {
			return filepath.SkipAll
		}
		return nil
	})
	return latest, found
}

// ignoredDirs returns the set of absolute directory paths git ignores in the
// worktree, so walkNewest can prune them. Files git ignores at the leaf level
// still get walked; pruning whole directories (node_modules, target, dist) is
// what keeps the walk cheap and the mtime meaningful.
func ignoredDirs(worktreePath string) map[string]bool {
	// Bound the call: this runs on the dashboard refresh path, and a hung git or
	// wedged filesystem must not stall it.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := gitCommandContext(ctx, worktreePath,
		"ls-files", "--others", "--ignored", "--exclude-standard", "--directory").Output()
	if err != nil {
		return nil // not a git repo, git unavailable, or timed out: prune only .git
	}
	dirs := map[string]bool{}
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasSuffix(line, "/") {
			continue // --directory lists ignored dirs with a trailing slash
		}
		dirs[filepath.Join(worktreePath, line)] = true
	}
	return dirs
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
