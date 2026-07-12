package main

import (
	"path"
	"path/filepath"
	"strings"
)

// A sessionScan is a best-effort snapshot of where live agent sessions are
// working, used to light up in-use worktrees without any cooperation from the
// session itself. It records two kinds of footprint per process:
//
//   - its working directory (catches a top-level `claude` running in a worktree)
//   - the regular files it currently holds open (catches in-process subagents,
//     which never chdir into the worktree they target but do read and write its
//     files — the cwd scan alone is blind to them)
//
// How the snapshot is gathered is platform-specific (see session_linux.go and
// session_darwin.go); platforms without an implementation report supported ==
// false. Open-file descriptors are transient, so this is paired with a decay
// tracker (see markInUse) to keep the in-use marker from flickering between
// refreshes. The cross-platform, deterministic signal is the .treetop-inuse
// marker file (see marker.go), which works even where no scan is available.
type sessionScan struct {
	supported bool
	cwds      []string // resolved (symlink-free) working directories
	openFiles []string // resolved paths of files agent processes hold open
}

// agentName reports whether a process named comm with the given command line
// looks like a supported agent session. Native Claude Code and Codex binaries
// are matched by process name; node-launched installs are matched by command
// line (covers npm-launched wrappers). comm is the executable's base name;
// cmdline is its full, space-joined command line (may be empty when unavailable).
func agentName(comm, cmdline string) bool {
	switch comm {
	case "claude", "codex":
		return true
	case "node":
		return nodeAgentCommandLine(cmdline)
	default:
		return false
	}
}

func nodeAgentCommandLine(cmdline string) bool {
	for _, token := range strings.Fields(strings.ToLower(cmdline)) {
		if nodeAgentToken(token) {
			return true
		}
	}
	return false
}

func nodeAgentToken(token string) bool {
	token = strings.Trim(token, `"'`)
	token = strings.ReplaceAll(token, `\`, `/`)
	if strings.Contains(token, "claude-code") || strings.Contains(token, "@anthropic-ai/claude-code") {
		return true
	}
	if strings.Contains(token, "@openai/codex") {
		return true
	}

	base := path.Base(token)
	switch base {
	case "claude", "claude.js", "claude.cmd", "codex", "codex.js", "codex.cmd":
		return true
	case "cli.js":
		parent := path.Base(path.Dir(token))
		return parent == "claude" || parent == "claude-code" || parent == "codex"
	default:
		return false
	}
}

// markInUse sets each worktree's Rooted flag and Activity tier from three
// signals, kept distinct so an anchored-but-idle session reads differently from
// one that is actively working:
//
//   - Rooted: an agent process is anchored here (its cwd, from the scan),
//     smoothed over inUseDecay via the root tracker.
//   - Activity: work is touching this worktree — the .treetop-inuse heartbeat
//     marker, or an open file the scan caught (work tracker). ActActive within
//     activeTTL, ActRecent within recentTTL, else ActIdle. The marker works on
//     every platform, so activity shows even where the scan can't run.
func (s sessionScan) markInUse(trs *trackers, projects []Project) {
	for pi := range projects {
		wts := projects[pi].Worktrees
		for wi := range wts {
			path := wts[wi].Path
			wt := filepath.Clean(path)
			if resolved, err := filepath.EvalSymlinks(wt); err == nil {
				wt = resolved
			}
			if containsUnder(s.cwds, wt) {
				trs.root.observe(wt)
			}
			if containsUnder(s.openFiles, wt) {
				trs.work.observe(wt)
			}

			wts[wi].Rooted = trs.root.within(wt, inUseDecay)
			switch {
			case markerActiveWithin(path, activeTTL) || trs.work.within(wt, inUseDecay):
				wts[wi].Activity = ActActive
			case markerActiveWithin(path, recentTTL) || trs.work.within(wt, recentTTL):
				wts[wi].Activity = ActRecent
			default:
				wts[wi].Activity = ActIdle
			}
		}
	}
}

// containsUnder reports whether any path equals dir or lives beneath it.
func containsUnder(paths []string, dir string) bool {
	prefix := dir + string(filepath.Separator)
	for _, p := range paths {
		if p == dir || strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}
