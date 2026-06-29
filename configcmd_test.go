package main

import (
	"bytes"
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
