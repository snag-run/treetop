package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// sessionScan locates live agent sessions by inspecting /proc. It records two
// kinds of footprint per process:
//
//   - its working directory (catches a top-level `claude` running in a worktree)
//   - the regular files it currently holds open (catches in-process subagents,
//     which never chdir into the worktree they target but do read and write its
//     files — the cwd scan alone is blind to them)
//
// Detection is best-effort and Linux-only. Open-file descriptors are transient,
// so this is paired with a decay tracker (see markInUse) to keep the in-use
// marker from flickering between refreshes. The cross-platform, deterministic
// signal is the .treetop-inuse marker file (see marker.go).
type sessionScan struct {
	supported bool
	cwds      []string // resolved (symlink-free) working directories
	openFiles []string // resolved paths of files agent processes hold open
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
		if cwd := resolvedLink(filepath.Join("/proc", p.Name(), "cwd")); cwd != "" {
			scan.cwds = append(scan.cwds, cwd)
		}
		scan.openFiles = append(scan.openFiles, openFiles(p.Name())...)
	}
	return scan
}

// openFiles returns the resolved paths of regular files the process holds open,
// read from /proc/<pid>/fd. Non-file descriptors (sockets, pipes, anon inodes)
// resolve to targets like "socket:[…]" that don't begin with "/" and are
// skipped, so they never match a worktree path.
func openFiles(pid string) []string {
	fdDir := filepath.Join("/proc", pid, "fd")
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return nil // process gone or fds unreadable (not ours)
	}
	var files []string
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join(fdDir, e.Name()))
		if err != nil || !strings.HasPrefix(target, "/") {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(target); err == nil {
			target = resolved
		}
		files = append(files, target)
	}
	return files
}

// resolvedLink reads a /proc symlink and resolves it to a symlink-free path.
func resolvedLink(link string) string {
	target, err := os.Readlink(link)
	if err != nil || target == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		return resolved
	}
	return target
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
//
// Two signals feed the marker:
//
//   - The /proc scan (cwd + open files). These are observed into the decay
//     tracker so a worktree stays marked for the tracker's window after the
//     signal last appeared, smoothing over transient open-file descriptors.
//   - The .treetop-inuse marker file, which is authoritative and works on every
//     platform — so a worktree can be in use even where the /proc scan can't run.
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
