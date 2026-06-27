package main

import "time"

// Worktree is a single git worktree belonging to a Project.
type Worktree struct {
	Path     string
	Branch   string // empty when Detached or Bare
	Detached bool
	Bare     bool
	InUse    bool      // a live session (e.g. claude) is running here
	Changed  time.Time // last git activity (commit/checkout/stage)
	HasTime  bool      // whether Changed could be determined
	Edited   time.Time // newest working-tree file mtime (unstaged edits included)
	HasEdit  bool      // whether Edited could be determined
}

// Ref renders the human-readable branch / state of a worktree.
func (w Worktree) Ref() string {
	switch {
	case w.Bare:
		return "(bare)"
	case w.Detached:
		return "(detached)"
	case w.Branch != "":
		return w.Branch
	default:
		return "(unknown)"
	}
}

// Project is a git repository and all of its worktrees.
type Project struct {
	Name      string // e.g. "snag", derived from the repo directory
	Worktrees []Worktree
}

// InUse reports whether any worktree in the project has a live session.
func (p Project) InUse() bool {
	for _, w := range p.Worktrees {
		if w.InUse {
			return true
		}
	}
	return false
}
