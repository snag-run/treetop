package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// sessionScan locates the working directories of live agent sessions.
//
// Detection is best-effort and Linux-only (it reads /proc). It finds top-level
// `claude` processes and maps each to its current working directory. Note this
// CANNOT see subagents: a Claude Code subagent runs in-process inside its
// parent and never chdir's into the worktree it targets, so it leaves no
// per-worktree footprint to detect.
type sessionScan struct {
	supported bool
	cwds      []string // resolved (symlink-free) working directories
}

func scanSessions() sessionScan {
	if runtime.GOOS != "linux" {
		return sessionScan{supported: false}
	}

	procs, err := os.ReadDir("/proc")
	if err != nil {
		return sessionScan{supported: false}
	}

	scan := sessionScan{supported: true}
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil {
			continue // not a pid directory
		}
		if !isAgentProcess(pid) {
			continue
		}
		cwd, err := os.Readlink(filepath.Join("/proc", p.Name(), "cwd"))
		if err != nil || cwd == "" {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
			cwd = resolved
		}
		scan.cwds = append(scan.cwds, cwd)
	}
	return scan
}

// isAgentProcess reports whether pid looks like a Claude Code session: either a
// process literally named `claude`, or a `node` process whose command line
// invokes claude (covers npm-launched installs).
func isAgentProcess(pid int) bool {
	base := filepath.Join("/proc", strconv.Itoa(pid))

	comm, err := os.ReadFile(filepath.Join(base, "comm"))
	if err != nil {
		return false
	}
	name := strings.TrimSpace(string(comm))
	switch name {
	case "claude":
		return true
	case "node":
		cmdline, err := os.ReadFile(filepath.Join(base, "cmdline"))
		if err != nil {
			return false
		}
		// cmdline args are NUL-separated.
		joined := strings.ToLower(string(bytes.ReplaceAll(cmdline, []byte{0}, []byte{' '})))
		return strings.Contains(joined, "claude")
	default:
		return false
	}
}

// markInUse flags every worktree that has a live session at or below its path.
func (s sessionScan) markInUse(projects []Project) {
	if !s.supported || len(s.cwds) == 0 {
		return
	}
	for pi := range projects {
		wts := projects[pi].Worktrees
		for wi := range wts {
			wt := filepath.Clean(wts[wi].Path)
			for _, c := range s.cwds {
				if c == wt || strings.HasPrefix(c, wt+string(filepath.Separator)) {
					wts[wi].InUse = true
					break
				}
			}
		}
	}
}
