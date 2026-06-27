package main

import (
	"context"
	"os/exec"
)

// gitHardenedArgs are config overrides prepended to every git invocation so a
// scanned repository's own config can't turn a read into code execution.
//
// treetop runs git inside repositories it does not trust: anything one level
// under a scanned root, including repos cloned or created by agents. Git honours
// repo-local config keys that name an external program and runs them during
// ordinary working-tree scans — most notably core.fsmonitor, which `git ls-files
// --others` executes. Forcing those keys to empty neutralises the exec vector
// while leaving the metadata these read-only commands actually need intact. The
// repo's own ownership check (safe.directory) does not help here, because the
// hostile repo is typically owned by the same user who runs treetop.
var gitHardenedArgs = []string{
	"-c", "core.fsmonitor=",
}

// gitCommand builds `git -C dir <hardened> args...`. All of treetop's git calls
// go through here (or gitCommandContext) so the hardening can't be forgotten.
func gitCommand(dir string, args ...string) *exec.Cmd {
	return exec.Command("git", gitArgv(dir, args)...)
}

// gitCommandContext is gitCommand bound to a context, for a timeout-guarded call.
func gitCommandContext(ctx context.Context, dir string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "git", gitArgv(dir, args)...)
}

// gitArgv assembles the full argument vector: -C dir, the hardening flags, then
// the subcommand and its args. The -c overrides must precede the subcommand.
func gitArgv(dir string, args []string) []string {
	argv := make([]string, 0, 2+len(gitHardenedArgs)+len(args))
	argv = append(argv, "-C", dir)
	argv = append(argv, gitHardenedArgs...)
	argv = append(argv, args...)
	return argv
}
