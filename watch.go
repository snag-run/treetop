package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Terminal control sequences for a flicker-free, self-restoring dashboard.
const (
	altScreenOn  = "\033[?1049h" // switch to alternate screen buffer
	altScreenOff = "\033[?1049l" // restore the primary screen (and scrollback)
	cursorHide   = "\033[?25l"
	cursorShow   = "\033[?25h"
	clearHome    = "\033[H\033[J" // move home + clear to end of screen
)

// runWatch renders a continuously-refreshing dashboard of worktrees until the
// user interrupts it (Ctrl-C). It uses the terminal's alternate screen so the
// primary scrollback is left untouched on exit.
func runWatch(opts options) {
	out := bufio.NewWriter(os.Stdout)
	enterAltScreen(out)

	// Restore the terminal on Ctrl-C / SIGTERM, then exit cleanly.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		leaveAltScreen(out)
		os.Exit(0)
	}()

	r := newRenderer(out, opts.color, opts.projectsOnly)
	ticker := time.NewTicker(time.Duration(opts.interval) * time.Second)
	defer ticker.Stop()

	for {
		projects, supported, err := collect(opts)
		fmt.Fprint(out, clearHome)
		drawHeader(out, r, opts, projects, supported)
		if err != nil {
			fmt.Fprintln(out, "  error:", err)
		} else {
			r.render(projects, supported)
		}
		out.Flush()
		<-ticker.C
	}
}

func enterAltScreen(out *bufio.Writer) {
	fmt.Fprint(out, altScreenOn+cursorHide)
	out.Flush()
}

func leaveAltScreen(out *bufio.Writer) {
	fmt.Fprint(out, cursorShow+altScreenOff)
	out.Flush()
}

// drawHeader prints the dashboard title line and a live summary of counts.
func drawHeader(out *bufio.Writer, r renderer, opts options, projects []Project, supported bool) {
	nProjects := len(projects)
	nWorktrees, nInUse := 0, 0
	for _, p := range projects {
		nWorktrees += len(p.Worktrees)
		for _, w := range p.Worktrees {
			if w.InUse {
				nInUse++
			}
		}
	}

	title := fmt.Sprintf("treetop  %s  (every %ds · Ctrl-C to exit)",
		time.Now().Format("15:04:05"), opts.interval)

	inUse := fmt.Sprintf("%d in use", nInUse)
	if supported {
		inUse = r.paint(colGreen, "● ") + inUse
	} else {
		inUse = r.paint(colDim, "? sessions unknown (Linux only)")
	}
	summary := fmt.Sprintf("%s · %s · %s",
		plural(nProjects, "project"), plural(nWorktrees, "worktree"), inUse)

	fmt.Fprintln(out, r.paint(colBold, title))
	fmt.Fprintln(out, r.paint(colDim, strings.Repeat("─", 48)))
	fmt.Fprintln(out, summary)
	fmt.Fprintln(out)
}

// plural formats a count with a naive singular/plural word.
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
