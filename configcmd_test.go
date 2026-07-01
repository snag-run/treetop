package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConfigPath(t *testing.T) {
	// `config path` prints the resolved path verbatim, present or not.
	path := filepath.Join(t.TempDir(), "config.json")
	var out, errw bytes.Buffer
	if err := runConfig(&out, &errw, path, []string{"path"}); err != nil {
		t.Fatalf("runConfig path: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != path {
		t.Errorf("path = %q, want %q", got, path)
	}
}

func TestRunConfigShowWithFile(t *testing.T) {
	// `config show` merges the file over the built-in defaults.
	path := writeConfig(t, `{"pr":true,"interval":7,"color":false}`)
	var out, errw bytes.Buffer
	if err := runConfig(&out, &errw, path, []string{"show"}); err != nil {
		t.Fatalf("runConfig show: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"path: " + path + "\n", // present: no "(not present)"
		"watch:    false",
		"pr:       true", // from file
		"checks:   false",
		"notify:   false",
		"projects: false",
		"color:    false", // stored preference, from file
		"interval: 7",     // from file
	} {
		if !strings.Contains(got, want) {
			t.Errorf("show output missing %q\n--- got ---\n%s", want, got)
		}
	}
	if strings.Contains(got, "(not present)") {
		t.Errorf("present file marked not-present:\n%s", got)
	}
}

func TestRunConfigShowNoFile(t *testing.T) {
	// No file: defaults stand and the path is flagged not-present.
	path := filepath.Join(t.TempDir(), "missing.json")
	var out, errw bytes.Buffer
	if err := runConfig(&out, &errw, path, []string{"show"}); err != nil {
		t.Fatalf("runConfig show: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, path+" (not present)") {
		t.Errorf("missing file not flagged not-present:\n%s", got)
	}
	for _, want := range []string{
		"watch:    false",
		"pr:       false",
		"color:    true", // default on
		"interval: 2",    // default
	} {
		if !strings.Contains(got, want) {
			t.Errorf("defaults output missing %q\n--- got ---\n%s", want, got)
		}
	}
}

func TestRunConfigBareIsShow(t *testing.T) {
	// Bare `config` (no subaction) behaves as `config show`.
	path := writeConfig(t, `{"watch":true}`)
	var bare, show, errw bytes.Buffer
	if err := runConfig(&bare, &errw, path, nil); err != nil {
		t.Fatalf("runConfig bare: %v", err)
	}
	if err := runConfig(&show, &errw, path, []string{"show"}); err != nil {
		t.Fatalf("runConfig show: %v", err)
	}
	if bare.String() != show.String() {
		t.Errorf("bare config differs from show:\nbare=%q\nshow=%q", bare.String(), show.String())
	}
	if !strings.Contains(bare.String(), "watch:    true") {
		t.Errorf("bare config did not apply file watch:true:\n%s", bare.String())
	}
}

func TestRunConfigShowMalformedFileWarnsAndDefaults(t *testing.T) {
	// A malformed file is non-fatal: show warns to errw and falls back to the
	// built-in defaults.
	path := writeConfig(t, `{not valid json`)
	var out, errw bytes.Buffer
	if err := runConfig(&out, &errw, path, []string{"show"}); err != nil {
		t.Fatalf("runConfig show should not error on malformed file: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "interval: 2") || !strings.Contains(got, "color:    true") {
		t.Errorf("malformed file did not fall back to defaults:\n%s", got)
	}
	if !strings.Contains(errw.String(), "ignoring config") {
		t.Errorf("malformed file did not warn to errw:\n%s", errw.String())
	}
}

func TestRunConfigUnknownSubaction(t *testing.T) {
	// An unknown subaction returns an error and prints usage to errw.
	var out, errw bytes.Buffer
	if err := runConfig(&out, &errw, "/some/config.json", []string{"frobnicate"}); err == nil {
		t.Error("unknown subaction did not return an error")
	}
	if !strings.Contains(errw.String(), configUsage) {
		t.Errorf("unknown subaction did not print usage to errw:\n%s", errw.String())
	}
}

func TestRunConfigRejectsTrailingOperand(t *testing.T) {
	// path/show take no positional args; a stray operand is a usage error, not
	// silently ignored.
	for _, args := range [][]string{{"show", "junk"}, {"path", "junk"}} {
		var out, errw bytes.Buffer
		if err := runConfig(&out, &errw, "/some/config.json", args); err == nil {
			t.Errorf("%v did not return an error for the stray operand", args)
		}
		if !strings.Contains(errw.String(), configUsage) {
			t.Errorf("%v did not print usage to errw:\n%s", args, errw.String())
		}
	}
}

func TestRunConfigSetWritesFile(t *testing.T) {
	// `config set` on a missing file creates it holding just the set key, and a
	// follow-up show reflects it. A bool zero value must persist (not be omitted).
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	var out, errw bytes.Buffer
	if err := runConfig(&out, &errw, path, []string{"set", "interval", "9"}); err != nil {
		t.Fatalf("runConfig set: %v", err)
	}
	if !strings.Contains(out.String(), "set interval = 9") {
		t.Errorf("set did not confirm to out:\n%s", out.String())
	}
	cfg, err := loadConfig(path)
	if err != nil || cfg == nil || cfg.Interval == nil || *cfg.Interval != 9 {
		t.Fatalf("file not written with interval=9: cfg=%+v err=%v", cfg, err)
	}
	// Only the set key is persisted; untouched keys stay absent (nil overlay).
	if cfg.Watch != nil || cfg.PR != nil {
		t.Errorf("set persisted keys other than interval: %+v", cfg)
	}

	// A second set on an existing file preserves the first key and adds the new
	// one, including a bool false (which must not be omitted).
	out.Reset()
	if err := runConfig(&out, &errw, path, []string{"set", "watch", "false"}); err != nil {
		t.Fatalf("runConfig set watch: %v", err)
	}
	cfg, err = loadConfig(path)
	if err != nil || cfg.Interval == nil || *cfg.Interval != 9 {
		t.Fatalf("second set dropped interval: cfg=%+v err=%v", cfg, err)
	}
	if cfg.Watch == nil || *cfg.Watch != false {
		t.Errorf("set watch false did not persist as false: %+v", cfg)
	}
}

func TestRunConfigSetInvalidValues(t *testing.T) {
	// Bad values and unknown keys are rejected without writing the file.
	cases := []struct{ key, value string }{
		{"watch", "yep"},     // not a bool
		{"interval", "soon"}, // not an int
		{"interval", "0"},    // out of range
		{"bogus", "true"},    // unknown key
	}
	for _, tc := range cases {
		path := filepath.Join(t.TempDir(), "config.json")
		var out, errw bytes.Buffer
		if err := runConfig(&out, &errw, path, []string{"set", tc.key, tc.value}); err == nil {
			t.Errorf("set %s %s: expected error", tc.key, tc.value)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("set %s %s wrote a file despite the error", tc.key, tc.value)
		}
	}
}

func TestRunConfigSetBadArgCount(t *testing.T) {
	// set needs exactly a key and a value; a bad count is a usage error.
	for _, args := range [][]string{{"set"}, {"set", "watch"}, {"set", "watch", "true", "extra"}} {
		var out, errw bytes.Buffer
		if err := runConfig(&out, &errw, "/some/config.json", args); err == nil {
			t.Errorf("%v: expected a usage error", args)
		}
		if !strings.Contains(errw.String(), configUsage) {
			t.Errorf("%v did not print usage to errw:\n%s", args, errw.String())
		}
	}
}

func TestRunConfigUnsetClearsKey(t *testing.T) {
	// `config unset` removes just the named key, leaving the rest intact.
	path := writeConfig(t, `{"pr":true,"interval":7}`)
	var out, errw bytes.Buffer
	if err := runConfig(&out, &errw, path, []string{"unset", "pr"}); err != nil {
		t.Fatalf("runConfig unset: %v", err)
	}
	if !strings.Contains(out.String(), "unset pr") {
		t.Errorf("unset did not confirm to out:\n%s", out.String())
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.PR != nil {
		t.Errorf("unset pr left pr set: %+v", cfg)
	}
	if cfg.Interval == nil || *cfg.Interval != 7 {
		t.Errorf("unset pr disturbed interval: %+v", cfg)
	}
}

func TestRunConfigUnsetNoFileNoWrite(t *testing.T) {
	// Unsetting a valid key with no file is a no-op success that creates nothing.
	path := filepath.Join(t.TempDir(), "config.json")
	var out, errw bytes.Buffer
	if err := runConfig(&out, &errw, path, []string{"unset", "watch"}); err != nil {
		t.Fatalf("runConfig unset: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("unset created a file when none existed")
	}
	// An unknown key still errors even without a file.
	if err := runConfig(&out, &errw, path, []string{"unset", "bogus"}); err == nil {
		t.Error("unset of unknown key did not error")
	}
}

func TestRunConfigSetRefusesMalformedFile(t *testing.T) {
	// set/unset must not overwrite a file they can't parse — that would discard
	// the user's existing settings.
	path := writeConfig(t, `{not valid json`)
	var out, errw bytes.Buffer
	if err := runConfig(&out, &errw, path, []string{"set", "watch", "true"}); err == nil {
		t.Error("set on malformed file did not error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{not valid json` {
		t.Errorf("malformed file was overwritten: %q", data)
	}
}
