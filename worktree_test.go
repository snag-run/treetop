package main

import "testing"

func TestParseWorktrees(t *testing.T) {
	out := []byte(`worktree /home/me/repo
bare

worktree /home/me/repo-feature
HEAD abc123
branch refs/heads/feature/login

worktree /home/me/repo-detached
HEAD def456
detached
`)

	got := parseWorktrees(out)
	if len(got) != 3 {
		t.Fatalf("expected 3 worktrees, got %d", len(got))
	}

	tests := []struct {
		path string
		ref  string
	}{
		{"/home/me/repo", "(bare)"}, // sorted by path
		{"/home/me/repo-detached", "(detached)"},
		{"/home/me/repo-feature", "feature/login"},
	}
	for i, tt := range tests {
		if got[i].Path != tt.path {
			t.Errorf("worktree %d: path = %q, want %q", i, got[i].Path, tt.path)
		}
		if got[i].Ref() != tt.ref {
			t.Errorf("worktree %d: ref = %q, want %q", i, got[i].Ref(), tt.ref)
		}
	}
}

func TestProjectName(t *testing.T) {
	tests := []struct {
		common string
		want   string
	}{
		{"/home/me/snag", "snag"},            // bare repo
		{"/home/me/athanor/.git", "athanor"}, // normal repo
		{"/home/me/repo.git", "repo"},        // bare with .git suffix
	}
	for _, tt := range tests {
		if got := projectName(tt.common); got != tt.want {
			t.Errorf("projectName(%q) = %q, want %q", tt.common, got, tt.want)
		}
	}
}
