package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	colReset = "\033[0m"
	colGreen = "\033[32m"
	colDim   = "\033[2m"
	colBold  = "\033[1m"
)

// renderer writes project/worktree tables, optionally with ANSI color.
type renderer struct {
	w          io.Writer
	color      bool
	compact    bool // one line per project, no worktree enumeration
	home       string
	filterDesc string // active filter description; empty means no filter
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

// sanitizeDisplay neutralises terminal control sequences that a malicious
// directory or branch name could embed. treetop renders names, paths, and refs
// that come straight from the filesystem and git, so a worktree dir named with
// an ESC sequence would otherwise inject raw escapes into the terminal (display
// spoofing, cursor hijacking). Every non-printable rune — ESC, other C0/C1
// controls, DEL — is replaced with '?'. Only the rendered copy is scrubbed; the
// stored path stays untouched so it remains usable for filesystem operations.
// ASCII space is printable and preserved.
func sanitizeDisplay(s string) string {
	clean := true
	for _, r := range s {
		if !unicode.IsPrint(r) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}

// shorten replaces the home prefix with ~ for compact, copy-pasteable paths.
func (r renderer) shorten(path string) string {
	if r.home != "" && (path == r.home || strings.HasPrefix(path, r.home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(path, r.home)
	}
	return path
}

// bodyLines renders the table to a slice of lines (no trailing newline),
// for the live dashboard to window into a scroll viewport.
func (r renderer) bodyLines(projects []Project, supported bool) []string {
	var b strings.Builder
	rb := r
	rb.w = &b
	rb.render(projects, supported)
	s := strings.TrimRight(b.String(), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// render prints the projects grouped by repo. supported indicates whether
// session detection ran (false -> the in-use marker is shown as unknown).
func (r renderer) render(projects []Project, supported bool) {
	if len(projects) == 0 {
		if r.filterDesc != "" {
			fmt.Fprintf(r.w, "No worktrees match %s.\n", r.filterDesc)
		} else {
			fmt.Fprintln(r.w, "No worktrees found.")
		}
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
			if n := len(sanitizeDisplay(r.shorten(wt.Path))); n > pathW {
				pathW = n
			}
			if n := len(sanitizeDisplay(wt.Ref())); n > refW {
				refW = n
			}
		}
	}

	// Width of the "edited …" segment, so the "changed …" segment lines up.
	editW := 0
	for _, p := range projects {
		for _, wt := range p.Worktrees {
			if n := len(editSegment(wt, now)); n > editW {
				editW = n
			}
		}
	}

	for _, p := range projects {
		fmt.Fprintln(r.w, r.paint(colBold, sanitizeDisplay(p.Name)))
		for _, wt := range p.Worktrees {
			marker := " "
			if wt.InUse {
				marker = r.paint(colGreen, "●")
			} else if !supported {
				marker = r.paint(colDim, "?")
			}
			path := sanitizeDisplay(r.shorten(wt.Path))
			times := fmt.Sprintf("%-*s · %s", editW, editSegment(wt, now), changedSegment(wt, now))
			fmt.Fprintf(r.w, "  %s %-*s  %-*s  %s\n",
				marker, pathW, path, refW, sanitizeDisplay(wt.Ref()), r.paint(colDim, times))
		}
	}
}

// editSegment renders the working-tree edit time, e.g. "edited 12s".
func editSegment(wt Worktree, now time.Time) string {
	if !wt.HasEdit {
		return "edited —"
	}
	return "edited " + humanizeShort(wt.Edited, now)
}

// changedSegment renders the git-activity time, e.g. "changed 5m".
func changedSegment(wt Worktree, now time.Time) string {
	if !wt.HasTime {
		return "changed —"
	}
	return "changed " + humanizeShort(wt.Changed, now)
}

// renderCompact prints one line per project: in-use marker, name, a
// worktree/in-use count, and the most recent change across its worktrees.
func (r renderer) renderCompact(projects []Project, supported bool) {
	now := time.Now()

	nameW := 0
	for _, p := range projects {
		if n := len(sanitizeDisplay(p.Name)); n > nameW {
			nameW = n
		}
	}

	for _, p := range projects {
		nWorktrees, nInUse := len(p.Worktrees), 0
		var edited, changed time.Time
		var hasEdit, hasChanged bool
		for _, wt := range p.Worktrees {
			if wt.InUse {
				nInUse++
			}
			if wt.HasEdit && wt.Edited.After(edited) {
				edited, hasEdit = wt.Edited, true
			}
			if wt.HasTime && wt.Changed.After(changed) {
				changed, hasChanged = wt.Changed, true
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

		// Surface the most recent edit across the project, falling back to git
		// activity when nothing has been edited on disk.
		recent := "—"
		switch {
		case hasEdit:
			recent = "edited " + humanizeShort(edited, now)
		case hasChanged:
			recent = "changed " + humanizeShort(changed, now)
		}

		name := sanitizeDisplay(p.Name)
		pad := strings.Repeat(" ", nameW-len(name))
		fmt.Fprintf(r.w, "  %s %s%s  %-13s  %s\n",
			marker, r.paint(colBold, name), pad, count, r.paint(colDim, recent))
	}
}
