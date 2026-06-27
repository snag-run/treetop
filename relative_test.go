package main

import (
	"testing"
	"time"
)

func TestHumanizeSince(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, "just now"},
		{1 * time.Minute, "1 minute ago"},
		{5 * time.Minute, "5 minutes ago"},
		{1 * time.Hour, "1 hour ago"},
		{3 * time.Hour, "3 hours ago"},
		{25 * time.Hour, "1 day ago"},
		{50 * time.Hour, "2 days ago"},
		{8 * 24 * time.Hour, "1 week ago"},
		{40 * 24 * time.Hour, "1 month ago"},
		{400 * 24 * time.Hour, "1 year ago"},
	}
	for _, tt := range tests {
		if got := humanizeSince(now.Add(-tt.ago), now); got != tt.want {
			t.Errorf("humanizeSince(%s ago) = %q, want %q", tt.ago, got, tt.want)
		}
	}
}

func TestHumanizeShort(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		ago  time.Duration
		want string
	}{
		{-5 * time.Second, "now"}, // future mtime / clock skew
		{500 * time.Millisecond, "now"},
		{12 * time.Second, "12s"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{2 * 24 * time.Hour, "2d"},
		{3 * 7 * 24 * time.Hour, "3w"},
		{40 * 24 * time.Hour, "1mo"},
		{400 * 24 * time.Hour, "1y"},
	}
	for _, tt := range tests {
		if got := humanizeShort(now.Add(-tt.ago), now); got != tt.want {
			t.Errorf("humanizeShort(%s ago) = %q, want %q", tt.ago, got, tt.want)
		}
	}
}
