package main

import (
	"fmt"
	"io"
)

// configUsage is the one-line usage for the `config` verb, printed to errw on an
// unknown subaction or stray operand.
const configUsage = "usage: treetop config [path|show]"

// runConfig handles the `treetop config` verb: read-only inspection of the
// persisted preferences. configFile is the resolved config path (injected so
// tests need no real home dir). out is the data sink (os.Stdout in main, a
// buffer in tests); errw takes usage and warnings (os.Stderr in main) so they're
// assertable in tests too.
//
// Subactions: `path` prints the resolved path; `show` (also the bare default)
// prints the path then the seven effective key/value pairs. Neither takes an
// operand. An unknown subaction or stray operand writes configUsage to errw and
// returns an error so main exits non-zero. Preferences are global-only: this
// never consults the cwd or any per-project config.
func runConfig(out, errw io.Writer, configFile string, args []string) error {
	action := "show"
	if len(args) > 0 {
		action = args[0]
	}
	// path/show take no positional arguments; a stray operand is a usage error
	// rather than being silently ignored.
	if len(args) > 1 {
		_, _ = fmt.Fprintln(errw, configUsage)
		return fmt.Errorf("config %s takes no arguments", action)
	}
	switch action {
	case "path":
		_, err := fmt.Fprintln(out, configFile)
		return err
	case "show":
		return configShow(out, errw, configFile)
	default:
		_, _ = fmt.Fprintln(errw, configUsage)
		return fmt.Errorf("unknown config subcommand %q", action)
	}
}

// configShow prints the config file path (flagging it when genuinely absent),
// then the seven keys with their effective value: the built-in default overlaid
// by the file. It reflects only defaults + file — never CLI flags — and shows
// the stored color preference, not a TTY-resolved value. A malformed file is
// non-fatal: it warns once to errw and shows the built-in defaults.
//
// Write failures on out are returned (a broken pipe must not look like success);
// the errw warning is best-effort.
func configShow(out, errw io.Writer, configFile string) error {
	cfg, err := loadConfig(configFile)
	// loadConfig returns (nil, nil) only when the file does not exist; a malformed
	// file is (nil, err) and the file itself is still present.
	missing := err == nil && cfg == nil
	if err != nil {
		_, _ = fmt.Fprintf(errw, "treetop: warning: ignoring config: %v\n", err)
		cfg = nil
	}

	label := configFile
	if missing {
		label += " (not present)"
	}
	if _, err := fmt.Fprintf(out, "path: %s\n", label); err != nil {
		return err
	}

	eff := effectiveConfig(cfg)
	// Fixed order; the labels are at most 9 chars, so %-10s leaves a gap.
	rows := []struct {
		key string
		val any
	}{
		{"watch:", *eff.Watch},
		{"pr:", *eff.PR},
		{"checks:", *eff.Checks},
		{"notify:", *eff.Notify},
		{"projects:", *eff.Projects},
		{"color:", *eff.Color},
		{"interval:", *eff.Interval},
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(out, "%-10s%v\n", r.key, r.val); err != nil {
			return err
		}
	}
	return nil
}
