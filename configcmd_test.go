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
	var buf bytes.Buffer
	if err := runConfig(&buf, path, []string{"path"}); err != nil {
		t.Fatalf("runConfig path: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != path {
		t.Errorf("path = %q, want %q", got, path)
	}
}

func TestRunConfigShowWithFile(t *testing.T) {
	// `config show` merges the file over the built-in defaults.
	path := writeConfig(t, `{"pr":true,"interval":7,"color":false}`)
	var buf bytes.Buffer
	if err := runConfig(&buf, path, []string{"show"}); err != nil {
		t.Fatalf("runConfig show: %v", err)
	}
	out := buf.String()
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
		if !strings.Contains(out, want) {
			t.Errorf("show output missing %q\n--- got ---\n%s", want, out)
		}
	}
	if strings.Contains(out, "(not present)") {
		t.Errorf("present file marked not-present:\n%s", out)
	}
}

func TestRunConfigShowNoFile(t *testing.T) {
	// No file: defaults stand and the path is flagged not-present.
	path := filepath.Join(t.TempDir(), "missing.json")
	var buf bytes.Buffer
	if err := runConfig(&buf, path, []string{"show"}); err != nil {
		t.Fatalf("runConfig show: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, path+" (not present)") {
		t.Errorf("missing file not flagged not-present:\n%s", out)
	}
	for _, want := range []string{
		"watch:    false",
		"pr:       false",
		"color:    true", // default on
		"interval: 2",    // default
	} {
		if !strings.Contains(out, want) {
			t.Errorf("defaults output missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestRunConfigBareIsShow(t *testing.T) {
	// Bare `config` (no subaction) behaves as `config show`.
	path := writeConfig(t, `{"watch":true}`)
	var bare, show bytes.Buffer
	if err := runConfig(&bare, path, nil); err != nil {
		t.Fatalf("runConfig bare: %v", err)
	}
	if err := runConfig(&show, path, []string{"show"}); err != nil {
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
	// A malformed file is non-fatal: show falls back to built-in defaults.
	path := writeConfig(t, `{not valid json`)
	var buf bytes.Buffer
	if err := runConfig(&buf, path, []string{"show"}); err != nil {
		t.Fatalf("runConfig show should not error on malformed file: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "interval: 2") || !strings.Contains(out, "color:    true") {
		t.Errorf("malformed file did not fall back to defaults:\n%s", out)
	}
}

func TestRunConfigUnknownSubaction(t *testing.T) {
	// An unknown subaction returns an error so main exits non-zero.
	var buf bytes.Buffer
	if err := runConfig(&buf, "/some/config.json", []string{"frobnicate"}); err == nil {
		t.Error("unknown subaction did not return an error")
	}
}
