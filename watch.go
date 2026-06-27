package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

// Terminal control sequences for a flicker-free, self-restoring dashboard.
const (
	altScreenOn  = "\033[?1049h" // switch to alternate screen buffer
	altScreenOff = "\033[?1049l" // restore the primary screen (and scrollback)
	cursorHide   = "\033[?25l"
	cursorShow   = "\033[?25h"
	clearHome    = "\033[H\033[J"           // move home + clear to end of screen
	mouseOn      = "\033[?1000h\033[?1006h" // button + SGR-encoded mouse reporting
	mouseOff     = "\033[?1006l\033[?1000l"
)

// event is a scroll/quit action derived from keyboard or mouse input.
type event int

const (
	evQuit event = iota
	evUp
	evDown
	evWheelUp
	evWheelDown
	evPageUp
	evPageDown
	evTop
	evBottom
)

const wheelLines = 3 // lines scrolled per mouse-wheel notch

// runWatch renders a continuously-refreshing, scrollable dashboard until the
// user quits (q / Ctrl-C). It uses the terminal's alternate screen and raw
// input so it can scroll and so keystrokes don't echo onto the display.
func runWatch(opts options) {
	inFd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(inFd)
	if err != nil {
		// Not a TTY (e.g. piped): fall back to a plain refresh loop.
		runWatchPlain(opts)
		return
	}

	out := bufio.NewWriter(os.Stdout)
	fmt.Fprint(out, altScreenOn+cursorHide+mouseOn)
	out.Flush()

	cleanup := func() {
		fmt.Fprint(out, mouseOff+cursorShow+altScreenOff)
		out.Flush()
		term.Restore(inFd, oldState)
	}
	defer cleanup()

	events := make(chan event, 16)
	go readInput(events)

	// SIGTERM still arrives in raw mode (Ctrl-C does not — it's read as a byte).
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sig; events <- evQuit }()

	r := newRenderer(out, opts.color, opts.projectsOnly)
	ticker := time.NewTicker(time.Duration(opts.interval) * time.Second)
	defer ticker.Stop()

	offset := 0
	var viewport, maxOffset int

	draw := func() {
		projects, supported, derr := collect(opts)
		_, rows, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || rows <= 0 {
			rows = 24
		}

		header := headerLines(r, opts, projects, supported)
		viewport = rows - len(header) - 1 // reserve one row for the footer
		if viewport < 1 {
			viewport = 1
		}

		var body []string
		if derr != nil {
			body = []string{"  error: " + derr.Error()}
		} else {
			body = r.bodyLines(projects, supported)
		}

		maxOffset = len(body) - viewport
		if maxOffset < 0 {
			maxOffset = 0
		}
		offset = clamp(offset, 0, maxOffset)
		end := min(offset+viewport, len(body))

		fmt.Fprint(out, clearHome)
		for _, l := range header {
			fmt.Fprintf(out, "%s\r\n", l)
		}
		for _, l := range body[offset:end] {
			fmt.Fprintf(out, "%s\r\n", l)
		}
		fmt.Fprint(out, scrollFooter(r, offset, len(body), viewport))
		out.Flush()
	}

	draw()
	for {
		select {
		case <-ticker.C:
			draw()
		case e := <-events:
			switch e {
			case evQuit:
				return
			case evUp:
				offset--
			case evDown:
				offset++
			case evWheelUp:
				offset -= wheelLines
			case evWheelDown:
				offset += wheelLines
			case evPageUp:
				offset -= viewport
			case evPageDown:
				offset += viewport
			case evTop:
				offset = 0
			case evBottom:
				offset = maxOffset
			}
			draw()
		}
	}
}

// readInput translates raw keyboard and SGR-mouse bytes into scroll events.
func readInput(ch chan<- event) {
	br := bufio.NewReader(os.Stdin)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return
		}
		switch b {
		case 'q', 3: // q or Ctrl-C
			ch <- evQuit
		case 'j':
			ch <- evDown
		case 'k':
			ch <- evUp
		case 'g':
			ch <- evTop
		case 'G':
			ch <- evBottom
		case ' ':
			ch <- evPageDown
		case 0x1b: // escape sequence
			if b2, err := br.ReadByte(); err != nil {
				return
			} else if b2 != '[' {
				continue
			}
			b3, err := br.ReadByte()
			if err != nil {
				return
			}
			switch b3 {
			case 'A':
				ch <- evUp
			case 'B':
				ch <- evDown
			case 'H':
				ch <- evTop
			case 'F':
				ch <- evBottom
			case '5':
				br.ReadByte() // consume '~'
				ch <- evPageUp
			case '6':
				br.ReadByte() // consume '~'
				ch <- evPageDown
			case '<': // SGR mouse: ESC [ < Cb ; Cx ; Cy (M|m)
				if e, ok := parseMouse(br); ok {
					ch <- e
				}
			}
		}
	}
}

// parseMouse reads an SGR mouse sequence body and returns a wheel event.
func parseMouse(br *bufio.Reader) (event, bool) {
	var sb []byte
	for {
		c, err := br.ReadByte()
		if err != nil {
			return 0, false
		}
		if c == 'M' || c == 'm' {
			break
		}
		sb = append(sb, c)
	}
	code := string(sb)
	if i := strings.IndexByte(code, ';'); i >= 0 {
		code = code[:i]
	}
	switch code {
	case "64":
		return evWheelUp, true
	case "65":
		return evWheelDown, true
	}
	return 0, false
}

// scrollFooter renders the bottom status line (no trailing newline so the
// alternate screen doesn't scroll by one row).
func scrollFooter(r renderer, offset, total, viewport int) string {
	if total == 0 {
		return r.paint(colDim, "  no worktrees")
	}
	last := min(offset+viewport, total)
	return r.paint(colDim, fmt.Sprintf("  rows %d–%d of %d   ↑/↓ · PgUp/PgDn · g/G · q quit",
		offset+1, last, total))
}

// runWatchPlain is the non-interactive fallback (stdin is not a TTY): a simple
// clear-and-reprint refresh loop with no scrolling.
func runWatchPlain(opts options) {
	out := bufio.NewWriter(os.Stdout)
	r := newRenderer(out, opts.color, opts.projectsOnly)
	ticker := time.NewTicker(time.Duration(opts.interval) * time.Second)
	defer ticker.Stop()
	for {
		projects, supported, err := collect(opts)
		fmt.Fprint(out, clearHome)
		for _, l := range headerLines(r, opts, projects, supported) {
			fmt.Fprintln(out, l)
		}
		if err != nil {
			fmt.Fprintln(out, "  error:", err)
		} else {
			r.render(projects, supported)
		}
		out.Flush()
		<-ticker.C
	}
}

// headerLines builds the dashboard title and a live summary of counts.
func headerLines(r renderer, opts options, projects []Project, supported bool) []string {
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

	title := fmt.Sprintf("treetop  %s  (every %ds)", time.Now().Format("15:04:05"), opts.interval)

	inUse := fmt.Sprintf("%d in use", nInUse)
	if supported {
		inUse = r.paint(colGreen, "● ") + inUse
	} else {
		inUse = r.paint(colDim, "? sessions unknown (Linux only)")
	}
	summary := fmt.Sprintf("%s · %s · %s",
		plural(nProjects, "project"), plural(nWorktrees, "worktree"), inUse)

	return []string{
		r.paint(colBold, title),
		r.paint(colDim, strings.Repeat("─", 48)),
		summary,
		"",
	}
}

// plural formats a count with a naive singular/plural word.
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
