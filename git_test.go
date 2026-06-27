package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestGitArgvHardening(t *testing.T) {
	argv := gitArgv("/repo", []string{"ls-files", "--others"})
	want := []string{"-C", "/repo", "-c", "core.fsmonitor=", "ls-files", "--others"}
	if !slices.Equal(argv, want) {
		t.Errorf("gitArgv = %v, want %v", argv, want)
	}
	// The hardening overrides must precede the subcommand, or git ignores them.
	ci := slices.Index(argv, "core.fsmonitor=")
	si := slices.Index(argv, "ls-files")
	if ci < 0 || si < 0 || ci > si {
		t.Errorf("core.fsmonitor override must come before the subcommand: %v", argv)
	}
}

// TestScanDoesNotExecuteRepoConfig is the regression test for the RCE: a
// repository's own config (core.fsmonitor) must never be executed when treetop
// reads it. Without the hardening, ignoredDirs' `git ls-files --others` runs the
// fsmonitor program; with it, the program must never fire.
func TestScanDoesNotExecuteRepoConfig(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "t@t.t")
	git("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "f")
	git("commit", "-qm", "init")

	// A malicious fsmonitor hook that writes a sentinel when executed.
	sentinel := filepath.Join(t.TempDir(), "pwned")
	hook := filepath.Join(repo, ".git", "evil.sh")
	script := "#!/bin/sh\ntouch " + shellQuote(sentinel) + "\n"
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	git("config", "core.fsmonitor", hook)

	// Drive the exact code path treetop uses to enumerate ignored dirs.
	ignoredDirs(repo)

	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("repo's core.fsmonitor was executed during scan: RCE not mitigated")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error: %v", err)
	}
}

// shellQuote wraps s in single quotes for safe embedding in /bin/sh.
func shellQuote(s string) string {
	return "'" + filepath.ToSlash(s) + "'"
}
