package main

import (
	"strings"
	"testing"
)

func TestUnsupportedSessionNote(t *testing.T) {
	if got := unsupportedSessionNote(true); got != "" {
		t.Errorf("supported platform should produce no note, got %q", got)
	}
	got := unsupportedSessionNote(false)
	if got == "" {
		t.Fatal("unsupported platform should produce a note")
	}
	if !strings.Contains(got, ".treetop-inuse") {
		t.Errorf("note should mention the .treetop-inuse marker, got %q", got)
	}
}
