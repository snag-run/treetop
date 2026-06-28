package main

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// markerName is the lockfile a session hook drops into a
// worktree to declare it in use. This is the deterministic, cross-platform
// counterpart to the best-effort /proc scan: it works on every OS and, unlike
// the cwd scan, it sees in-process subagents because the hook — not treetop —
// reports the activity.
//
// The file may be empty, or its first line may hold the owning process PID:
//
//	echo $PPID > <worktree>/.treetop-inuse   # on SubagentStart / PreToolUse
//	rm -f       <worktree>/.treetop-inuse     # on SubagentStop
//
// When a PID is present the marker is honoured only while that process is both
// alive and still looks like an agent session (see pidIsAgentFunc), so a crashed
// hook can't leave a worktree pinned in-use forever — not even if the kernel
// recycles the PID to an unrelated process. With no PID we fall back to a
// freshness window on the file's mtime.
const markerName = ".treetop-inuse"

// pidIsAgentFunc verifies that a live marker PID still belongs to an agent
// session, guarding against PID reuse. It's a package var so tests can swap in a
// deterministic check; the real, platform-specific implementations live in
// session_{linux,darwin,other}.go (the "other" build can't introspect processes
// and always returns true, preserving existence-only behaviour there).
var pidIsAgentFunc = pidIsAgent

// markerTTL bounds how long a PID-less marker counts as live after its last
// write, so a hook that forgets to clean up doesn't pin a worktree forever.
const markerTTL = 5 * time.Minute

// markerHeadMax caps how much of the marker we read: enough for a PID line, but
// bounded so a pathological file can't trigger an unbounded read on the refresh
// path.
const markerHeadMax = 128

// markerActive reports whether worktreePath holds a live .treetop-inuse marker.
func markerActive(worktreePath string) bool {
	path := filepath.Join(worktreePath, markerName)
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return false // absent, or not a plain file (don't follow a symlink)
	}
	data, err := readHead(path, markerHeadMax)
	if err != nil {
		return false
	}
	if pid, ok := firstLinePID(data); ok {
		// Honour the PID marker only while the process is alive AND still looks
		// like an agent — guarding against a recycled PID pinning the worktree.
		return pidAlive(pid) && pidIsAgentFunc(pid)
	}
	// No PID: trust the marker only while its mtime is recent — and not in the
	// future, which would otherwise read as perpetually fresh.
	age := time.Since(fi.ModTime())
	return age >= 0 && age <= markerTTL
}

// readHead reads up to max bytes from the start of a file.
func readHead(path string, max int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(max)))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// firstLinePID parses a PID from the marker's first line, if present.
func firstLinePID(data []byte) (int, bool) {
	line := data
	if i := strings.IndexByte(string(data), '\n'); i >= 0 {
		line = data[:i]
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(line)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// pidAlive reports whether a process with the given PID currently exists.
// Signal 0 performs the kernel's existence/permission check without delivering
// a signal, and is portable across Unix platforms (covers macOS, where the
// /proc scan is unavailable but the marker still works).
func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	// nil  -> alive and ours; EPERM -> alive but owned by another user.
	return err == nil || err == syscall.EPERM
}
