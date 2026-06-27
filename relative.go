package main

import (
	"fmt"
	"time"
)

// humanizeSince renders a duration as natural relative wording, e.g.
// "just now", "5 minutes ago", "1 hour ago", "2 days ago", "3 weeks ago".
func humanizeSince(t time.Time, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < 0:
		return "just now" // clock skew / future mtime
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return agoUnit(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return agoUnit(int(d.Hours()), "hour")
	case d < 7*24*time.Hour:
		return agoUnit(int(d.Hours()/24), "day")
	case d < 30*24*time.Hour:
		return agoUnit(int(d.Hours()/(24*7)), "week")
	case d < 365*24*time.Hour:
		return agoUnit(int(d.Hours()/(24*30)), "month")
	default:
		return agoUnit(int(d.Hours()/(24*365)), "year")
	}
}

func agoUnit(n int, unit string) string {
	if n <= 1 {
		return fmt.Sprintf("1 %s ago", unit)
	}
	return fmt.Sprintf("%d %ss ago", n, unit)
}

// humanizeShort renders a duration as a compact relative token — "now", "12s",
// "5m", "3h", "2d", "3w", "4mo", "1y" — for the side-by-side edited/changed
// columns where the long-form wording would be too wide. Unlike humanizeSince
// it keeps sub-minute resolution, since recent edits are the interesting case.
func humanizeShort(t time.Time, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Second:
		return "now" // also covers clock skew / future mtime
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}
