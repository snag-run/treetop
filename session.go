package main

import (
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
	lower := strings.ToLower(cmdline)
	return strings.Contains(lower, "claude") ||
		strings.Contains(lower, "@openai/codex") ||
		strings.Contains(lower, "/codex/") ||
		strings.Contains(lower, `\codex\`)
}

// markInUse flags every worktree that has a live session at or below its path.
//
// Two signals feed the marker:
//
//   - The session scan (cwd + open files). These are observed into the decay
//     tracker so a worktree stays marked for the tracker's window after the
//     signal last appeared, smoothing over transient open-file descriptors.
//   - The .treetop-inuse marker file, which is authoritative and works on every
//     platform — so a worktree can be in use even where the scan can't run.
func (s sessionScan) markInUse(tr *tracker, projects []Project) {
	for pi := range projects {
		wts := projects[pi].Worktrees
		for wi := range wts {
			wt := filepath.Clean(wts[wi].Path)
			if resolved, err := filepath.EvalSymlinks(wt); err == nil {
				wt = resolved
			}
			if containsUnder(s.cwds, wt) || containsUnder(s.openFiles, wt) {
				tr.observe(wt)
			}
			wts[wi].InUse = markerActive(wts[wi].Path) || tr.active(wt)
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
