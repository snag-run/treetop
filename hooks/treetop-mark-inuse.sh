#!/usr/bin/env bash
#
# treetop agent hook: heartbeat the active worktree as "in use" by (re)stamping
# a .treetop-inuse marker at the worktree root.
#
# Wired to fire on every SessionStart / SubagentStart / PreToolUse / PostToolUse,
# so the marker's mtime tracks the last moment an agent touched this worktree.
# treetop honours the marker only while that mtime is recent (see marker.go), so
# activity that stops simply decays — there is no "stop" hook to miss, and a
# crashed or force-quit session can't pin a worktree in use beyond the decay
# window. In-process subagents (which never chdir into the worktree they target)
# are covered because the hook — not treetop — reports the cwd it worked in.
#
# Some agent hooks pass a JSON object on stdin that includes "cwd"; Codex command
# hooks also run with the session cwd. We resolve that to the worktree root and
# touch the marker there.
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

input=$(cat)

cwd=$(extract_cwd "$input")
cwd=${cwd:-$PWD}
[ -n "$cwd" ] || exit 0

root=$(git -C "$cwd" rev-parse --show-toplevel 2>/dev/null) || exit 0
[ -n "$root" ] || exit 0

# Re-stamp the marker with the current time. treetop trusts it only while the
# mtime is fresh, so each heartbeat renews the worktree's in-use window and a
# lapse in activity lets the marker decay on its own.
printf 'treetop in-use since %s\n' "$(date -u +%FT%TZ 2>/dev/null)" >"$root/.treetop-inuse" 2>/dev/null || true
exit 0
