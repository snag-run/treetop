package main

import "time"

// Activity is how recently work touched a worktree, on a decaying scale. It is
// driven by the tool-use heartbeat marker and the open-file scan (see markInUse),
// independent of Rooted (which tracks where a session is anchored).
type Activity int

const (
	ActIdle   Activity = iota // no work seen within the recent window
	ActRecent                 // worked within recentTTL, but not the active window
	ActActive                 // worked within activeTTL — hitting this worktree now
)

// Worktree is a single git worktree belonging to a Project.
type Worktree struct {
	Path     string
	Branch   string // empty when Detached or Bare
	Detached bool
	Bare     bool
	Rooted   bool       // a live agent process is anchored here (its working dir)
	Activity Activity   // how recently work (tool use / open files) touched here
	Changed  time.Time  // last git activity (commit/checkout/stage)
	HasTime  bool       // whether Changed could be determined
	Edited   time.Time  // newest working-tree file mtime (unstaged edits included)
	HasEdit  bool       // whether Edited could be determined
	HasPR    bool       // an open PR was found for this branch (only set with --pr)
	PRNumber int        // the open PR's number; meaningful only when HasPR
	PRURL    string     // the open PR's html URL, for the clickable PR link; meaningful only when HasPR
	PRReview PRReview   // the PR's review state, for colouring the number; meaningful only when HasPR
	Check    CheckState // rolled-up CI status of that PR; meaningful only when HasPR
	Checks   []Check    // per-check breakdown behind Check, for the --checks rows; nil otherwise
}

// InUse reports whether a live session is anchored here or actively working
// here. A worktree that was only touched recently (ActRecent) is not "in use" —
// it decays back to open once work stops.
func (w Worktree) InUse() bool {
	return w.Rooted || w.Activity == ActActive
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
		if w.InUse() {
			return true
		}
	}
	return false
}
