//go:build linux

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// scanSessions locates live agent sessions by inspecting /proc: each process's
// working directory and the regular files it holds open. Detection is
// best-effort; an unreadable /proc reports supported == false.
func scanSessions() sessionScan {
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return sessionScan{supported: false}
	}

	scan := sessionScan{supported: true}
	for _, p := range procs {
		if _, err := strconv.Atoi(p.Name()); err != nil {
			continue // not a pid directory
		}
		if !isAgentProcess(p.Name()) {
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

// isAgentProcess reports whether pid looks like a Claude Code session, reading
// /proc/<pid>/comm for the name and /proc/<pid>/cmdline for node disambiguation.
func isAgentProcess(pid string) bool {
	base := filepath.Join("/proc", pid)

	comm, err := os.ReadFile(filepath.Join(base, "comm"))
	if err != nil {
		return false
	}
	name := strings.TrimSpace(string(comm))

	var cmdline string
	if name == "node" {
		raw, err := os.ReadFile(filepath.Join(base, "cmdline"))
		if err != nil {
			return false
		}
		// cmdline args are NUL-separated.
		cmdline = string(bytes.ReplaceAll(raw, []byte{0}, []byte{' '}))
	}
	return agentName(name, cmdline)
}
