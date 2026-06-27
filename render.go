package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	colReset = "\033[0m"
	colGreen = "\033[32m"
	colDim   = "\033[2m"
	colBold  = "\033[1m"
)

// renderer writes project/worktree tables, optionally with ANSI color.
type renderer struct {
	w       io.Writer
	color   bool
	compact bool // one line per project, no worktree enumeration
	home    string
}

func newRenderer(w io.Writer, color, compact bool) renderer {
	home, _ := os.UserHomeDir()
	return renderer{w: w, color: color, compact: compact, home: home}
}

func (r renderer) paint(c, s string) string {
	if !r.color {
		return s
	}
	return c + s + colReset
}

// shorten replaces the home prefix with ~ for compact, copy-pasteable paths.
func (r renderer) shorten(path string) string {
	if r.home != "" && (path == r.home || strings.HasPrefix(path, r.home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(path, r.home)
	}
	return path
}

// render prints the projects grouped by repo. supported indicates whether
// session detection ran (false -> the in-use marker is shown as unknown).
func (r renderer) render(projects []Project, supported bool) {
	if len(projects) == 0 {
		fmt.Fprintln(r.w, "No worktrees found.")
		return
	}
	if r.compact {
		r.renderCompact(projects, supported)
		return
	}
	now := time.Now()

	// Column widths for alignment: longest path and longest branch/ref.
	pathW, refW := 0, 0
	for _, p := range projects {
		for _, wt := range p.Worktrees {
			if n := len(r.shorten(wt.Path)); n > pathW {
				pathW = n
			}
			if n := len(wt.Ref()); n > refW {
				refW = n
			}
		}
	}

	for _, p := range projects {
		fmt.Fprintln(r.w, r.paint(colBold, p.Name))
		for _, wt := range p.Worktrees {
			marker := " "
			if wt.InUse {
				marker = r.paint(colGreen, "●")
			} else if !supported {
				marker = r.paint(colDim, "?")
			}
			path := r.shorten(wt.Path)
			changed := "—"
			if wt.HasTime {
				changed = humanizeSince(wt.Changed, now)
			}
			fmt.Fprintf(r.w, "  %s %-*s  %-*s  %s\n",
				marker, pathW, path, refW, wt.Ref(), r.paint(colDim, changed))
		}
	}
}

// renderCompact prints one line per project: in-use marker, name, a
// worktree/in-use count, and the most recent change across its worktrees.
func (r renderer) renderCompact(projects []Project, supported bool) {
	now := time.Now()

	nameW := 0
	for _, p := range projects {
		if n := len(p.Name); n > nameW {
			nameW = n
		}
	}

	for _, p := range projects {
		nWorktrees, nInUse := len(p.Worktrees), 0
		var latest time.Time
		hasTime := false
		for _, wt := range p.Worktrees {
			if wt.InUse {
				nInUse++
			}
			if wt.HasTime && wt.Changed.After(latest) {
				latest, hasTime = wt.Changed, true
			}
		}

		marker := " "
		if nInUse > 0 {
			marker = r.paint(colGreen, "●")
		} else if !supported {
			marker = r.paint(colDim, "?")
		}

		var count string
		if supported {
			count = fmt.Sprintf("%d/%d in use", nInUse, nWorktrees)
		} else {
			count = plural(nWorktrees, "worktree")
		}

		changed := "—"
		if hasTime {
			changed = humanizeSince(latest, now)
		}

		pad := strings.Repeat(" ", nameW-len(p.Name))
		fmt.Fprintf(r.w, "  %s %s%s  %-13s  %s\n",
			marker, r.paint(colBold, p.Name), pad, count, r.paint(colDim, changed))
	}
}
