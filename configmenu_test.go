package main

import (
	"strings"
	"testing"
)

// rowIndex returns the menuRows index for key, failing the test on a typo.
func rowIndex(t *testing.T, key string) int {
	t.Helper()
	for i, r := range menuRows {
		if r.key == key {
			return i
		}
	}
	t.Fatalf("no menu row for %q", key)
	return -1
}

func TestConfigMenuMoveCursorClamps(t *testing.T) {
	m := configMenu{cfg: &config{}}
	m.apply(event{kind: evUp}) // already at top
	if m.cursor != 0 {
		t.Errorf("cursor moved above top: %d", m.cursor)
	}
	for range menuRows {
		m.apply(event{kind: evDown})
	}
	if m.cursor != len(menuRows)-1 {
		t.Errorf("cursor = %d, want bottom %d", m.cursor, len(menuRows)-1)
	}
}

func TestConfigMenuToggleBool(t *testing.T) {
	m := configMenu{cfg: &config{}, cursor: rowIndex(t, "watch")}
	quit, changed := m.apply(event{kind: evRune, ch: ' '})
	if quit || !changed {
		t.Fatalf("toggle: quit=%v changed=%v, want false/true", quit, changed)
	}
	if m.cfg.Watch == nil || *m.cfg.Watch != true {
		t.Errorf("watch not toggled on: %+v", m.cfg.Watch)
	}
	// Toggling again flips back to false — still an explicit value, still a change.
	if _, changed := m.apply(event{kind: evRune, ch: ' '}); !changed {
		t.Error("second toggle reported no change")
	}
	if m.cfg.Watch == nil || *m.cfg.Watch != false {
		t.Errorf("watch not toggled off: %+v", m.cfg.Watch)
	}
}

func TestConfigMenuArrowsSetDirectional(t *testing.T) {
	m := configMenu{cfg: &config{}, cursor: rowIndex(t, "pr")}
	// → turns on; a second → is a no-op (already on).
	if _, changed := m.apply(event{kind: evRight}); !changed || m.cfg.PR == nil || !*m.cfg.PR {
		t.Fatalf("right did not set pr true: changed=%v cfg=%+v", changed, m.cfg.PR)
	}
	if _, changed := m.apply(event{kind: evRight}); changed {
		t.Error("right on an already-true bool reported a change")
	}
	// ← turns off.
	if _, changed := m.apply(event{kind: evLeft}); !changed || *m.cfg.PR {
		t.Errorf("left did not set pr false: changed=%v cfg=%+v", changed, m.cfg.PR)
	}
}

func TestConfigMenuIntervalStepsAndClamps(t *testing.T) {
	m := configMenu{cfg: &config{}, cursor: rowIndex(t, "interval")}
	// Default interval is 2; one → makes it explicit at 3.
	if _, changed := m.apply(event{kind: evRight}); !changed || *m.cfg.Interval != 3 {
		t.Fatalf("right did not step interval to 3: changed=%v cfg=%+v", changed, m.cfg.Interval)
	}
	// Step down twice: 3 -> 2 -> 1, then clamp holds at 1 (no further change).
	m.apply(event{kind: evLeft})
	m.apply(event{kind: evLeft})
	if *m.cfg.Interval != 1 {
		t.Fatalf("interval = %d, want clamped-approach 1", *m.cfg.Interval)
	}
	if _, changed := m.apply(event{kind: evLeft}); changed || *m.cfg.Interval != 1 {
		t.Errorf("interval stepped below 1: changed=%v val=%d", changed, *m.cfg.Interval)
	}
}

func TestConfigMenuIntervalToggleIsNoop(t *testing.T) {
	m := configMenu{cfg: &config{}, cursor: rowIndex(t, "interval")}
	if _, changed := m.apply(event{kind: evRune, ch: ' '}); changed {
		t.Error("space on the interval row reported a change")
	}
	if m.cfg.Interval != nil {
		t.Errorf("space wrote interval: %+v", m.cfg.Interval)
	}
}

func TestConfigMenuReset(t *testing.T) {
	tru := true
	m := configMenu{cfg: &config{Watch: &tru}, cursor: rowIndex(t, "watch")}
	if _, changed := m.apply(event{kind: evRune, ch: 'r'}); !changed {
		t.Fatal("reset of a set key reported no change")
	}
	if m.cfg.Watch != nil {
		t.Errorf("reset left watch set: %+v", m.cfg.Watch)
	}
	// Resetting an already-default key is a no-op (nothing to persist).
	if _, changed := m.apply(event{kind: evRune, ch: 'r'}); changed {
		t.Error("reset of a default key reported a change")
	}
}

func TestConfigMenuQuitKeys(t *testing.T) {
	for _, e := range []event{{kind: evQuit}, {kind: evEsc}, {kind: evRune, ch: 'q'}} {
		m := configMenu{cfg: &config{}}
		if quit, _ := m.apply(e); !quit {
			t.Errorf("event %+v did not quit", e)
		}
	}
}

func TestConfigMenuLines(t *testing.T) {
	// color:false is explicit; watch is default. Render without color so the tags
	// and values are plain to assert.
	f := false
	m := configMenu{cfg: &config{Color: &f}, cursor: rowIndex(t, "pr")}
	r := newRenderer(nil, false, false, false)
	got := strings.Join(m.lines(r, "/cfg.json"), "\n")

	for _, want := range []string{
		"path: /cfg.json",
		"watch",
		"color", // explicit, so no "(default)" tag on its line
		"interval   2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("menu lines missing %q\n---\n%s", want, got)
		}
	}
	// The cursor row (pr) is marked; color (explicit) is not tagged default but
	// watch (inherited) is.
	if !strings.Contains(got, "› pr") {
		t.Errorf("cursor marker missing on pr row:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "color") && strings.Contains(line, "(default)") {
			t.Errorf("explicit color row wrongly tagged default: %q", line)
		}
		if strings.Contains(line, "watch") && !strings.Contains(line, "(default)") {
			t.Errorf("inherited watch row missing default tag: %q", line)
		}
	}
}
