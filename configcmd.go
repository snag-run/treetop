package main

import (
	"fmt"
	"io"
)

// configUsage is the one-line usage for the `config` verb, printed to errw on an
// unknown subaction or bad operands.
const configUsage = "usage: treetop config [path|show|set <key> <value>|unset <key>]"

// configUsageErr prints configUsage to errw and returns err, so main exits
// non-zero. Used for the argument-shape errors (wrong operand count, unknown
// subaction) where echoing the usage line helps; value/key errors from the
// set/unset helpers are returned as-is without the usage banner.
func configUsageErr(errw io.Writer, err error) error {
	_, _ = fmt.Fprintln(errw, configUsage)
	return err
}

// runConfig handles the `treetop config` verb: inspecting and editing the
// persisted preferences. configFile is the resolved config path (injected so
// tests need no real home dir). out is the data sink (os.Stdout in main, a
// buffer in tests); errw takes usage and warnings (os.Stderr in main) so they're
// assertable in tests too.
//
// Subactions: `path` prints the resolved path; `show` (also the bare default)
// prints the path then the seven effective key/value pairs; `set <key> <value>`
// and `unset <key>` edit a single key in the file. path/show take no operand.
// A bad operand count or unknown subaction writes configUsage to errw and
// returns an error so main exits non-zero. Preferences are global-only: this
// never consults the cwd or any per-project config.
func runConfig(out, errw io.Writer, configFile string, args []string) error {
	action := "show"
	var rest []string
	if len(args) > 0 {
		action, rest = args[0], args[1:]
	}
	switch action {
	case "path":
		if len(rest) > 0 {
			return configUsageErr(errw, fmt.Errorf("config path takes no arguments"))
		}
		_, err := fmt.Fprintln(out, configFile)
		return err
	case "show":
		if len(rest) > 0 {
			return configUsageErr(errw, fmt.Errorf("config show takes no arguments"))
		}
		return configShow(out, errw, configFile)
	case "set":
		return configSet(out, errw, configFile, rest)
	case "unset":
		return configUnset(out, errw, configFile, rest)
	default:
		return configUsageErr(errw, fmt.Errorf("unknown config subcommand %q", action))
	}
}

// configSet handles `config set <key> <value>`: load the existing file, set one
// key, and write it back (creating the file/dir if absent). A malformed file is
// a hard error here — silently overwriting it would discard whatever the user
// already had — so it must be fixed or removed first.
func configSet(out, errw io.Writer, configFile string, rest []string) error {
	if len(rest) != 2 {
		return configUsageErr(errw, fmt.Errorf("config set takes a key and a value"))
	}
	key, value := rest[0], rest[1]
	cfg, err := loadConfig(configFile)
	if err != nil {
		return fmt.Errorf("refusing to modify malformed config %s: %w", configFile, err)
	}
	if cfg == nil {
		cfg = &config{}
	}
	if err := setConfigValue(cfg, key, value); err != nil {
		return err
	}
	if err := saveConfig(configFile, cfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "set %s = %s\n", key, value)
	return err
}

// configUnset handles `config unset <key>`: clear one key so it reverts to the
// built-in default. When no file exists there is nothing to remove, so it
// validates the key and reports success without creating an empty file. As with
// set, a malformed file is a hard error rather than being overwritten.
func configUnset(out, errw io.Writer, configFile string, rest []string) error {
	if len(rest) != 1 {
		return configUsageErr(errw, fmt.Errorf("config unset takes a key"))
	}
	key := rest[0]
	cfg, err := loadConfig(configFile)
	if err != nil {
		return fmt.Errorf("refusing to modify malformed config %s: %w", configFile, err)
	}
	if cfg == nil {
		// No file: nothing persisted to clear. Still validate the key so a typo
		// errors, but don't write an empty file just to unset a default.
		if err := unsetConfigValue(&config{}, key); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "unset %s\n", key)
		return err
	}
	if err := unsetConfigValue(cfg, key); err != nil {
		return err
	}
	if err := saveConfig(configFile, cfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "unset %s\n", key)
	return err
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
