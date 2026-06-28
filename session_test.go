package main

import "testing"

// agentName lives in the untagged session.go and is used on every platform, so
// its test is untagged too — it runs on the Linux CI leg and local Linux dev,
// not only the darwin leg.
func TestAgentName(t *testing.T) {
	cases := []struct {
		comm, cmdline string
		want          bool
	}{
		{"claude", "", true},
		{"claude", "claude --resume", true},
		{"codex", "", true},
		{"codex", "codex --sandbox workspace-write", true},
		{"node", "node /path/to/claude-code/cli.js", true},
		{"node", "node /path/to/CLAUDE/cli.js", true}, // case-insensitive
		{"node", "node /path/to/@openai/codex/bin/codex.js", true},
		{"node", "node /path/to/CODEX/cli.js", true}, // case-insensitive
		{"node", `node C:\Users\me\AppData\Roaming\npm\node_modules\codex\cli.js`, true},
		{"node", "node server.js", false},
		{"node", "node /path/to/codexfile/server.js", false},
		{"vim", "vim claude.txt", false},
		{"vim", "vim codex.txt", false},
		{"treetop", "treetop -w", false},
	}
	for _, c := range cases {
		if got := agentName(c.comm, c.cmdline); got != c.want {
			t.Errorf("agentName(%q, %q) = %v, want %v", c.comm, c.cmdline, got, c.want)
		}
	}
}
