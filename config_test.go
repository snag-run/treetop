package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes content as config.json into a fresh temp dir and returns
// the file path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestConfigPrecedence(t *testing.T) {
	// config enables watch, pr, projects, interval; the explicit --interval flag
	// must win over the config value, while the unset flags take the config.
	cfg := `{"watch":true,"pr":true,"projects":true,"interval":5}`
	path := writeConfig(t, cfg)

	opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir", "--interval", "9"}, path)
	if err != nil {
		t.Fatalf("parseFlagsWithConfig: %v", err)
	}
	if !opts.watch {
		t.Error("watch: config value not applied to unset flag")
	}
	if !opts.pr {
		t.Error("pr: config value not applied to unset flag")
	}
	if !opts.projectsOnly {
		t.Error("projects: config value not applied to unset flag")
	}
	if opts.interval != 9 {
		t.Errorf("interval = %d, want 9 (explicit flag beats config)", opts.interval)
	}
}

func TestConfigUnsetFlagDoesNotOverride(t *testing.T) {
	// pr:true in config; the user passes no --pr. The unset --pr (built-in
	// default false) must NOT clobber the config value.
	path := writeConfig(t, `{"pr":true}`)
	opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir"}, path)
	if err != nil {
		t.Fatalf("parseFlagsWithConfig: %v", err)
	}
	if !opts.pr {
		t.Error("pr: unset flag clobbered config value")
	}
}

func TestConfigBeatsBuiltinDefault(t *testing.T) {
	// interval built-in default is 2; config sets 7 and no flag is passed.
	path := writeConfig(t, `{"interval":7}`)
	opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir"}, path)
	if err != nil {
		t.Fatalf("parseFlagsWithConfig: %v", err)
	}
	if opts.interval != 7 {
		t.Errorf("interval = %d, want 7 (config beats built-in default)", opts.interval)
	}
}

func TestConfigMissingFileYieldsDefaults(t *testing.T) {
	// A missing config file is normal: no error, built-in defaults stand.
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir"}, path)
	if err != nil {
		t.Fatalf("parseFlagsWithConfig: %v", err)
	}
	if opts.pr || opts.watch || opts.interval != 2 {
		t.Errorf("missing config did not yield built-in defaults: %+v", opts)
	}
}

func TestConfigMalformedFallsBackToDefaults(t *testing.T) {
	// A malformed file is non-fatal: parseFlags succeeds with built-in defaults.
	path := writeConfig(t, `{"pr": tru`)
	opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir"}, path)
	if err != nil {
		t.Fatalf("malformed config should be non-fatal, got error: %v", err)
	}
	if opts.pr || opts.interval != 2 {
		t.Errorf("malformed config did not fall back to defaults: %+v", opts)
	}
}

func TestConfigUnknownKeysIgnored(t *testing.T) {
	// Unknown keys are ignored (forward-compatible); known keys still apply.
	path := writeConfig(t, `{"pr":true,"future_key":"whatever","depth":99}`)
	opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir"}, path)
	if err != nil {
		t.Fatalf("parseFlagsWithConfig: %v", err)
	}
	if !opts.pr {
		t.Error("pr: known key not applied alongside unknown keys")
	}
	if opts.depth != 1 {
		t.Errorf("depth = %d, want 1 (config must not set situational keys)", opts.depth)
	}
}

func TestConfigImpliesPR(t *testing.T) {
	// checks ⇒ pr and notify ⇒ pr must hold when set via config (rules run after
	// the merge).
	for _, tc := range []struct {
		name string
		cfg  string
	}{
		{"checks", `{"checks":true}`},
		{"notify", `{"notify":true}`},
	} {
		path := writeConfig(t, tc.cfg)
		opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir"}, path)
		if err != nil {
			t.Fatalf("%s: parseFlagsWithConfig: %v", tc.name, err)
		}
		if !opts.pr {
			t.Errorf("%s via config did not imply pr", tc.name)
		}
	}
}

func TestConfigColorFalseDisablesColor(t *testing.T) {
	// color:false behaves like --no-color: final color is off regardless of TTY.
	path := writeConfig(t, `{"color":false}`)
	opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir"}, path)
	if err != nil {
		t.Fatalf("parseFlagsWithConfig: %v", err)
	}
	if opts.color {
		t.Error("color: config color:false did not disable color")
	}
}

func TestApplyConfigColorMapsToNoColor(t *testing.T) {
	// color:false sets noColor true (so final color = !noColor && useColor());
	// color:true clears it. Verified at applyConfig to isolate the mapping from
	// the TTY-dependent useColor() in parseFlags.
	for _, tc := range []struct {
		color       bool
		wantNoColor bool
	}{
		{false, true},
		{true, false},
	} {
		noColor := true // start opposite to prove the config drove it
		if !tc.color {
			noColor = false
		}
		cfg := &config{Color: &tc.color}
		applyConfig(cfg, nil, nil, nil, nil, nil, &noColor, nil)
		if noColor != tc.wantNoColor {
			t.Errorf("color:%v -> noColor=%v, want %v", tc.color, noColor, tc.wantNoColor)
		}
	}
}

func TestConfigPathXDGHonored(t *testing.T) {
	// $XDG_CONFIG_HOME is honored when set.
	t.Setenv("XDG_CONFIG_HOME", "/xdg/conf")
	got, err := configPath("")
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	want := filepath.Join("/xdg/conf", "treetop", "config.json")
	if got != want {
		t.Errorf("configPath = %q, want %q", got, want)
	}
}

func TestConfigPathFallsBackToHomeConfig(t *testing.T) {
	// With XDG_CONFIG_HOME unset, fall back to ~/.config/treetop/config.json.
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := configPath("")
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	want := filepath.Join(home, ".config", "treetop", "config.json")
	if got != want {
		t.Errorf("configPath = %q, want %q", got, want)
	}
}
