package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestRenderEmptyNoFilter(t *testing.T) {
	var b strings.Builder
	r := newRenderer(&b, false, false, false)
	r.render(nil, true)
	if got := b.String(); !strings.Contains(got, "No worktrees found.") {
		t.Errorf("empty render without a filter = %q, want \"No worktrees found.\"", got)
	}
}

func TestRenderEmptyWithFilter(t *testing.T) {
	var b strings.Builder
	r := newRenderer(&b, false, false, false)
	r.filterDesc = "--in-use"
	r.render(nil, true)
	got := b.String()
	if !strings.Contains(got, "No worktrees match") || !strings.Contains(got, "--in-use") {
		t.Errorf("empty render with a filter = %q, want a \"No worktrees match --in-use\" message", got)
	}
}

func TestFilterDescription(t *testing.T) {
	pat := regexp.MustCompile("(?i)snag")
	cases := []struct {
		name string
		opts options
		want string
	}{
		{"none", options{}, ""},
		{"in-use", options{onlyInUse: true}, "--in-use"},
		{"open", options{onlyOpen: true}, "--open"},
		{"pattern", options{patterns: []*regexp.Regexp{pat}}, `pattern "snag"`},
		{"in-use+pattern", options{onlyInUse: true, patterns: []*regexp.Regexp{pat}}, `--in-use and pattern "snag"`},
	}
	for _, c := range cases {
		if got := filterDescription(c.opts); got != c.want {
			t.Errorf("%s: filterDescription = %q, want %q", c.name, got, c.want)
		}
	}
}
