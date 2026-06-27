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
	w     io.Writer
	color bool
	home  string
}

func newRenderer(w io.Writer, color bool) renderer {
	home, _ := os.UserHomeDir()
	return renderer{w: w, color: color, home: home}
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
