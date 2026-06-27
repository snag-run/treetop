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
  -w, --watch            refresh continuously (like top)
  -i, --interval N       refresh interval in seconds with --watch (default 2)
  --active               show only worktrees with a live session
  --inactive             show only worktrees without a live session
  --root DIR             directory to scan for repos (repeatable; default: $HOME)
  --no-color             disable ANSI color
  -h, --help             show this help

Active-session detection is best-effort and Linux-only (reads /proc). It finds
top-level claude sessions; it cannot see in-process subagents.
`

type options struct {
	filter     string
	watch      bool
	interval   int
	onlyActive bool
	onlyIdle   bool
	roots      []string
	color      bool
}

// stringSlice is a repeatable string flag.
type stringSlice []string

func (s *stringSlice) String() string     { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error { *s = append(*s, v); return nil }

func parseFlags(args []string) (options, error) {
	fs := flag.NewFlagSet("treetop", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	var (
		watch      bool
		interval   int
		onlyActive bool
		onlyIdle   bool
		noColor    bool
		roots      stringSlice
	)
	fs.BoolVar(&watch, "watch", false, "")
	fs.BoolVar(&watch, "w", false, "")
	fs.IntVar(&interval, "interval", 2, "")
	fs.IntVar(&interval, "i", 2, "")
	fs.BoolVar(&onlyActive, "active", false, "")
	fs.BoolVar(&onlyIdle, "inactive", false, "")
	fs.BoolVar(&noColor, "no-color", false, "")
	fs.Var(&roots, "root", "")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if onlyActive && onlyIdle {
		return options{}, fmt.Errorf("--active and --inactive are mutually exclusive")
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
		filter:     strings.Join(fs.Args(), " "),
		watch:      watch,
		interval:   interval,
		onlyActive: onlyActive,
		onlyIdle:   onlyIdle,
		roots:      roots,
		color:      !noColor && useColor(),
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
	projects, supported, err := collect(opts)
	if err != nil {
		return err
	}
	newRenderer(os.Stdout, opts.color).render(projects, supported)
	return nil
}

func runWatch(opts options) {
	r := newRenderer(os.Stdout, opts.color)
	for {
		projects, supported, err := collect(opts)
		fmt.Print("\033[H\033[2J") // home cursor + clear screen
		fmt.Printf("treetop — %s (every %ds, Ctrl-C to exit)\n\n",
			time.Now().Format("15:04:05"), opts.interval)
		if err != nil {
			fmt.Fprintln(os.Stderr, "treetop:", err)
		} else {
			r.render(projects, supported)
		}
		time.Sleep(time.Duration(opts.interval) * time.Second)
	}
}

// collect discovers projects, marks active worktrees, and applies filters.
// The bool reports whether session detection is supported on this platform.
func collect(opts options) ([]Project, bool, error) {
	projects, err := discoverProjects(opts.roots)
	if err != nil {
		return nil, false, err
	}
	scan := scanSessions()
	scan.markActive(projects)
	return filterProjects(projects, opts), scan.supported, nil
}

// filterProjects applies the name filter and active/inactive mode, dropping
// projects that end up with no matching worktrees.
func filterProjects(projects []Project, opts options) []Project {
	var out []Project
	for _, p := range projects {
		if opts.filter != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(opts.filter)) {
			continue
		}
		var wts []Worktree
		for _, w := range p.Worktrees {
			if opts.onlyActive && !w.Active {
				continue
			}
			if opts.onlyIdle && w.Active {
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
