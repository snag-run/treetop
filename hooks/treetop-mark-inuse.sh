#!/usr/bin/env bash
#
# treetop SubagentStart hook: mark the worktree a subagent is working in as
# "in use" by dropping a .treetop-inuse marker at the worktree root.
#
# Claude Code passes the hook a JSON object on stdin that includes "cwd" — for a
# subagent this is the directory it is operating in (the isolated worktree, or
# the parent worktree for a non-isolated subagent). We resolve that to the
# worktree root and write the marker there.
#
# The marker's first line is the PID of the owning `claude` process, so treetop
# honours it only while that process is alive: even if the matching SubagentStop
# never fires (crash, force-quit), the marker can't pin a worktree in use beyond
# the session's lifetime. See marker.go for how treetop reads it.
#
# Hooks must never disrupt the session, so this always exits 0.
set -uo pipefail

# extract_cwd reads the "cwd" field from the hook's JSON stdin, preferring jq,
# then python3, then a best-effort grep so the hook still works without jq.
extract_cwd() {
	local json="$1"
	if command -v jq >/dev/null 2>&1; then
		printf '%s' "$json" | jq -r '.cwd // empty' 2>/dev/null && return
	fi
	if command -v python3 >/dev/null 2>&1; then
		printf '%s' "$json" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("cwd",""))' 2>/dev/null && return
	fi
	printf '%s' "$json" | grep -o '"cwd"[[:space:]]*:[[:space:]]*"[^"]*"' | head -n1 | sed 's/.*"cwd"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
}

# claude_pid walks the parent-process chain (Linux /proc) to find the long-lived
# `claude` session that spawned this hook, so the marker outlives a single
# subagent but not the session. Empty on non-Linux or if none is found.
claude_pid() {
	[ -r /proc ] || return 0
	local pid=$PPID i
	for i in 1 2 3 4 5 6 7 8; do
		[ "${pid:-0}" -gt 1 ] 2>/dev/null || return 0
		local comm
		comm=$(cat "/proc/$pid/comm" 2>/dev/null) || return 0
		if [ "$comm" = "claude" ]; then
			printf '%s' "$pid"
			return 0
		fi
		if [ "$comm" = "node" ] && tr '\0' ' ' <"/proc/$pid/cmdline" 2>/dev/null | grep -qi claude; then
			printf '%s' "$pid"
			return 0
		fi
		pid=$(awk '{print $4}' "/proc/$pid/stat" 2>/dev/null) || return 0
	done
}

input=$(cat)

cwd=$(extract_cwd "$input")
[ -n "$cwd" ] || exit 0

root=$(git -C "$cwd" rev-parse --show-toplevel 2>/dev/null) || exit 0
[ -n "$root" ] || exit 0

pid=$(claude_pid)
if [ -n "$pid" ]; then
	printf '%s\n' "$pid" >"$root/.treetop-inuse" 2>/dev/null || true
else
	# No usable PID (e.g. non-Linux): fall back to a timestamped marker, which
	# treetop trusts for a few minutes via the file's mtime.
	printf 'treetop in-use since %s\n' "$(date -u +%FT%TZ 2>/dev/null)" >"$root/.treetop-inuse" 2>/dev/null || true
fi
exit 0
