#!/usr/bin/env bash
# PostToolUse hook: gofmt edited Go files in place so they never trip CI's
# `gofmt -l` formatting check (.github/workflows/ci.yml).
#
# Reads the hook payload as JSON on stdin and formats the touched file.
# Needs jq; if it's missing the hook is a no-op rather than an error.
set -euo pipefail

command -v jq >/dev/null 2>&1 || exit 0

payload="$(cat)"
file="$(printf '%s' "$payload" | jq -r '.tool_input.file_path // empty')"

[ -z "$file" ] && exit 0

case "$file" in
  *.go) gofmt -w "$file" 2>/dev/null || true ;;
esac

exit 0
