package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// settingKind distinguishes the two value shapes the menu can edit.
type settingKind int

const (
	kindBool settingKind = iota
	kindInt
)

// settingRow is one editable line in the menu. The order of menuRows is the
// on-screen order and matches the fixed order used by `config show`.
type settingRow struct {
	key  string
	kind settingKind
}

var menuRows = []settingRow{
	{"watch", kindBool},
	{"pr", kindBool},
	{"checks", kindBool},
	{"notify", kindBool},
	{"projects", kindBool},
	{"color", kindBool},
	{"interval", kindInt},
}

// configMenu is the interactive editor's state: the overlay being edited (cfg,
// the same minimal subset of set keys that gets persisted) and the highlighted
// row. It is deliberately free of any terminal I/O so the whole key-handling
// behaviour is unit-testable; runConfigMenu is the thin raw-mode shell around
// it.
type configMenu struct {
	cfg    *config
	cursor int
}

// apply folds one input event into the menu, returning whether the caller should
// quit and whether cfg changed (so the shell can persist only on real edits).
//
// ↑/↓ (and k/j) move the cursor. On the selected row, → / l / space turn a bool
// on and ← / h turn it off (space toggles); for interval they step it up/down by
// one. 'r' resets the row to its built-in default (drops the key from cfg). q /
// Esc / Ctrl-C quit.
func (m *configMenu) apply(e event) (quit, changed bool) {
	switch e.kind {
	case evQuit, evEsc:
		return true, false
	case evUp:
		m.moveCursor(-1)
	case evDown:
		m.moveCursor(1)
	case evLeft:
		return false, m.adjust(-1)
	case evRight:
		return false, m.adjust(1)
	case evRune:
		switch e.ch {
		case 'q':
			return true, false
		case 'k':
			m.moveCursor(-1)
		case 'j':
			m.moveCursor(1)
		case 'h', '-':
			return false, m.adjust(-1)
		case 'l', '+':
			return false, m.adjust(1)
		case ' ':
			return false, m.toggle()
		case 'r':
			return false, m.reset()
		}
	}
	return false, false
}

func (m *configMenu) moveCursor(delta int) {
	m.cursor = clamp(m.cursor+delta, 0, len(menuRows)-1)
}

// adjust changes the selected row by delta: for a bool, delta>0 sets true and
// delta<0 sets false; for interval it steps the value by delta within range. It
// reports whether the stored value actually changed.
func (m *configMenu) adjust(delta int) bool {
	row := menuRows[m.cursor]
	eff := effectiveConfig(m.cfg)
	switch row.kind {
	case kindBool:
		return m.setBool(row.key, delta > 0)
	case kindInt:
		// Step by delta but never below 1, matching setConfigValue's contract
		// (any positive integer). No upper clamp, so an interval already above
		// what the menu would ever dial to isn't silently rewritten down.
		next := max(1, intValue(eff, row.key)+delta)
		return m.setInt(row.key, next)
	}
	return false
}

// toggle flips the selected bool row; on the interval row it is a no-op (there
// is nothing to toggle), returning false so no save is triggered.
func (m *configMenu) toggle() bool {
	row := menuRows[m.cursor]
	if row.kind != kindBool {
		return false
	}
	eff := effectiveConfig(m.cfg)
	return m.setBool(row.key, !boolValue(eff, row.key))
}

// reset drops the selected key from the overlay so it reverts to its built-in
// default. It reports whether the key was actually set (and so cleared).
func (m *configMenu) reset() bool {
	row := menuRows[m.cursor]
	if !isSet(m.cfg, row.key) {
		return false
	}
	_ = unsetConfigValue(m.cfg, row.key)
	return true
}

func (m *configMenu) setBool(key string, v bool) bool {
	if isSet(m.cfg, key) && boolValue(effectiveConfig(m.cfg), key) == v {
		return false
	}
	_ = setConfigValue(m.cfg, key, strconv.FormatBool(v))
	return true
}

func (m *configMenu) setInt(key string, v int) bool {
	if isSet(m.cfg, key) && intValue(effectiveConfig(m.cfg), key) == v {
		return false
	}
	_ = setConfigValue(m.cfg, key, strconv.Itoa(v))
	return true
}

// lines renders the menu to a slice of display lines: a title, the config path,
// then one row per setting with the selected row marked. A row using its
// built-in default (absent from the overlay) is tagged so the user can tell an
// explicit choice from an inherited one.
func (m configMenu) lines(r renderer, path string) []string {
	eff := effectiveConfig(m.cfg)
	out := []string{
		r.paint(colBold, "treetop config"),
		r.paint(colDim, strings.Repeat("─", 48)),
		r.paint(colDim, "path: "+path),
		"",
	}
	for i, row := range menuRows {
		key := fmt.Sprintf("%-9s", row.key)
		marker := "  "
		if i == m.cursor {
			marker = r.paint(colGreen, "› ")
			key = r.paint(colBold, key)
		}
		line := marker + key + "  " + valueString(eff, row.key)
		if !isSet(m.cfg, row.key) {
			line += r.paint(colDim, "   (default)")
		}
		out = append(out, line)
	}
	out = append(out,
		"",
		r.paint(colDim, "  ↑/↓ move · ←/→ change · space toggle · r reset · q quit"),
		r.paint(colDim, "  changes save immediately"),
	)
	return out
}

// boolValue / intValue / valueString read a key out of an already-merged
// effective config. They panic-proof themselves against nil only insofar as
// effectiveConfig always fills every field, which it does.
func boolValue(eff config, key string) bool {
	switch key {
	case "watch":
		return *eff.Watch
	case "pr":
		return *eff.PR
	case "checks":
		return *eff.Checks
	case "notify":
		return *eff.Notify
	case "projects":
		return *eff.Projects
	case "color":
		return *eff.Color
	}
	return false
}

func intValue(eff config, key string) int {
	if key == "interval" {
		return *eff.Interval
	}
	return 0
}

func valueString(eff config, key string) string {
	if key == "interval" {
		return strconv.Itoa(*eff.Interval)
	}
	return strconv.FormatBool(boolValue(eff, key))
}

// isSet reports whether key is explicitly present in the overlay (as opposed to
// inheriting the built-in default).
func isSet(cfg *config, key string) bool {
	if cfg == nil {
		return false
	}
	switch key {
	case "watch":
		return cfg.Watch != nil
	case "pr":
		return cfg.PR != nil
	case "checks":
		return cfg.Checks != nil
	case "notify":
		return cfg.Notify != nil
	case "projects":
		return cfg.Projects != nil
	case "color":
		return cfg.Color != nil
	case "interval":
		return cfg.Interval != nil
	}
	return false
}

// runConfigMenu is the raw-mode shell around configMenu: it puts the terminal in
// the alternate screen, reads keys via the shared input decoder, redraws on
// every event, and persists to configFile whenever a value changes. It refuses
// to run when stdin isn't a TTY (there is nothing to drive an interactive menu)
// or when the existing config file is malformed (overwriting it would discard
// the user's settings). out/errw are injected for testability; in main they are
// os.Stdout.
func runConfigMenu(out io.Writer, configFile string) error {
	cfg, err := loadConfigStrict(configFile)
	if err != nil {
		return fmt.Errorf("refusing to edit config %s: %w", configFile, err)
	}
	if cfg == nil {
		cfg = &config{}
	}

	inFd := int(os.Stdin.Fd())
	if !term.IsTerminal(inFd) {
		return fmt.Errorf("config menu needs an interactive terminal")
	}
	oldState, err := term.MakeRaw(inFd)
	if err != nil {
		return err
	}

	w := bufio.NewWriter(out)
	if _, err := fmt.Fprint(w, altScreenOn+cursorHide); err != nil {
		_ = term.Restore(inFd, oldState)
		return err
	}
	if err := w.Flush(); err != nil {
		_ = term.Restore(inFd, oldState)
		return err
	}
	// Teardown is best-effort: on the way out we can't do anything useful with a
	// write error, and the terminal restore must happen regardless.
	defer func() {
		fmt.Fprint(w, cursorShow+altScreenOff)
		w.Flush()
		term.Restore(inFd, oldState)
	}()

	// Colour follows the same rule as the dashboard: the stored preference AND an
	// interactive stdout (TTY, no NO_COLOR).
	r := newRenderer(w, *effectiveConfig(cfg).Color && useColor(), false, false)
	m := configMenu{cfg: cfg}

	events := make(chan event, 64)
	go readInput(events)

	// Turn catchable termination signals into a quit so the deferred cleanup
	// restores the terminal, mirroring runWatch.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, watchSignals...)
	defer signal.Stop(sig)
	go func() { <-sig; events <- event{kind: evQuit} }()

	// draw repaints the whole menu. Writes to the main output are required: if
	// stdout stops accepting them, abort rather than keep processing keys and
	// saving config against a frozen display.
	draw := func() error {
		if _, err := fmt.Fprint(w, clearHome); err != nil {
			return err
		}
		for _, l := range m.lines(r, configFile) {
			if _, err := fmt.Fprintf(w, "%s\r\n", l); err != nil {
				return err
			}
		}
		return w.Flush()
	}

	if err := draw(); err != nil {
		return err
	}
	for e := range events {
		quit, changed := m.apply(e)
		if changed {
			if err := saveConfig(configFile, m.cfg); err != nil {
				// Restore first (deferred cleanup runs on return), then surface why.
				return fmt.Errorf("saving config: %w", err)
			}
		}
		if quit {
			return nil
		}
		if err := draw(); err != nil {
			return err
		}
	}
	return nil
}
