#!/usr/bin/env bash
#
# Install (or remove) the treetop in-use hooks into an agent's settings.
#
# Providers:
#
#   --claude  Claude Code settings (default)
#   --codex   Codex hooks.json
#
# Scopes:
#
#   --global  user-level agent config; fires in every project (default)
#   --repo    repo-level agent config; committable with that repo
#
# The two scopes are independent. Installing one never touches the other, and a
# global + repo install simply runs both. Pick --global if you want every
# worktree across all your projects tracked; pick --repo to scope it to (and
# commit it with) a single repository.
#
# A --global install can also add ".treetop-inuse" to your global git excludes
# (core.excludesfile) so the marker file is not reported as untracked in every
# repo. Because that edits your global git config, it asks first: it prompts when
# run interactively and is skipped otherwise. Pass --git-excludes to opt in
# without a prompt (e.g. in scripts/CI) or --no-git-excludes to skip it.
#
# Usage:
#   hooks/install.sh [--claude | --codex] [--global | --repo [DIR]] [--git-excludes | --no-git-excludes] [--uninstall]
#
# Requires: jq (to merge into settings without clobbering existing keys).
set -euo pipefail

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROVIDER="claude"
SCOPE="global"
SCOPE_DIR=""
UNINSTALL=0
EXCLUDES_CHOICE="" # "", "yes", or "no"; set by --git-excludes/--no-git-excludes

while [ $# -gt 0 ]; do
	case "$1" in
	--claude) PROVIDER="claude" ;;
	--codex) PROVIDER="codex" ;;
	--provider)
		case "${2:-}" in
		claude | codex) PROVIDER="$2"; shift ;;
		*) echo "install.sh: --provider must be claude or codex" >&2; exit 2 ;;
		esac
		;;
	--global) SCOPE="global" ;;
	--repo | --project)
		SCOPE="repo"
		case "${2:-}" in -* | "") : ;; *) SCOPE_DIR="$2"; shift ;; esac
		;;
	--git-excludes) EXCLUDES_CHOICE="yes" ;;
	--no-git-excludes) EXCLUDES_CHOICE="no" ;;
	--uninstall) UNINSTALL=1 ;;
	-h | --help)
		# Print the header comment block (skip the shebang, stop at the first
		# non-comment line) with the leading "# " stripped.
		awk 'NR==1{next} /^#/{sub(/^# ?/,""); print; next} {exit}' "${BASH_SOURCE[0]}"
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

root=""
if [ "$SCOPE" = "repo" ]; then
	root="${SCOPE_DIR:-$PWD}"
	root="$(git -C "$root" rev-parse --show-toplevel 2>/dev/null || echo "$root")"
fi

case "$PROVIDER:$SCOPE" in
claude:global)
	BASE="$HOME/.claude"
	SETTINGS="$BASE/settings.json"
	HOOKS_DIR="$BASE/hooks"
	MARK_CMD="$HOOKS_DIR/treetop-mark-inuse.sh"
	;;
claude:repo)
	BASE="$root/.claude"
	SETTINGS="$BASE/settings.json"
	HOOKS_DIR="$BASE/hooks"
	MARK_CMD="\$CLAUDE_PROJECT_DIR/.claude/hooks/treetop-mark-inuse.sh"
	;;
codex:global)
	BASE="$HOME/.codex"
	SETTINGS="$BASE/hooks.json"
	HOOKS_DIR="$BASE/hooks"
	MARK_CMD="$HOOKS_DIR/treetop-mark-inuse.sh"
	;;
codex:repo)
	BASE="$root/.codex"
	SETTINGS="$BASE/hooks.json"
	HOOKS_DIR="$BASE/hooks"
	# Codex command hooks run with the session cwd. Resolve repo-local scripts
	# from the git root so the config works when Codex starts in a subdirectory.
	MARK_CMD='"$(git rev-parse --show-toplevel)/.codex/hooks/treetop-mark-inuse.sh"'
	;;
esac

mkdir -p "$BASE"
if [ -f "$SETTINGS" ]; then
	cp "$SETTINGS" "$SETTINGS.bak" # back up real prior settings only
	BACKED_UP=1
else
	echo '{}' >"$SETTINGS"
	BACKED_UP=0
fi

# merge_settings rewrites $SETTINGS by piping it through the given jq program,
# with $mark bound to the mark-hook command string.
merge_settings() {
	local prog="$1" tmp
	tmp="$(mktemp)"
	jq --arg mark "$MARK_CMD" "$prog" "$SETTINGS" >"$tmp"
	mv "$tmp" "$SETTINGS"
}

# The mark hook heartbeats every SessionStart / SubagentStart / PreToolUse /
# PostToolUse (same four events for Claude and Codex), and there is no stop hook
# — treetop lets the marker decay by mtime (see marker.go). STRIP removes any
# treetop hook (by script name, so it also cleans up the pre-heartbeat
# SubagentStop/Stop "unmark" hooks on upgrade) from every slot we might touch,
# then drops now-empty legacy slots; ADD re-adds the mark hook to the four
# heartbeat events. Keyed by script name, so re-running is idempotent.
STRIP='
  def strip_tt:
    map(.hooks |= map(select((.command // "") | test("treetop-(un)?mark-inuse\\.sh") | not)))
    | map(select((.hooks | length) > 0));
  .hooks //= {}
  | reduce ("SessionStart","SubagentStart","PreToolUse","PostToolUse","SubagentStop","Stop") as $e
      (.; .hooks[$e] = ((.hooks[$e] // []) | strip_tt))
  | reduce ("SubagentStop","Stop") as $e
      (.; if (.hooks[$e] | length) == 0 then del(.hooks[$e]) else . end)
'
ADD='
  | reduce ("SessionStart","SubagentStart","PreToolUse","PostToolUse") as $e
      (.; .hooks[$e] += [{matcher: "*", hooks: [{type: "command", command: $mark}]}])
'

if [ "$UNINSTALL" = 1 ]; then
	merge_settings "$STRIP"
	rm -f "$HOOKS_DIR/treetop-mark-inuse.sh" "$HOOKS_DIR/treetop-unmark-inuse.sh"
	echo "treetop $PROVIDER hooks removed from $SETTINGS"
	[ "$BACKED_UP" = 1 ] && echo "(backup at $SETTINGS.bak)"
	exit 0
fi

# Install: copy the hook script into place, then add the hook entries. Remove any
# stale unmark script from a pre-heartbeat install (its hook entries are stripped
# above).
mkdir -p "$HOOKS_DIR"
install -m 0755 "$SRC_DIR/treetop-mark-inuse.sh" "$HOOKS_DIR/treetop-mark-inuse.sh"
rm -f "$HOOKS_DIR/treetop-unmark-inuse.sh"

merge_settings "$STRIP$ADD"

# want_global_excludes decides whether to add .treetop-inuse to the user's
# global git excludes. This edits global git config, so never do it silently:
# an explicit flag wins; otherwise prompt when interactive and default to "no"
# (so a piped/CI install.sh never modifies global config without consent).
want_global_excludes() {
	case "$EXCLUDES_CHOICE" in
	yes) return 0 ;;
	no) return 1 ;;
	esac
	if [ -t 0 ]; then
		printf 'treetop: add ".treetop-inuse" to your global git excludes so the marker is ignored in every repo? [y/N] ' >&2
		local reply=""
		read -r reply || reply=""
		case "$reply" in [Yy]*) return 0 ;; *) return 1 ;; esac
	fi
	return 1
}

# Keep the marker from showing up as an untracked file in every repo it lands
# in, by adding it to the user's global git excludes (global install only, and
# only with the user's consent; see want_global_excludes).
if [ "$SCOPE" = "global" ]; then
	if want_global_excludes; then
		excludes="$(git config --global --get core.excludesfile 2>/dev/null || true)"
		excludes="${excludes:-$HOME/.config/git/ignore}"
		# Expand a leading ~ that git stores literally.
		case "$excludes" in "~/"*) excludes="$HOME/${excludes#\~/}" ;; esac
		mkdir -p "$(dirname "$excludes")"
		git config --global core.excludesfile "$excludes"
		if ! { [ -f "$excludes" ] && grep -qxF '.treetop-inuse' "$excludes"; }; then
			# Start on a fresh line if the file is non-empty and lacks a trailing
			# newline, so .treetop-inuse does not glue onto the previous entry.
			# Command substitution strips trailing newlines, so a final newline
			# makes $(tail -c1) empty.
			if [ -s "$excludes" ] && [ -n "$(tail -c1 "$excludes")" ]; then
				printf '\n' >>"$excludes"
			fi
			printf '%s\n' '.treetop-inuse' >>"$excludes"
			echo "added .treetop-inuse to global gitignore ($excludes)"
		fi
	else
		echo "skipped global gitignore change (no consent); .treetop-inuse may show as untracked."
		echo "  to enable later, rerun with --git-excludes, or add '.treetop-inuse' to your"
		echo "  global git excludes (core.excludesfile, default ~/.config/git/ignore)."
	fi
fi

echo "treetop $PROVIDER hooks installed at $SCOPE scope:"
echo "  settings: $SETTINGS"
echo "  scripts:  $HOOKS_DIR/treetop-{mark,unmark}-inuse.sh"
[ "$BACKED_UP" = 1 ] && echo "  (backup of prior settings at $SETTINGS.bak)"
if [ "$SCOPE" = "repo" ]; then
	echo "  commit ${BASE#"$root"/}/ to share with the repo."
	if [ "$PROVIDER" = "codex" ]; then
		echo "  Codex runs repo-local hooks only after the project config is trusted."
	fi
fi
