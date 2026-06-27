package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
// session detection ran (false -> the active column is shown as unknown).
func (r renderer) render(projects []Project, supported bool) {
	if len(projects) == 0 {
		fmt.Fprintln(r.w, "No worktrees found.")
		return
	}

	// Width of the longest shortened path, for branch-column alignment.
	width := 0
	for _, p := range projects {
		for _, wt := range p.Worktrees {
			if n := len(r.shorten(wt.Path)); n > width {
				width = n
			}
		}
	}

	for _, p := range projects {
		fmt.Fprintln(r.w, r.paint(colBold, p.Name))
		for _, wt := range p.Worktrees {
			marker := " "
			if wt.Active {
				marker = r.paint(colGreen, "●")
			} else if !supported {
				marker = r.paint(colDim, "?")
			}
			path := r.shorten(wt.Path)
			fmt.Fprintf(r.w, "  %s %-*s  %s\n", marker, width, path, r.paint(colDim, wt.Ref()))
		}
	}
}
