// Command treetop is a top-style live tracker for your git worktrees across
// projects: see every worktree, its branch, and which ones have a live session.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"time"
)

// version is the treetop version. Release builds override it via
// -ldflags "-X main.version=...". For `go install`-built binaries it stays
// "dev" and versionString falls back to the embedded module version.
var version = "dev"

// versionString returns the build version, preferring the ldflags-injected
// value and falling back to the VCS/module version embedded by `go install
// module@version`.
func versionString() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

const usage = `treetop - track git worktrees across projects

Usage:
  treetop [flags] [pattern...]
  treetop bug                  open a prefilled bug report (auto-fills your env)
  treetop config               inspect and edit persisted preferences (config file)
                               [path|show|menu|set <key> <value>|unset <key>]
  treetop config menu          edit preferences in an interactive menu

  [pattern] is an optional regular expression matched against project names
  (case-insensitive). Pass several patterns, or use alternation, to match more
  than one project; a project is shown if it matches any pattern.

    treetop 'snag|athanor'     # projects matching snag OR athanor
    treetop -e snag -e athanor # the same, grep-style

Flags:
  -w, --watch            live mode: refresh continuously (like top)
  -e, --regexp PATTERN   project-name pattern (repeatable; OR'd together)
  -i, --interval N       refresh interval in seconds with --watch (default 2)
  -p, --projects         collapse to one line per project (no worktrees)
  --pr                   show a PR check-status glyph per worktree (needs gh;
                         only polls when the list is filtered, max 5 projects)
  --checks               expand --pr into one row per CI check under each
                         worktree (implies --pr; full view only)
  --notify               with --watch, raise a desktop notification when a PR is
                         approved or sent back for changes, or CI fails (implies
                         --pr; same polling/gating; needs an OSC 777 terminal)
  --in-use               show only worktrees with a live session (in use)
  --open                 show only worktrees without a session (open)
  --root DIR             directory to scan for repos (repeatable; default: $HOME)
  --depth N              levels below each root to scan for repos (default 1, max 3)
  -V, --version          print version and exit
  -h, --help             show this help

Negation flags:
  Turn a config-enabled boolean off for a single run; each beats the config
  file (CLI > config). With both the positive and negative form, the last one
  on the command line wins. --no-pr cannot combine with --checks/--notify,
  which require PR data.

  --no-watch             disable live mode
  --no-pr                disable the PR check-status glyph
  --no-checks            disable per-check rows
  --no-notify            disable desktop notifications
  --no-projects          disable one-line-per-project collapsing
  --no-color             disable ANSI color

In-use detection combines a best-effort session scan (Linux via /proc, macOS via
ps+lsof: live Claude Code and Codex sessions, including subagents via open
files) with a .treetop-inuse marker file that any platform can drop. See the
README for agent hooks.
`

type options struct {
	patterns     []*regexp.Regexp
	watch        bool
	interval     int
	onlyInUse    bool
	onlyOpen     bool
	projectsOnly bool
	pr           bool
	checks       bool
	notify       bool
	roots        []string
	depth        int
	color        bool
	showVersion  bool
}

// maxScanDepth caps --depth. Repos are never descended into, so the practical
// cost of a larger depth is the directory stats along the way; the cap keeps a
// fat-fingered value from walking deep into large unrelated trees.
const maxScanDepth = 3

// stringSlice is a repeatable string flag.
type stringSlice []string

func (s *stringSlice) String() string     { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error { *s = append(*s, v); return nil }

// parseFlags parses CLI args, seeding option defaults from the config file at
// its standard location ($XDG_CONFIG_HOME/treetop/config.json, falling back to
// ~/.config/treetop/config.json).
func parseFlags(args []string) (options, error) {
	path, err := configPath("")
	if err != nil {
		// Couldn't resolve a config location (e.g. $HOME unset): treat as no
		// config rather than failing, and let flag/scan handling report any real
		// problem. An empty path makes loadConfig return defaults silently.
		path = ""
	}
	return parseFlagsWithConfig(args, path)
}

// parseFlagsWithConfig is parseFlags with the config file path injected, so
// tests can point it at a temp file without touching the real home dir.
func parseFlagsWithConfig(args []string, configFile string) (options, error) {
	fs := flag.NewFlagSet("treetop", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	var (
		watch        bool
		interval     int
		depth        int
		onlyInUse    bool
		onlyOpen     bool
		projectsOnly bool
		pr           bool
		checks       bool
		notify       bool
		noColor      bool
		showVersion  bool
		roots        stringSlice
		exprs        stringSlice
	)
	// Built-in flag defaults come from defaultConfig() so flag registration and
	// `treetop config show` derive from one struct.
	d := defaultConfig()
	// --no-color is a BoolFunc (see negate below), which has no default slot, so
	// seed the built-in here to match the old --no-color BoolVar default.
	noColor = !*d.Color
	fs.BoolVar(&watch, "watch", *d.Watch, "")
	fs.BoolVar(&watch, "w", *d.Watch, "")
	fs.BoolVar(&watch, "live", *d.Watch, "") // alias for --watch
	fs.Var(&exprs, "regexp", "")
	fs.Var(&exprs, "e", "")
	fs.IntVar(&interval, "interval", *d.Interval, "")
	fs.IntVar(&interval, "i", *d.Interval, "")
	fs.BoolVar(&onlyInUse, "in-use", false, "")
	fs.BoolVar(&onlyOpen, "open", false, "")
	fs.BoolVar(&projectsOnly, "projects", *d.Projects, "")
	fs.BoolVar(&projectsOnly, "p", *d.Projects, "")
	fs.BoolVar(&pr, "pr", *d.PR, "")
	fs.BoolVar(&checks, "checks", *d.Checks, "")
	fs.BoolVar(&notify, "notify", *d.Notify, "")
	fs.BoolVar(&showVersion, "version", false, "")
	fs.BoolVar(&showVersion, "V", false, "")
	fs.Var(&roots, "root", "")
	fs.IntVar(&depth, "depth", 1, "")

	// Negation flags turn a config-backed boolean off for a single run. Each
	// writes false into the same variable as its positive form, so flag's
	// left-to-right parse makes the last of --pr/--no-pr on the line win, and
	// flag.Visit reports the --no-* name as set — letting wasSet() count it as an
	// explicit CLI flag that beats config (CLI > config). --no-color writes the
	// inverted noColor var, mirroring the positive form's !*d.Color default.
	negate := func(name string, target *bool, val bool) {
		fs.BoolFunc(name, "", func(string) error { *target = val; return nil })
	}
	negate("no-watch", &watch, false)
	negate("no-pr", &pr, false)
	negate("no-checks", &checks, false)
	negate("no-notify", &notify, false)
	negate("no-projects", &projectsOnly, false)
	negate("no-color", &noColor, true)

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if showVersion {
		return options{showVersion: true}, nil
	}
	if onlyInUse && onlyOpen {
		return options{}, fmt.Errorf("--in-use and --open are mutually exclusive")
	}

	// Layer config under the explicitly-set flags. After Parse, each flag var
	// holds either its built-in default (flag unset) or the user's value (flag
	// set). flag.Visit walks only the flags actually set, so a config value seeds
	// any flag the user left unset — giving precedence CLI flag > config >
	// built-in default. The implied-flag rules and color resolution run after,
	// so they hold however a value arrived.
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	wasSet := func(names ...string) bool {
		for _, n := range names {
			if set[n] {
				return true
			}
		}
		return false
	}
	cfg, cfgErr := loadConfig(configFile)
	if cfgErr != nil {
		// A malformed or unreadable config is non-fatal: warn once and fall back
		// to built-in defaults, never aborting the dashboard.
		fmt.Fprintf(os.Stderr, "treetop: warning: ignoring config: %v\n", cfgErr)
		cfg = nil
	}
	if cfg != nil {
		var cw, cpr, cchecks, cnotify, cprojects, cnoColor *bool
		var cinterval *int
		if !wasSet("watch", "w", "live", "no-watch") {
			cw = &watch
		}
		if !wasSet("pr", "no-pr") {
			cpr = &pr
		}
		if !wasSet("checks", "no-checks") {
			cchecks = &checks
		}
		if !wasSet("notify", "no-notify") {
			cnotify = &notify
		}
		if !wasSet("projects", "p", "no-projects") {
			cprojects = &projectsOnly
		}
		if !wasSet("no-color") {
			cnoColor = &noColor
		}
		if !wasSet("interval", "i") {
			cinterval = &interval
		}
		applyConfig(cfg, cw, cpr, cchecks, cnotify, cprojects, cnoColor, cinterval)
	}

	// --no-pr conflicts with anything that implies --pr. --checks and --notify
	// both need PR data, so an explicitly-disabled --pr alongside an effective
	// --checks/--notify (from either their flag or config) is contradictory: error
	// rather than silently honour one over the other. Keyed on the effective PR
	// state (not just whether --no-pr was seen) so the last-wins rule for the
	// --pr/--no-pr pair still holds; gated on the same conditions the implications
	// below use, so the two stay in lockstep. Note this checks wasSet("no-pr"), not
	// "pr": --pr=false is deliberately left overridable by the implications below
	// (see TestChecksImplicationOutranksExplicitFalsePR), only an explicit --no-pr
	// is a hard conflict.
	prExplicitlyDisabled := !pr && wasSet("no-pr")
	if prExplicitlyDisabled && (checks || notify) {
		var implied []string
		if checks {
			implied = append(implied, "--checks")
		}
		if notify {
			implied = append(implied, "--notify")
		}
		return options{}, fmt.Errorf("--no-pr conflicts with %s, which require PR data", strings.Join(implied, " and "))
	}

	// --checks expands the PR column into per-check rows, so it implies --pr: the
	// same polling/gating, plus the rollup glyph each row hangs beneath. The rule
	// runs after the config merge so it holds whether checks came from a flag or
	// the config.
	if checks {
		pr = true
	}
	// --notify watches the same PR data for state changes, so it too implies --pr
	// (and rides its filter-gated polling). Notifications are raised only in watch
	// mode; outside it, --notify just behaves like --pr.
	if notify {
		pr = true
	}
	if interval < 1 {
		interval = 1
	}
	// Clamp scan depth to a sane range. 1 preserves the default one-level scan;
	// the cap keeps an accidental deep value from walking large directory trees.
	if depth < 1 {
		depth = 1
	}
	if depth > maxScanDepth {
		depth = maxScanDepth
	}
	if len(roots) == 0 {
		if home, err := os.UserHomeDir(); err == nil {
			roots = stringSlice{home}
		}
	}
	if len(roots) == 0 {
		return options{}, fmt.Errorf("could not determine a directory to scan: $HOME is unset (or unreadable) and no --root was given")
	}

	// Patterns come from -e/--regexp flags and from positional args; a project
	// is shown if its name matches any of them (grep-style OR). Matching is
	// case-insensitive.
	patterns, err := compilePatterns(append(append([]string{}, exprs...), fs.Args()...))
	if err != nil {
		return options{}, err
	}

	return options{
		patterns:     patterns,
		watch:        watch,
		interval:     interval,
		onlyInUse:    onlyInUse,
		onlyOpen:     onlyOpen,
		projectsOnly: projectsOnly,
		pr:           pr,
		checks:       checks,
		notify:       notify,
		roots:        roots,
		depth:        depth,
		color:        !noColor && useColor(),
	}, nil
}

func main() {
	args := os.Args[1:]
	// `treetop bug` is a verb, not a flag: intercept it before flag parsing and
	// open a prefilled bug report with the host environment auto-filled.
	if len(args) > 0 && args[0] == "bug" {
		if err := runBugReport(os.Stdin, os.Stdout, openBrowser); err != nil {
			fmt.Fprintln(os.Stderr, "treetop:", err)
			os.Exit(1)
		}
		return
	}
	// `treetop config` is likewise a verb: intercept it before flag parsing and
	// inspect the persisted preferences (global config file only).
	if len(args) > 0 && args[0] == "config" {
		// The path is the whole point of this command, so a failure to resolve it
		// (e.g. $HOME unset) is surfaced, not swallowed — printing an empty path
		// would defeat the discoverability the command exists for.
		path, err := configPath("")
		if err != nil {
			fmt.Fprintln(os.Stderr, "treetop:", err)
			os.Exit(1)
		}
		if err := runConfig(os.Stdout, os.Stderr, path, args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "treetop:", err)
			os.Exit(1)
		}
		return
	}

	opts, err := parseFlags(args)
	if err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "treetop:", err)
		os.Exit(2)
	}

	if opts.showVersion {
		fmt.Println("treetop", versionString())
		return
	}

	if opts.watch {
		runWatch(opts)
		return
	}
	if err := runOnce(opts); err != nil {
		fmt.Fprintln(os.Stderr, "treetop:", err)
		os.Exit(1)
	}
}

func runOnce(opts options) error {
	projects, badRoots, supported, err := collect(opts, newTracker(inUseDecay), nil)
	if err != nil {
		return err
	}
	// Warn about unreadable roots here (one-shot) rather than inside discovery:
	// watch mode calls collect every tick and printing there would corrupt the
	// live TUI.
	for _, bad := range badRoots {
		fmt.Fprintf(os.Stderr, "treetop: warning: cannot read root %s\n", bad)
	}
	r := newRenderer(os.Stdout, opts.color, opts.projectsOnly, opts.pr)
	r.checks = initialCheckView(opts.checks)
	r.filterDesc = filterDescription(opts)
	r.render(projects, supported)
	if note := unsupportedSessionNote(supported); note != "" {
		fmt.Fprintln(os.Stderr, note)
	}
	// One-shot has no live "/" filter, so PR polling rides on the CLI filters.
	if note := prHeaderNote(opts, projects, false); note != "" {
		fmt.Fprintln(os.Stderr, "treetop: "+note)
	}
	return nil
}

// filterDescription summarises the active filters for the one-shot empty
// message, so "nothing matched the filter" reads differently from "no worktrees
// exist". Returns "" when no filter is active.
func filterDescription(opts options) string {
	var parts []string
	if opts.onlyInUse {
		parts = append(parts, "--in-use")
	}
	if opts.onlyOpen {
		parts = append(parts, "--open")
	}
	if len(opts.patterns) > 0 {
		pats := make([]string, 0, len(opts.patterns))
		for _, re := range opts.patterns {
			// Patterns are compiled case-insensitive ("(?i)…"); show the source.
			pats = append(pats, fmt.Sprintf("%q", strings.TrimPrefix(re.String(), "(?i)")))
		}
		label := "pattern "
		if len(pats) > 1 {
			label = "patterns "
		}
		parts = append(parts, label+strings.Join(pats, ", "))
	}
	return strings.Join(parts, " and ")
}

// unsupportedSessionNote returns a one-line note for one-shot output explaining
// that live-session detection is unavailable on this platform (so the in-use
// column shows "?"), or "" when detection is supported. It goes to stderr so a
// piped stdout table stays clean. Watch mode shows an equivalent note in its
// footer, so this path is one-shot only.
func unsupportedSessionNote(supported bool) string {
	if supported {
		return ""
	}
	return `treetop: live-session detection is unavailable on this platform; the in-use column shows "?". Drop a .treetop-inuse marker file in a worktree to mark it in use.`
}

// inUseDecay is how long a worktree stays marked in-use after its session
// signal last appeared, smoothing over transient open file descriptors.
const inUseDecay = 30 * time.Second

// collect discovers projects, marks in-use worktrees, and applies filters. The
// tracker carries in-use decay state across refreshes (in watch mode the caller
// reuses one tracker; a fresh one makes decay a no-op for snapshots). The bool
// reports whether live session detection is supported on this platform.
//
// live is an optional extra name filter (the live-mode filter box), AND'd with
// the CLI patterns. Both are applied during discovery so filtered-out projects
// are never git-queried or walked — not just hidden after the fact.
//
// badRoots holds any roots that couldn't be read (as "<root>: <err>" strings);
// discovery is non-fatal so the other roots are still scanned. Callers on the
// watch refresh path should ignore it — printing would corrupt the live TUI.
func collect(opts options, tr *tracker, live []*regexp.Regexp) (projects []Project, badRoots []string, supported bool, err error) {
	keep := func(name string) bool { return keepName(opts.patterns, live, name) }
	projects, badRoots, err = discoverProjects(opts.roots, opts.depth, keep)
	if err != nil {
		return nil, badRoots, false, err
	}
	scan := scanSessions()
	scan.markInUse(tr, projects)
	projects = filterProjects(projects, opts)
	// PR status is opt-in (--pr) and only polled when the list is narrowed, so an
	// unfiltered $HOME scan never fans out a gh call per repo. Enrichment caps the
	// number of projects polled and caches per repo, so it's safe on the refresh
	// path; results land on the worktrees in place.
	if shouldPollPR(opts, live) {
		enrichPRChecks(projects)
	}
	return projects, badRoots, scan.supported, nil
}

// compilePatterns compiles each non-empty pattern into a case-insensitive
// regular expression. An invalid pattern is a usage error.
func compilePatterns(raw []string) ([]*regexp.Regexp, error) {
	var patterns []*regexp.Regexp
	for _, p := range raw {
		if p == "" {
			continue
		}
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", p, err)
		}
		patterns = append(patterns, re)
	}
	return patterns, nil
}

// matchesName reports whether name matches any pattern. With no patterns,
// everything matches.
func matchesName(patterns []*regexp.Regexp, name string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, re := range patterns {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

// keepName reports whether a project name survives both filters: the CLI
// patterns (cli) AND the live-mode filter box (live). Each filter matches when
// it has no patterns, so an absent filter never excludes anything. This is the
// predicate that lets discovery skip enrichment for filtered-out projects.
func keepName(cli, live []*regexp.Regexp, name string) bool {
	return matchesName(cli, name) && matchesName(live, name)
}

// filterByName keeps only projects whose name matches the patterns, reusing the
// same case-insensitive regex matching as the CLI pattern args. With no
// patterns every project is kept. Used by the live-mode filter box to narrow an
// already-collected project set on top of any CLI-launch filters.
func filterByName(projects []Project, patterns []*regexp.Regexp) []Project {
	if len(patterns) == 0 {
		return projects
	}
	out := make([]Project, 0, len(projects))
	for _, p := range projects {
		if matchesName(patterns, p.Name) {
			out = append(out, p)
		}
	}
	return out
}

// filterProjects applies the name filter and in-use/open mode, dropping
// projects that end up with no matching worktrees.
func filterProjects(projects []Project, opts options) []Project {
	var out []Project
	for _, p := range projects {
		if !matchesName(opts.patterns, p.Name) {
			continue
		}
		var wts []Worktree
		for _, w := range p.Worktrees {
			if opts.onlyInUse && !w.InUse {
				continue
			}
			if opts.onlyOpen && w.InUse {
				continue
			}
			wts = append(wts, w)
		}
		if len(wts) == 0 {
			continue
		}
		p.Worktrees = wts
		out = append(out, p)
	}
	return out
}

// useColor reports whether stdout is an interactive terminal and NO_COLOR is unset.
func useColor() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
