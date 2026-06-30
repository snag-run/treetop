package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestExplicitFalseFlagOverridesConfigTrue(t *testing.T) {
	// Config is a floor: an explicitly-false boolean flag must override a config
	// value that turns it on — for every config-backed boolean that has a positive
	// flag. This is the only way to disable a config-enabled boolean for a single
	// run (until --no-* flags land, #94), so pin the whole set.
	//
	// color is the exception (no positive flag, only --no-color) — its override
	// path is covered by TestApplyConfigColorMapsToNoColor.
	path := writeConfig(t, `{"watch":true,"pr":true,"checks":true,"notify":true,"projects":true}`)
	opts, err := parseFlagsWithConfig(
		[]string{"--root", "/some/dir", "-w=false", "--pr=false", "--checks=false", "--notify=false", "-p=false"},
		path,
	)
	if err != nil {
		t.Fatalf("parseFlagsWithConfig: %v", err)
	}
	for _, c := range []struct {
		name string
		on   bool
	}{
		{"watch", opts.watch},
		{"pr", opts.pr},
		{"checks", opts.checks},
		{"notify", opts.notify},
		{"projects", opts.projectsOnly},
	} {
		if c.on {
			t.Errorf("%s: explicit --%s=false did not override config true", c.name, c.name)
		}
	}
}

func TestChecksImplicationOutranksExplicitFalsePR(t *testing.T) {
	// Contract: --checks/--notify imply --pr, and that runs after the merge, so a
	// config (or flag) that turns checks on forces pr on even against --pr=false.
	// To run without pr you must also drop checks. Documented so it's not mistaken
	// for the override being broken.
	path := writeConfig(t, `{"checks":true}`)
	opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir", "--pr=false"}, path)
	if err != nil {
		t.Fatalf("parseFlagsWithConfig: %v", err)
	}
	if !opts.pr {
		t.Error("pr: --checks (from config) should imply pr even with --pr=false")
	}
}

func TestNegationFlagsBeatConfig(t *testing.T) {
	// The --no-* variants are the discoverable way to disable a config-enabled
	// boolean for a single run; each must register as explicitly set and beat the
	// config (CLI > config), mirroring the --flag=false path.
	path := writeConfig(t, `{"watch":true,"pr":true,"projects":true,"color":true}`)
	opts, err := parseFlagsWithConfig(
		[]string{"--root", "/some/dir", "--no-watch", "--no-pr", "--no-projects", "--no-color"},
		path,
	)
	if err != nil {
		t.Fatalf("parseFlagsWithConfig: %v", err)
	}
	if opts.watch {
		t.Error("watch: --no-watch did not override config true")
	}
	if opts.pr {
		t.Error("pr: --no-pr did not override config true")
	}
	if opts.projectsOnly {
		t.Error("projects: --no-projects did not override config true")
	}
	if opts.color {
		t.Error("color: --no-color did not override config color:true")
	}
}

func TestNegationFlagsChecksNotifyBeatConfig(t *testing.T) {
	// --no-checks / --no-notify on their own (no --pr implication present) must
	// disable the config-enabled booleans. pr stays on from config since nothing
	// disables it here.
	path := writeConfig(t, `{"checks":true,"notify":true,"pr":true}`)
	opts, err := parseFlagsWithConfig(
		[]string{"--root", "/some/dir", "--no-checks", "--no-notify"},
		path,
	)
	if err != nil {
		t.Fatalf("parseFlagsWithConfig: %v", err)
	}
	if opts.checks {
		t.Error("checks: --no-checks did not override config true")
	}
	if opts.notify {
		t.Error("notify: --no-notify did not override config true")
	}
	if !opts.pr {
		t.Error("pr: config true should survive --no-checks/--no-notify")
	}
}

func TestNegationLastOneWins(t *testing.T) {
	// --pr and --no-pr in the same invocation is last-one-wins (no special
	// conflict detection): both write the same variable, so flag's left-to-right
	// parse decides. Check both orderings.
	t.Run("no-pr then pr", func(t *testing.T) {
		opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir", "--no-pr", "--pr"}, "")
		if err != nil {
			t.Fatalf("parseFlagsWithConfig: %v", err)
		}
		if !opts.pr {
			t.Error("pr: trailing --pr should win over earlier --no-pr")
		}
	})
	t.Run("pr then no-pr", func(t *testing.T) {
		opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir", "--pr", "--no-pr"}, "")
		if err != nil {
			t.Fatalf("parseFlagsWithConfig: %v", err)
		}
		if opts.pr {
			t.Error("pr: trailing --no-pr should win over earlier --pr")
		}
	})
	// The conflict check keys off the effective PR state, not whether --no-pr was
	// ever seen, so a trailing --pr that wins must not trip the hard error even
	// when an implied flag (here --notify) is also on.
	t.Run("no-pr then pr with notify does not error", func(t *testing.T) {
		opts, err := parseFlagsWithConfig([]string{"--root", "/some/dir", "--notify", "--no-pr", "--pr"}, "")
		if err != nil {
			t.Fatalf("parseFlagsWithConfig: %v", err)
		}
		if !opts.pr {
			t.Error("pr: trailing --pr should win over earlier --no-pr")
		}
	})
}

func TestNoPRConflictsWithImpliedFlags(t *testing.T) {
	// --no-pr alongside anything that implies PR data (--checks/--notify, from
	// either their flag or config) is contradictory and a hard error.
	cases := []struct {
		name string
		args []string
		cfg  string
	}{
		{"flag checks", []string{"--root", "/d", "--no-pr", "--checks"}, ""},
		{"flag notify", []string{"--root", "/d", "--no-pr", "--notify"}, ""},
		{"config checks", []string{"--root", "/d", "--no-pr"}, `{"checks":true}`},
		{"config notify", []string{"--root", "/d", "--no-pr"}, `{"notify":true}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := ""
			if tc.cfg != "" {
				path = writeConfig(t, tc.cfg)
			}
			_, err := parseFlagsWithConfig(tc.args, path)
			if err == nil {
				t.Fatal("expected error for --no-pr with an implied --pr flag, got nil")
			}
			if !strings.Contains(err.Error(), "--no-pr conflicts") {
				t.Errorf("error = %q, want it to mention the --no-pr conflict", err)
			}
		})
	}
}
