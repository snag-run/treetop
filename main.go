// Command treetop is a top-style live tracker for your git worktrees across
// projects: see every worktree, its branch, and which ones have a live session.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

const usage = `treetop - track git worktrees across projects

Usage:
  treetop [flags] [filter]

  [filter] is an optional substring matched against project names.

Flags:
  -w, --watch            live mode: refresh continuously (like top)
  -i, --interval N       refresh interval in seconds with --watch (default 2)
  -p, --projects         collapse to one line per project (no worktrees)
  --in-use               show only worktrees with a live session (in use)
  --open                 show only worktrees without a session (open)
  --root DIR             directory to scan for repos (repeatable; default: $HOME)
  --no-color             disable ANSI color
  -h, --help             show this help

In-use detection combines a best-effort /proc scan (Linux-only: live claude
sessions, including subagents via open files) with a .treetop-inuse marker file
that any platform can drop. See the README for the subagent hooks.
`

type options struct {
	filter       string
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
	)
	fs.BoolVar(&watch, "watch", false, "")
	fs.BoolVar(&watch, "w", false, "")
	fs.BoolVar(&watch, "live", false, "") // alias for --watch
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

	return options{
		filter:       strings.Join(fs.Args(), " "),
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
	projects, supported, err := collect(opts, newTracker(inUseDecay))
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
// reports whether /proc session detection is supported on this platform.
func collect(opts options, tr *tracker) ([]Project, bool, error) {
	projects, err := discoverProjects(opts.roots)
	if err != nil {
		return nil, false, err
	}
	scan := scanSessions()
	scan.markInUse(tr, projects)
	return filterProjects(projects, opts), scan.supported, nil
}

// filterProjects applies the name filter and in-use/open mode, dropping
// projects that end up with no matching worktrees.
func filterProjects(projects []Project, opts options) []Project {
	var out []Project
	for _, p := range projects {
		if opts.filter != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(opts.filter)) {
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
