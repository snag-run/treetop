//go:build darwin

package main

import (
	"reflect"
	"testing"
)

func TestParsePSAgents(t *testing.T) {
	// pid + full command line, as `ps -axww -o pid=,command=` emits it. Includes
	// native `claude`, an npm node-launched claude, and noise that must not match
	// (a node process unrelated to claude, a path that merely contains "claude").
	out := []byte(`  82450 claude
  83632 claude --resume
  90001 /Users/me/.nvm/versions/node/v22/bin/node /Users/me/.nvm/.../claude-code/cli.js
  90002 /usr/local/bin/node /Users/me/app/server.js
  90003 /usr/bin/vim /Users/me/claudefile.txt
  90004 /opt/homebrew/bin/treetop -w
not-a-pid here
`)
	got := parsePSAgents(out)
	want := []string{"82450", "83632", "90001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsePSAgents = %v, want %v", got, want)
	}
}

func TestParseLSOF(t *testing.T) {
	// Two processes, in the -Fpftn shape lsof emits: p<pid>, then f/t/n per
	// descriptor. cwd is fd "cwd"; numbered REG and DIR fds are open files; the
	// txt executable mapping and non-REG/DIR descriptors (CHR, unix) are ignored.
	out := []byte(`p83632
fcwd
tDIR
n/Users/davidtaing/treetop
ftxt
tREG
n/Users/davidtaing/.nvm/.../bin/claude.exe
f0
tCHR
n/dev/null
f3
tREG
n/Users/davidtaing/treetop/main.go
f11
tunix
n->0x61894cad7bf3803d
p90001
fcwd
tDIR
n/Users/me/proj
f5
tREG
n/Users/me/proj/src/app.go
f7
tDIR
n/Users/me/proj/watched
`)
	cwds, files := parseLSOF(out)

	wantCwds := []string{"/Users/davidtaing/treetop", "/Users/me/proj"}
	if !reflect.DeepEqual(cwds, wantCwds) {
		t.Errorf("cwds = %v, want %v", cwds, wantCwds)
	}
	// A numbered DIR fd (f7) counts as an open file, matching what Linux's
	// /proc/<pid>/fd surfaces; only the "cwd" descriptor lands in cwds.
	wantFiles := []string{"/Users/davidtaing/treetop/main.go", "/Users/me/proj/src/app.go", "/Users/me/proj/watched"}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Errorf("files = %v, want %v", files, wantFiles)
	}
}

func TestParseLSOFEmpty(t *testing.T) {
	cwds, files := parseLSOF(nil)
	if cwds != nil || files != nil {
		t.Fatalf("parseLSOF(nil) = (%v, %v), want (nil, nil)", cwds, files)
	}
}
