// Command treetop is a top-style live tracker for your git worktrees across
// projects: see every worktree, its branch, and which ones have a live session.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

const usage = `treetop - track git worktrees across projects

Usage:
  treetop [flags] [pattern...]

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
  --in-use               show only worktrees with a live session (in use)
  --open                 show only worktrees without a session (open)
  --root DIR             directory to scan for repos (repeatable; default: $HOME)
  --no-color             disable ANSI color
  -h, --help             show this help

In-use detection combines a best-effort session scan (Linux via /proc, macOS via
ps+lsof: live claude sessions, including subagents via open files) with a
.treetop-inuse marker file that any platform can drop. See the README for the
subagent hooks.
`

type options struct {
	patterns     []*regexp.Regexp
	watch        bool
	interval     int
	onlyInUse    bool
	onlyOpen     bool
	projectsOnly bool
	roots        []string
	color        bool
}

// stringSlice is a repeatable string flag.
type stringSlice []string

func (s *stringSlice) String() string     { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error { *s = append(*s, v); return nil }

func parseFlags(args []string) (options, error) {
	fs := flag.NewFlagSet("treetop", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	var (
		watch        bool
		interval     int
		onlyInUse    bool
		onlyOpen     bool
		projectsOnly bool
		noColor      bool
		roots        stringSlice
		exprs        stringSlice
	)
	fs.BoolVar(&watch, "watch", false, "")
	fs.BoolVar(&watch, "w", false, "")
	fs.BoolVar(&watch, "live", false, "") // alias for --watch
	fs.Var(&exprs, "regexp", "")
	fs.Var(&exprs, "e", "")
	fs.IntVar(&interval, "interval", 2, "")
	fs.IntVar(&interval, "i", 2, "")
	fs.BoolVar(&onlyInUse, "in-use", false, "")
	fs.BoolVar(&onlyOpen, "open", false, "")
	fs.BoolVar(&projectsOnly, "projects", false, "")
	fs.BoolVar(&projectsOnly, "p", false, "")
	fs.BoolVar(&noColor, "no-color", false, "")
	fs.Var(&roots, "root", "")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if onlyInUse && onlyOpen {
		return options{}, fmt.Errorf("--in-use and --open are mutually exclusive")
	}
	if interval < 1 {
		interval = 1
	}
	if len(roots) == 0 {
		if home, err := os.UserHomeDir(); err == nil {
			roots = stringSlice{home}
		}
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
		roots:        roots,
		color:        !noColor && useColor(),
	}, nil
}

func main() {
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "treetop:", err)
		os.Exit(2)
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
	projects, supported, err := collect(opts, newTracker(inUseDecay), nil)
	if err != nil {
		return err
	}
	newRenderer(os.Stdout, opts.color, opts.projectsOnly).render(projects, supported)
	return nil
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
func collect(opts options, tr *tracker, live []*regexp.Regexp) ([]Project, bool, error) {
	keep := func(name string) bool { return keepName(opts.patterns, live, name) }
	projects, err := discoverProjects(opts.roots, keep)
	if err != nil {
		return nil, false, err
	}
	scan := scanSessions()
	scan.markInUse(tr, projects)
	return filterProjects(projects, opts), scan.supported, nil
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
