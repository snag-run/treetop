package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// config holds the persisted flag defaults loaded from config.json. Every field
// is a pointer so an absent JSON key (nil) is distinguishable from a key set to
// its zero value: nil means "not in the file", so it never overrides a value
// already in place. Unknown JSON keys are ignored on purpose (no
// DisallowUnknownFields) to keep the format forward-compatible.
// The omitempty on every field is load-bearing for the write path (saveConfig):
// a nil pointer is omitted, so the persisted file stays a minimal overlay that
// lists only the keys the user has actually set. A pointer to a zero value
// (e.g. watch=false) is non-nil and is written, so `config set watch false`
// round-trips.
type config struct {
	Watch    *bool `json:"watch,omitempty"`
	PR       *bool `json:"pr,omitempty"`
	Checks   *bool `json:"checks,omitempty"`
	Notify   *bool `json:"notify,omitempty"`
	Projects *bool `json:"projects,omitempty"`
	Color    *bool `json:"color,omitempty"`
	Interval *int  `json:"interval,omitempty"`
}

// defaultConfig returns the built-in defaults for every config key
// (watch/pr/checks/notify/projects false, color on, interval 2), in one place:
// parseFlagsWithConfig registers these as its flag defaults and `treetop config
// show` reports them, so both derive from this single struct.
func defaultConfig() config {
	t, f := true, false
	two := 2
	return config{
		Watch:    &f,
		PR:       &f,
		Checks:   &f,
		Notify:   &f,
		Projects: &f,
		Color:    &t,
		Interval: &two,
	}
}

// effectiveConfig overlays the file's set keys on the built-in defaults,
// yielding the persisted preferences in force before any CLI flag. A nil cfg
// (no file) leaves the defaults untouched.
func effectiveConfig(cfg *config) config {
	eff := defaultConfig()
	if cfg == nil {
		return eff
	}
	if cfg.Watch != nil {
		eff.Watch = cfg.Watch
	}
	if cfg.PR != nil {
		eff.PR = cfg.PR
	}
	if cfg.Checks != nil {
		eff.Checks = cfg.Checks
	}
	if cfg.Notify != nil {
		eff.Notify = cfg.Notify
	}
	if cfg.Projects != nil {
		eff.Projects = cfg.Projects
	}
	if cfg.Color != nil {
		eff.Color = cfg.Color
	}
	if cfg.Interval != nil {
		eff.Interval = cfg.Interval
	}
	return eff
}

// configPath returns the path to the config file. It honours $XDG_CONFIG_HOME
// and falls back to ~/.config/treetop/config.json when it is unset or empty.
// dir, when non-empty, overrides the lookup entirely (used by tests).
func configPath(dir string) (string, error) {
	if dir != "" {
		return filepath.Join(dir, "config.json"), nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "treetop", "config.json"), nil
}

// loadConfig reads and parses the config file at path. A missing file is normal
// and returns a nil config with no error. A malformed file is non-fatal: it
// returns an error so the caller can warn once and fall back to built-in
// defaults, never aborting the dashboard (mirrors how PR-fetch failures
// degrade gracefully).
func loadConfig(path string) (*config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cfg, nil
}

// saveConfig writes cfg to path as pretty-printed JSON, creating the parent
// directory when absent. Only the keys set on cfg are written (nil pointers are
// omitted, see the config struct), so the file stays a minimal overlay of the
// built-in defaults rather than a full snapshot. Written 0o644 in a 0o755 dir,
// matching the usual dotfile permissions.
func saveConfig(path string, cfg *config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// setConfigValue parses value for the given key and assigns it on cfg. Boolean
// keys accept the strconv.ParseBool spellings (true/false/1/0/…); interval must
// be a positive integer (a non-positive refresh interval is meaningless and the
// dashboard would clamp it to 1 anyway). An unknown key is an error so a typo
// like `config set watchh true` is caught rather than silently ignored.
func setConfigValue(cfg *config, key, value string) error {
	setBool := func(dst **bool) error {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean %q for %s (want true or false)", value, key)
		}
		*dst = &b
		return nil
	}
	switch key {
	case "watch":
		return setBool(&cfg.Watch)
	case "pr":
		return setBool(&cfg.PR)
	case "checks":
		return setBool(&cfg.Checks)
	case "notify":
		return setBool(&cfg.Notify)
	case "projects":
		return setBool(&cfg.Projects)
	case "color":
		return setBool(&cfg.Color)
	case "interval":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer %q for interval", value)
		}
		if n < 1 {
			return fmt.Errorf("interval must be at least 1, got %d", n)
		}
		cfg.Interval = &n
		return nil
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
}

// unsetConfigValue clears the given key on cfg (back to nil, so it reverts to
// the built-in default). An unknown key is an error, mirroring setConfigValue.
func unsetConfigValue(cfg *config, key string) error {
	switch key {
	case "watch":
		cfg.Watch = nil
	case "pr":
		cfg.PR = nil
	case "checks":
		cfg.Checks = nil
	case "notify":
		cfg.Notify = nil
	case "projects":
		cfg.Projects = nil
	case "color":
		cfg.Color = nil
	case "interval":
		cfg.Interval = nil
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

// applyConfig seeds option defaults from the config, for keys present in the
// file. It runs after built-in defaults and before explicitly-set flags, so the
// precedence is CLI flag > config > built-in default. Implied-flag rules
// (checks/notify ⇒ pr) and the color resolution are applied by the caller after
// the explicit-flag layer.
func applyConfig(cfg *config, watch, pr, checks, notify, projectsOnly, noColor *bool, interval *int) {
	if cfg == nil {
		return
	}
	// A nil target means the user set that flag explicitly, so config must not
	// touch it (CLI flag > config). Only seed targets the caller left non-nil.
	if cfg.Watch != nil && watch != nil {
		*watch = *cfg.Watch
	}
	if cfg.PR != nil && pr != nil {
		*pr = *cfg.PR
	}
	if cfg.Checks != nil && checks != nil {
		*checks = *cfg.Checks
	}
	if cfg.Notify != nil && notify != nil {
		*notify = *cfg.Notify
	}
	if cfg.Projects != nil && projectsOnly != nil {
		*projectsOnly = *cfg.Projects
	}
	// config color:false behaves like --no-color; color:true clears the
	// preference. The final color value still respects useColor() (TTY/NO_COLOR),
	// resolved by the caller.
	if cfg.Color != nil && noColor != nil {
		*noColor = !*cfg.Color
	}
	if cfg.Interval != nil && interval != nil {
		*interval = *cfg.Interval
	}
}
