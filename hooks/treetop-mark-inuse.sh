#!/usr/bin/env bash
#
# treetop agent hook: mark the active worktree as
# "in use" by dropping a .treetop-inuse marker at the worktree root.
#
# Some agent hooks pass a JSON object on stdin that includes "cwd". Codex
# command hooks also run with the session cwd. We resolve that directory to the
# worktree root and write the marker there.
#
# The marker's first line is the PID of the owning agent process, so treetop
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

node_agent_cmdline() {
	local cmdline="$1" token norm base parent
	for token in $cmdline; do
		norm=${token//\\//}
		norm=$(printf '%s' "$norm" | tr -d "\"'")
		norm=${norm,,}
		case "$norm" in
		*claude-code*|*@anthropic-ai/claude-code*|*@openai/codex*) return 0 ;;
		esac
		base=${norm##*/}
		case "$base" in
		claude|claude.js|claude.cmd|codex|codex.js|codex.cmd) return 0 ;;
		cli.js)
			parent=${norm%/*}
			case "${parent##*/}" in
			claude|claude-code|codex) return 0 ;;
			esac
			;;
		esac
	done
	return 1
}

# agent_pid walks the parent-process chain to find the long-lived agent
# session that spawned this hook, so the marker outlives a single subagent but
# not the session. Uses /proc on Linux and `ps` on macOS/other Unix; empty if no
# agent ancestor is found. Treetop honours a PID-stamped marker only while that
# process is alive (see marker.go), so this is what lets the marker survive a
# subagent that runs longer than the PID-less mtime window.
agent_pid() {
	if [ -r /proc ]; then
		agent_pid_proc
	else
		agent_pid_ps
	fi
}

# agent_pid_proc walks the chain via Linux /proc.
agent_pid_proc() {
	local pid=$PPID i
	for i in 1 2 3 4 5 6 7 8; do
		[ "${pid:-0}" -gt 1 ] 2>/dev/null || return 0
		local comm
		comm=$(cat "/proc/$pid/comm" 2>/dev/null) || return 0
		if [ "$comm" = "claude" ] || [ "$comm" = "codex" ]; then
			printf '%s' "$pid"
			return 0
		fi
		if [ "$comm" = "node" ] && node_agent_cmdline "$(tr '\0' ' ' <"/proc/$pid/cmdline" 2>/dev/null)"; then
			printf '%s' "$pid"
			return 0
		fi
		pid=$(awk '{print $4}' "/proc/$pid/stat" 2>/dev/null) || return 0
	done
}

# agent_pid_ps walks the chain via `ps` (macOS and any Unix without /proc),
# applying the same agent test as the /proc path. Columns
# are read one at a time: `ps -o ppid=` right-justifies with leading spaces, so a
# combined format would be fragile to split. The command line is only consulted
# to disambiguate a `node` ancestor.
agent_pid_ps() {
	command -v ps >/dev/null 2>&1 || return 0
	local pid=$PPID i
	for i in 1 2 3 4 5 6 7 8; do
		[ "${pid:-0}" -gt 1 ] 2>/dev/null || return 0
		local ppid comm
		comm=$(ps -o comm= -p "$pid" 2>/dev/null) || return 0
		[ -n "$comm" ] || return 0
		case "${comm##*/}" in
		claude|codex)
			printf '%s' "$pid"
			return 0
			;;
		node)
			if node_agent_cmdline "$(ps -o command= -p "$pid" 2>/dev/null)"; then
				printf '%s' "$pid"
				return 0
			fi
			;;
		esac
		ppid=$(ps -o ppid= -p "$pid" 2>/dev/null | tr -d ' ') || return 0
		pid=$ppid
	done
}

input=$(cat)

cwd=$(extract_cwd "$input")
cwd=${cwd:-$PWD}
[ -n "$cwd" ] || exit 0

root=$(git -C "$cwd" rev-parse --show-toplevel 2>/dev/null) || exit 0
[ -n "$root" ] || exit 0

pid=$(agent_pid)
if [ -n "$pid" ]; then
	printf '%s\n' "$pid" >"$root/.treetop-inuse" 2>/dev/null || true
else
	# No usable PID (e.g. non-Linux): fall back to a timestamped marker, which
	# treetop trusts for a few minutes via the file's mtime.
	printf 'treetop in-use since %s\n' "$(date -u +%FT%TZ 2>/dev/null)" >"$root/.treetop-inuse" 2>/dev/null || true
fi
exit 0
