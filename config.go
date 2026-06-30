package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// config holds the persisted flag defaults loaded from config.json. Every field
// is a pointer so an absent JSON key (nil) is distinguishable from a key set to
// its zero value: nil means "not in the file", so it never overrides a value
// already in place. Unknown JSON keys are ignored on purpose (no
// DisallowUnknownFields) to keep the format forward-compatible.
type config struct {
	Watch    *bool `json:"watch"`
	PR       *bool `json:"pr"`
	Checks   *bool `json:"checks"`
	Notify   *bool `json:"notify"`
	Projects *bool `json:"projects"`
	Color    *bool `json:"color"`
	Interval *int  `json:"interval"`
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
