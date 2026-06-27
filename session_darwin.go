//go:build darwin

package main

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// scanSessions locates live agent sessions on macOS, where there is no /proc.
// It finds candidate `claude` (or node-launched claude) processes with `ps`,
// then reads each one's working directory and the files it holds open with
// `lsof`. If `ps` can't run at all, detection reports supported == false; with
// `ps` available but no sessions, it reports supported with an empty scan.
//
// cwd and open-file detection both come from `lsof`, so if lsof is missing or
// sandbox-restricted the scan still reports supported == true but finds nothing
// (unlike Linux, which reads cwd straight from /proc). That silent degradation
// is acceptable: the .treetop-inuse marker file still drives in-use tracking.
func scanSessions() sessionScan {
	pids, err := agentPIDs()
	if err != nil {
		return sessionScan{supported: false}
	}
	scan := sessionScan{supported: true}
	if len(pids) == 0 {
		return scan
	}

	// -n/-P skip DNS and port-name lookups (faster, no hangs); -Fpftn emits the
	// pid, fd, type, and name fields one per line. lsof exits non-zero if a pid
	// vanished mid-scan but still prints usable output for the rest, so we parse
	// whatever we got regardless of the exit status — and if lsof can't run at
	// all, parseLSOF simply yields nothing (see the supported-but-empty note).
	out, _ := exec.Command("lsof", "-w", "-n", "-P", "-Fpftn", "-p", strings.Join(pids, ",")).Output()
	cwds, files := parseLSOF(out)
	for _, c := range cwds {
		scan.cwds = append(scan.cwds, resolveSymlinks(c))
	}
	for _, f := range files {
		scan.openFiles = append(scan.openFiles, resolveSymlinks(f))
	}
	return scan
}

// agentPIDs returns the pids of live agent sessions, parsed from `ps`.
func agentPIDs() ([]string, error) {
	// -axww: every process, full (un-truncated) command line. pid + command are
	// enough to recognise an agent and to feed lsof.
	out, err := exec.Command("ps", "-axww", "-o", "pid=,command=").Output()
	if err != nil {
		return nil, err
	}
	return parsePSAgents(out), nil
}

// parsePSAgents extracts the pids of agent processes from `ps -o pid=,command=`
// output. Each line is "<pid> <command line>"; a process counts when its argv[0]
// base name (or a node command line mentioning claude) looks like an agent.
//
// argv0 is taken as the command up to its first space, so an executable whose
// own path contains a space could be misclassified. That's vanishingly rare for
// the node/claude binaries this targets (they live in PATH or under nvm/brew
// prefixes), and any miss is still covered by the .treetop-inuse marker file.
func parsePSAgents(out []byte) []string {
	var pids []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.IndexAny(line, " \t")
		if i <= 0 {
			continue
		}
		pid := line[:i]
		if _, err := strconv.Atoi(pid); err != nil {
			continue // leading token wasn't a pid
		}
		command := strings.TrimSpace(line[i:])
		if command == "" {
			continue
		}
		argv0 := command
		if j := strings.IndexByte(command, ' '); j >= 0 {
			argv0 = command[:j]
		}
		if agentName(filepath.Base(argv0), command) {
			pids = append(pids, pid)
		}
	}
	return pids
}

// parseLSOF parses `lsof -Fpftn` output into the processes' working directories
// and the files they hold open on numbered descriptors. The -F format prints one
// field per line, tagged by a leading letter: f<fd>, t<type>, n<name>, repeating
// per descriptor. A descriptor is a cwd when its fd is "cwd"; it's an open file
// when its fd is a number and its type is a regular file (REG) or directory
// (DIR). This mirrors Linux's /proc/<pid>/fd, which lists numbered descriptors —
// open files and open directories alike — but not the mmap'd program text or
// libraries that lsof reports on its "txt"/"mem" descriptors.
func parseLSOF(out []byte) (cwds, files []string) {
	var fd, typ string
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		tag, val := line[0], line[1:]
		switch tag {
		case 'f':
			fd, typ = val, ""
		case 't':
			typ = val
		case 'n':
			switch {
			case fd == "cwd":
				cwds = append(cwds, val)
			case (typ == "REG" || typ == "DIR") && isAllDigits(fd):
				files = append(files, val)
			}
		}
	}
	return cwds, files
}

// isAllDigits reports whether s is non-empty and only ASCII digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// resolveSymlinks returns the symlink-free form of path, or path unchanged if it
// can't be resolved. This matches worktree paths, which markInUse also resolves.
func resolveSymlinks(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}
