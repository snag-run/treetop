#!/usr/bin/env bash
#
# Install (or remove) the treetop in-use hooks into Claude Code's settings.
#
# Claude Code merges hooks from every settings scope, so you can install at:
#
#   --global  ~/.claude/settings.json          (default; fires in every project)
#   --repo    <repo>/.claude/settings.json      (committable; fires in that repo)
#
# The two scopes are independent — installing one never touches the other, and a
# global + repo install simply runs both. Pick --global if you want every
# worktree across all your projects tracked; pick --repo to scope it to (and
# commit it with) a single repository.
#
# Usage:
#   hooks/install.sh [--global | --repo [DIR]] [--uninstall]
#
# Requires: jq (to merge into settings.json without clobbering existing keys).
set -euo pipefail

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCOPE="global"
SCOPE_DIR=""
UNINSTALL=0

while [ $# -gt 0 ]; do
	case "$1" in
	--global) SCOPE="global" ;;
	--repo | --project)
		SCOPE="repo"
		case "${2:-}" in -* | "") : ;; *) SCOPE_DIR="$2"; shift ;; esac
		;;
	--uninstall) UNINSTALL=1 ;;
	-h | --help)
		sed -n '2,28p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "install.sh: unknown argument: $1" >&2
		exit 2
		;;
	esac
	shift
done

command -v jq >/dev/null 2>&1 || {
	echo "install.sh: jq is required (https://jqlang.github.io/jq/)" >&2
	exit 1
}

# Resolve the settings file, the directory the hook scripts live in, and the
# command paths written into settings. For --repo we reference the scripts via
# $CLAUDE_PROJECT_DIR (which Claude Code expands) so the config stays valid when
# the repo is committed and cloned elsewhere.
if [ "$SCOPE" = "global" ]; then
	BASE="$HOME/.claude"
	SETTINGS="$BASE/settings.json"
	HOOKS_DIR="$BASE/hooks"
	MARK_CMD="$HOOKS_DIR/treetop-mark-inuse.sh"
	UNMARK_CMD="$HOOKS_DIR/treetop-unmark-inuse.sh"
else
	root="${SCOPE_DIR:-$PWD}"
	root="$(git -C "$root" rev-parse --show-toplevel 2>/dev/null || echo "$root")"
	BASE="$root/.claude"
	SETTINGS="$BASE/settings.json"
	HOOKS_DIR="$BASE/hooks"
	MARK_CMD="\$CLAUDE_PROJECT_DIR/.claude/hooks/treetop-mark-inuse.sh"
	UNMARK_CMD="\$CLAUDE_PROJECT_DIR/.claude/hooks/treetop-unmark-inuse.sh"
fi

mkdir -p "$BASE"
if [ -f "$SETTINGS" ]; then
	cp "$SETTINGS" "$SETTINGS.bak" # back up real prior settings only
	BACKED_UP=1
else
	echo '{}' >"$SETTINGS"
	BACKED_UP=0
fi

# merge_settings rewrites $SETTINGS by piping it through the given jq program,
# with $mark/$unmark bound to the command strings. Always removes any existing
# treetop entries first (keyed by exact command match), so it's idempotent.
merge_settings() {
	local prog="$1" tmp
	tmp="$(mktemp)"
	jq --arg mark "$MARK_CMD" --arg unmark "$UNMARK_CMD" "$prog" "$SETTINGS" >"$tmp"
	mv "$tmp" "$SETTINGS"
}

STRIP='
  .hooks //= {}
  | .hooks.SubagentStart //= []
  | .hooks.SubagentStop  //= []
  | .hooks.SubagentStart |= map(select([.hooks[]?.command] | index($mark)  | not))
  | .hooks.SubagentStop  |= map(select([.hooks[]?.command] | index($unmark) | not))
'

if [ "$UNINSTALL" = 1 ]; then
	merge_settings "$STRIP"
	rm -f "$HOOKS_DIR/treetop-mark-inuse.sh" "$HOOKS_DIR/treetop-unmark-inuse.sh"
	echo "treetop hooks removed from $SETTINGS"
	[ "$BACKED_UP" = 1 ] && echo "(backup at $SETTINGS.bak)"
	exit 0
fi

# Install: copy the hook scripts into place, then add the two hook entries.
mkdir -p "$HOOKS_DIR"
install -m 0755 "$SRC_DIR/treetop-mark-inuse.sh" "$HOOKS_DIR/treetop-mark-inuse.sh"
install -m 0755 "$SRC_DIR/treetop-unmark-inuse.sh" "$HOOKS_DIR/treetop-unmark-inuse.sh"

merge_settings "$STRIP"'
  | .hooks.SubagentStart += [{matcher: "*", hooks: [{type: "command", command: $mark}]}]
  | .hooks.SubagentStop  += [{matcher: "*", hooks: [{type: "command", command: $unmark}]}]
'

# Keep the marker from showing up as an untracked file in every repo it lands
# in, by adding it to the user's global git excludes (global install only).
if [ "$SCOPE" = "global" ]; then
	excludes="$(git config --global --get core.excludesfile 2>/dev/null || true)"
	excludes="${excludes:-$HOME/.config/git/ignore}"
	# Expand a leading ~ that git stores literally.
	case "$excludes" in "~/"*) excludes="$HOME/${excludes#\~/}" ;; esac
	mkdir -p "$(dirname "$excludes")"
	git config --global core.excludesfile "$excludes"
	if ! { [ -f "$excludes" ] && grep -qxF '.treetop-inuse' "$excludes"; }; then
		printf '%s\n' '.treetop-inuse' >>"$excludes"
		echo "added .treetop-inuse to global gitignore ($excludes)"
	fi
fi

echo "treetop hooks installed at $SCOPE scope:"
echo "  settings: $SETTINGS"
echo "  scripts:  $HOOKS_DIR/treetop-{mark,unmark}-inuse.sh"
[ "$BACKED_UP" = 1 ] && echo "  (backup of prior settings at $SETTINGS.bak)"
if [ "$SCOPE" = "repo" ]; then
	echo "  commit .claude/settings.json and .claude/hooks/ to share with the repo."
fi
