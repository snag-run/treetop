#!/usr/bin/env bash
#
# treetop SubagentStop hook: clear the .treetop-inuse marker for the worktree a
# subagent was working in, as soon as it finishes.
#
# This is the prompt counterpart to treetop-mark-inuse.sh. (The marker's PID
# gate is only a backstop for when this hook doesn't fire.)
#
# Note: if several subagents share one worktree, the first to stop clears the
# marker while the others keep working; treetop's /proc scan covers that gap.
#
# Hooks must never disrupt the session, so this always exits 0.
set -uo pipefail

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

input=$(cat)

cwd=$(extract_cwd "$input")
[ -n "$cwd" ] || exit 0

root=$(git -C "$cwd" rev-parse --show-toplevel 2>/dev/null) || exit 0
[ -n "$root" ] || exit 0

rm -f "$root/.treetop-inuse" 2>/dev/null || true
exit 0
