package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

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

// eventKind is a scroll/quit/edit action derived from keyboard or mouse input.
type eventKind int

const (
	evQuit eventKind = iota
	evUp
	evDown
	evWheelUp
	evWheelDown
	evPageUp
	evPageDown
	evTop
	evBottom
	evRune      // a printable character (ch is set)
	evEnter     // Return
	evEsc       // Escape
	evBackspace // Backspace / Delete
)

// event is a single decoded input. For evRune, ch holds the typed character;
// the main loop decides whether a rune is a scroll command or filter text based
// on whether the filter box is open, so the input reader stays mode-agnostic.
type event struct {
	kind eventKind
	ch   rune
}

// watchSignals are the trappable termination signals that runWatch turns into a
// graceful quit so the terminal (alternate screen, cursor, raw mode) is
// restored on the way out. SIGKILL is deliberately absent — it can't be caught.
var watchSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}

const wheelLines = 3 // lines scrolled per mouse-wheel notch

// escSeqTimeout is how long to wait for the rest of a possibly-fragmented escape
// sequence before deciding a lone ESC byte was a standalone Escape keypress.
const escSeqTimeout = 50 * time.Millisecond

// snapshot is the latest collected dashboard data, delivered by the refresh
// goroutine. Rendering (and live filtering) happens on the main loop so a typed
// filter re-windows the cached snapshot instantly without waiting for a tick.
type snapshot struct {
	projects  []Project
	supported bool
	err       error
	startedAt time.Time // when this refresh's collection began, for the staleness age
}

// runWatch renders a continuously-refreshing, scrollable dashboard until the
// user quits (q / Ctrl-C). It uses the terminal's alternate screen and raw
// input so it can scroll and so keystrokes don't echo onto the display.
//
// Data collection (git + /proc scan) runs on a background goroutine so a slow
// scan never blocks scrolling or quitting; the input loop only ever touches the
// last cached frame, which is cheap to re-window and redraw.
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

	events := make(chan event, 64)
	go readInput(events)

	// Catch the signals that would otherwise terminate the process with the
	// alternate screen still active, and turn them into a graceful quit so the
	// deferred cleanup restores the terminal. SIGINT via Ctrl-C doesn't arrive
	// here (raw mode reads it as a byte), but `kill -INT` does; SIGHUP fires when
	// the terminal window closes or a multiplexer detaches; SIGQUIT is Ctrl-\.
	// SIGKILL is untrappable, so a `kill -9` or hard crash can still leave the
	// alt screen up — run `reset` (or `tput rmcup`) to recover.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, watchSignals...)
	go func() { <-sig; events <- event{kind: evQuit} }()

	r := newRenderer(out, opts.color, opts.projectsOnly, opts.pr)
	r.checks = opts.checks

	snaps := make(chan snapshot, 1)
	queries := make(chan string, 1)
	done := make(chan struct{})
	go refreshLoop(opts, queries, snaps, done)
	defer close(done)

	cur := snapshot{}
	loading := true
	var lastUpdate time.Time // when cur last changed, for the staleness indicator
	offset := 0
	var viewport, maxOffset int

	// Live-filter state. filtering is true while the filter box is open for
	// editing; query persists (and stays applied) after Enter until cleared.
	// The box is disabled when CLI grep flags are in use — the filter is
	// already pinned at launch, so an in-TUI filter would just be confusing.
	filterable := len(opts.patterns) == 0
	filtering := false
	query := ""

	// pushQuery hands the current filter to the refresh goroutine so it stops
	// collecting filtered-out projects (drop-stale so a fast typist never blocks).
	pushQuery := func() {
		select {
		case <-queries:
		default:
		}
		select {
		case queries <- query:
		default:
		}
	}

	// view applies the live filter to the latest snapshot, returning the
	// header lines, body lines, and whether the current query is a valid regex.
	view := func() (header, body []string, valid bool) {
		if loading {
			return nil, []string{r.paint(colDim, "  loading…")}, true
		}
		projects := cur.projects
		patterns, err := compilePatterns([]string{query})
		valid = err == nil
		if valid {
			projects = filterByName(projects, patterns)
		}
		header = headerLines(r, opts, projects, cur.supported, time.Since(lastUpdate), query != "")
		switch {
		case cur.err != nil:
			body = []string{"  error: " + cur.err.Error()}
		default:
			body = r.bodyLines(projects, cur.supported)
		}
		return header, body, valid
	}

	render := func() {
		header, body, valid := view()

		_, rows, gerr := term.GetSize(int(os.Stdout.Fd()))
		if gerr != nil || rows <= 0 {
			rows = 24
		}
		viewport = rows - len(header) - 1 // reserve one row for the footer
		if viewport < 1 {
			viewport = 1
		}
		maxOffset = max(0, len(body)-viewport)
		offset = clamp(offset, 0, maxOffset)
		end := min(offset+viewport, len(body))

		fmt.Fprint(out, clearHome)
		for _, l := range header {
			fmt.Fprintf(out, "%s\r\n", l)
		}
		for _, l := range body[offset:end] {
			fmt.Fprintf(out, "%s\r\n", l)
		}
		fmt.Fprint(out, watchFooter(r, footerState{
			offset: offset, total: len(body), viewport: viewport,
			filterable: filterable, filtering: filtering, query: query, validQuery: valid,
		}))
		out.Flush()
	}

	// applyEvent updates scroll/filter state; it returns true when the user quit.
	applyEvent := func(e event) bool {
		if filtering {
			// In filter mode, printable keys edit the query; only a few keys
			// (Enter/Esc/Ctrl-C and the scroll arrows/wheel) act as commands.
			switch e.kind {
			case evQuit:
				return true
			case evEnter:
				filtering = false // keep the query applied
			case evEsc:
				filtering, query = false, "" // cancel and clear
			case evBackspace:
				query = trimLastRune(query)
			case evRune:
				query += string(e.ch)
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
			return false
		}

		switch e.kind {
		case evQuit:
			return true
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
		case evEsc:
			query = "" // clear an applied filter
		case evRune:
			switch e.ch {
			case 'q':
				return true
			case 'j':
				offset++
			case 'k':
				offset--
			case 'g':
				offset = 0
			case 'G':
				offset = maxOffset
			case ' ':
				offset += viewport
			case '/':
				if filterable {
					filtering = true // open the filter box
				}
			}
		}
		return false
	}

	// A slow refresh can't block this loop (refreshLoop runs separately), so tick
	// once a second to keep the clock and the staleness indicator live even when
	// no snapshot or input arrives — otherwise an overdue refresh would go
	// unnoticed on a frozen frame.
	ui := time.NewTicker(time.Second)
	defer ui.Stop()

	render()
	for {
		select {
		case cur = <-snaps:
			loading = false
			lastUpdate = cur.startedAt
			render()
		case <-ui.C:
			render()
		case e := <-events:
			prevQuery := query
			if applyEvent(e) {
				return
			}
			// Drain any queued events (e.g. a mouse-wheel burst) before
			// redrawing, so quit can't sit behind them and we render once.
			for drained := false; !drained; {
				select {
				case e := <-events:
					if applyEvent(e) {
						return
					}
				default:
					drained = true
				}
			}
			if query != prevQuery {
				pushQuery() // tell the collector to stop walking filtered-out projects
			}
			render()
		}
	}
}

// refreshLoop collects data on the configured interval and delivers the latest
// snapshot to the main loop, dropping any stale undelivered snapshot so a slow
// consumer never makes the producer block.
//
// The active filter query arrives on queries; refreshLoop applies it during
// discovery so filtered-out projects are never git-queried or walked, and
// re-collects immediately when the query changes so broadening the filter
// repopulates without waiting for the next tick. An invalid (partial) regex is
// treated as no filter, so data stays available while the user is mid-type.
func refreshLoop(opts options, queries <-chan string, out chan snapshot, done <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(opts.interval) * time.Second)
	defer ticker.Stop()

	// One tracker for the session, so in-use decay carries across refreshes.
	tr := newTracker(inUseDecay)
	query := ""

	emit := func() {
		live, err := compilePatterns([]string{query})
		if err != nil {
			live = nil // invalid regex: don't narrow the collected set
		}
		// Stamp when collection began, not when it finishes, so the staleness
		// age reflects true data age: a slow-but-steady collect (each refresh
		// running longer than the interval) still ages past the threshold.
		started := time.Now()
		projects, _, supported, cerr := collect(opts, tr, live)
		s := snapshot{projects: projects, supported: supported, err: cerr, startedAt: started}
		select { // drop a stale pending snapshot, then deliver the fresh one
		case <-out:
		default:
		}
		select {
		case out <- s:
		case <-done:
		}
	}

	emit()
	for {
		select {
		case <-ticker.C:
			emit()
		case q := <-queries:
			if q != query {
				query = q
				emit() // re-collect now so the filtered set updates immediately
			}
		case <-done:
			return
		}
	}
}

// trimLastRune drops the final rune of s (correctly handling multi-byte runes).
func trimLastRune(s string) string {
	if s == "" {
		return ""
	}
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

// readInput decodes raw keyboard and SGR-mouse bytes into events. Printable
// characters become evRune so the main loop can treat them as scroll commands or
// filter text depending on mode; Ctrl-C always quits. The reader is deliberately
// mode-agnostic.
func readInput(ch chan<- event) {
	br := bufio.NewReader(os.Stdin)
	send := func(k eventKind) { ch <- event{kind: k} }
	for {
		b, err := br.ReadByte()
		if err != nil {
			return
		}
		switch {
		case b == 3: // Ctrl-C always quits, even in filter mode
			send(evQuit)
		case b == '\r' || b == '\n':
			send(evEnter)
		case b == 0x7f || b == 0x08: // Backspace / Delete
			send(evBackspace)
		case b == 0x1b: // Escape, alone or as a CSI sequence prefix
			if b2, ok := nextEscByte(br); ok {
				readEscape(b2, br, ch)
			} else {
				send(evEsc) // nothing followed within the window: a real Escape
			}
		case b >= 0x20 && b < 0x7f: // printable ASCII -> rune
			ch <- event{kind: evRune, ch: rune(b)}
		case b >= 0xc0: // start of a multi-byte UTF-8 rune
			if r, ok := readRune(br, b); ok {
				ch <- event{kind: evRune, ch: r}
			}
		}
	}
}

// nextEscByte returns the byte following a just-read ESC, distinguishing a CSI
// sequence (arrows, mouse) from a standalone Escape keypress. The remaining
// sequence bytes usually arrive together, so a non-empty buffer means a
// sequence; otherwise we wait briefly, since a sequence can be fragmented across
// reads on slow links (e.g. SSH) and a premature decision would mis-read an
// arrow key as Esc. If read deadlines aren't supported we fall back to the
// buffer check, which never blocks on a true Escape.
func nextEscByte(br *bufio.Reader) (byte, bool) {
	if br.Buffered() > 0 {
		b, err := br.ReadByte()
		return b, err == nil
	}
	if err := os.Stdin.SetReadDeadline(time.Now().Add(escSeqTimeout)); err != nil {
		return 0, false // deadlines unsupported: treat as a standalone Escape
	}
	defer os.Stdin.SetReadDeadline(time.Time{})
	b, err := br.ReadByte()
	return b, err == nil
}

// readEscape parses the body of a CSI escape sequence (the leading ESC and the
// following byte b2 already consumed) into a scroll/mouse event. An ESC not
// followed by '[' is treated as a standalone Escape.
func readEscape(b2 byte, br *bufio.Reader, ch chan<- event) {
	if b2 != '[' {
		ch <- event{kind: evEsc}
		return
	}
	b3, err := br.ReadByte()
	if err != nil {
		return
	}
	switch b3 {
	case 'A':
		ch <- event{kind: evUp}
	case 'B':
		ch <- event{kind: evDown}
	case 'H':
		ch <- event{kind: evTop}
	case 'F':
		ch <- event{kind: evBottom}
	case '5':
		br.ReadByte() // consume '~'
		ch <- event{kind: evPageUp}
	case '6':
		br.ReadByte() // consume '~'
		ch <- event{kind: evPageDown}
	case '<': // SGR mouse: ESC [ < Cb ; Cx ; Cy (M|m)
		if e, ok := parseMouse(br); ok {
			ch <- e
		}
	}
}

// readRune reads the continuation bytes of a UTF-8 rune whose lead byte is b.
func readRune(br *bufio.Reader, b byte) (rune, bool) {
	n := 1
	switch {
	case b >= 0xf0:
		n = 4
	case b >= 0xe0:
		n = 3
	case b >= 0xc0:
		n = 2
	}
	buf := make([]byte, n)
	buf[0] = b
	for i := 1; i < n; i++ {
		c, err := br.ReadByte()
		if err != nil {
			return 0, false
		}
		buf[i] = c
	}
	r, size := utf8.DecodeRune(buf)
	if r == utf8.RuneError && size <= 1 {
		return 0, false
	}
	return r, true
}

// parseMouse reads an SGR mouse sequence body and returns a wheel event.
func parseMouse(br *bufio.Reader) (event, bool) {
	var sb []byte
	for {
		c, err := br.ReadByte()
		if err != nil {
			return event{}, false
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
		return event{kind: evWheelUp}, true
	case "65":
		return event{kind: evWheelDown}, true
	}
	return event{}, false
}

// footerState is the input to the bottom status line.
type footerState struct {
	offset, total, viewport int
	filterable              bool   // the live filter box is available (no CLI grep flags)
	filtering               bool   // the filter box is open for editing
	query                   string // active filter text (may be empty)
	validQuery              bool   // whether query compiles as a regex
}

// watchFooter renders the bottom status line (no trailing newline so the
// alternate screen doesn't scroll by one row). While the filter box is open it
// shows the editable query and a cursor; otherwise it shows the scroll status,
// with any applied filter appended.
func watchFooter(r renderer, s footerState) string {
	if s.filtering {
		line := "  /" + s.query + "█" // block cursor
		if !s.validQuery {
			line += "  (invalid regex)"
		} else {
			line += "   enter apply · esc clear"
		}
		return r.paint(colDim, line)
	}
	if s.total == 0 {
		line := "  no matches"
		if s.query == "" {
			line = "  no worktrees"
		}
		return r.paint(colDim, line)
	}
	last := min(s.offset+s.viewport, s.total)
	keys := "↑/↓ · PgUp/PgDn · g/G · q quit"
	if s.filterable {
		keys = "↑/↓ · PgUp/PgDn · g/G · / filter · q quit"
	}
	line := fmt.Sprintf("  rows %d–%d of %d   %s", s.offset+1, last, s.total, keys)
	if s.query != "" {
		line += "   [/" + s.query + "]"
	}
	return r.paint(colDim, line)
}

// runWatchPlain is the non-interactive fallback (stdin is not a TTY): a simple
// clear-and-reprint refresh loop with no scrolling.
func runWatchPlain(opts options) {
	out := bufio.NewWriter(os.Stdout)
	r := newRenderer(out, opts.color, opts.projectsOnly, opts.pr)
	r.checks = opts.checks
	ticker := time.NewTicker(time.Duration(opts.interval) * time.Second)
	defer ticker.Stop()
	tr := newTracker(inUseDecay)
	for {
		projects, _, supported, err := collect(opts, tr, nil)
		fmt.Fprint(out, clearHome)
		// collect ran synchronously just now, so the data is fresh (age 0). No live
		// "/" filter in the plain (non-TTY) loop, so PR polling rides on CLI flags.
		for _, l := range headerLines(r, opts, projects, supported, 0, false) {
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

// headerLines builds the dashboard title and a live summary of counts. age is
// how long since the displayed snapshot was collected; when it runs well past
// the refresh interval the title flags the data as stale, so an overdue or
// stalled refresh isn't mistaken for fresh data.
func headerLines(r renderer, opts options, projects []Project, supported bool, age time.Duration, prLiveActive bool) []string {
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
	// Flag staleness once the data is older than two refresh intervals (a missed
	// tick), so normal jitter under one interval stays quiet. Appended plain so
	// it inherits the bold title rather than nesting color escapes.
	if stale := 2 * time.Duration(opts.interval) * time.Second; age >= stale {
		title += fmt.Sprintf("  · stale %ds", int(age.Seconds()))
	}

	inUse := fmt.Sprintf("%d in use", nInUse)
	if supported {
		inUse = r.paint(colGreen, "● ") + inUse
	} else {
		inUse = r.paint(colDim, "? sessions unknown (Linux/macOS only)")
	}
	summary := fmt.Sprintf("%s · %s · %s",
		plural(nProjects, "project"), plural(nWorktrees, "worktree"), inUse)

	lines := []string{
		r.paint(colBold, title),
		r.paint(colDim, strings.Repeat("─", 48)),
		summary,
	}
	if note := prHeaderNote(opts, projects, prLiveActive); note != "" {
		lines = append(lines, r.paint(colDim, note))
	}
	return append(lines, "")
}

// prHeaderNote returns a one-line status for the --pr feature, or "" when --pr is
// off or there's nothing to say. It explains why glyphs are absent (polling is
// dormant until the list is filtered) or partial (more projects matched than the
// poll cap), so the empty/short column never looks like a bug. liveActive is
// whether the watch-mode "/" filter currently holds a query.
func prHeaderNote(opts options, projects []Project, liveActive bool) string {
	if !opts.pr {
		return ""
	}
	if !prFilterActive(opts, liveActive) {
		return "PR checks: filter the list (pattern, /, --in-use/--open) to enable"
	}
	// Polling is active: if gh itself is unusable, say so — otherwise a blank
	// column looks like "no PRs" when it really means "gh can't answer".
	if note := ghProblemNote(); note != "" {
		return note
	}
	if n := len(projects); n > maxPRPollProjects {
		return fmt.Sprintf("PR checks: first %d of %d projects — narrow further for the rest", maxPRPollProjects, n)
	}
	return ""
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
