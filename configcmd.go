package main

import (
	"fmt"
	"io"
	"os"
)

// configUsage is the one-line usage for the `config` verb, printed to stderr on
// an unknown subaction.
const configUsage = "usage: treetop config [path|show]"

// runConfig handles the `treetop config` verb: read-only inspection of the
// persisted preferences. configFile is the resolved config path (injected so
// tests need no real home dir); an empty string makes loadConfig yield defaults
// silently. w is the output sink (os.Stdout in main, a buffer in tests).
//
// Subactions: `path` prints the resolved path; `show` (also the bare default)
// prints the path then the seven effective key/value pairs. An unknown
// subaction writes configUsage to stderr and returns an error so main exits
// non-zero. Preferences are global-only: this never consults the cwd or any
// per-project config.
func runConfig(w io.Writer, configFile string, args []string) error {
	action := "show"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "path":
		fmt.Fprintln(w, configFile)
		return nil
	case "show":
		return configShow(w, configFile)
	default:
		fmt.Fprintln(os.Stderr, configUsage)
		return fmt.Errorf("unknown config subcommand %q", action)
	}
}

// configShow prints the config file path (flagging it when absent), then the
// seven keys with their effective value: the built-in default overlaid by the
// file. It reflects only defaults + file — never CLI flags — and shows the
// stored color preference, not a TTY-resolved value. A malformed file is
// non-fatal: it warns once to stderr and shows the built-in defaults.
func configShow(w io.Writer, configFile string) error {
	pathLabel := configFile
	if configFile == "" || !fileExists(configFile) {
		pathLabel += " (not present)"
	}
	fmt.Fprintf(w, "path: %s\n", pathLabel)

	cfg, err := loadConfig(configFile)
	if err != nil {
		// A malformed/unreadable file is non-fatal: warn once, then show the
		// built-in defaults (mirrors how parseFlags degrades).
		fmt.Fprintf(os.Stderr, "treetop: warning: ignoring config: %v\n", err)
		cfg = nil
	}
	eff := effectiveConfig(cfg)

	// Fixed order: watch, pr, checks, notify, projects, color, interval. The
	// "key:" labels are at most 9 chars ("projects:", "interval:"), so a
	// %-10s column leaves at least one space before every value.
	fmt.Fprintf(w, "%-10s%v\n", "watch:", *eff.Watch)
	fmt.Fprintf(w, "%-10s%v\n", "pr:", *eff.PR)
	fmt.Fprintf(w, "%-10s%v\n", "checks:", *eff.Checks)
	fmt.Fprintf(w, "%-10s%v\n", "notify:", *eff.Notify)
	fmt.Fprintf(w, "%-10s%v\n", "projects:", *eff.Projects)
	fmt.Fprintf(w, "%-10s%v\n", "color:", *eff.Color)
	fmt.Fprintf(w, "%-10s%v\n", "interval:", *eff.Interval)
	return nil
}

// fileExists reports whether path names an existing file (any stat error,
// including not-exist, counts as absent).
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
