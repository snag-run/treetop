package main

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestOSName(t *testing.T) {
	cases := map[string]string{
		"linux":   "Linux",
		"darwin":  "macOS",
		"freebsd": "freebsd",
	}
	for goos, want := range cases {
		if got := osName(goos); got != want {
			t.Errorf("osName(%q) = %q, want %q", goos, got, want)
		}
	}
}

func TestParseOSRelease(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "quoted pretty name",
			content: "NAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\nID=ubuntu\n",
			want:    "Ubuntu 24.04 LTS",
		},
		{
			name:    "unquoted",
			content: "PRETTY_NAME=Fedora\n",
			want:    "Fedora",
		},
		{
			name:    "missing",
			content: "NAME=Whatever\n",
			want:    "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseOSRelease(c.content); got != c.want {
				t.Errorf("parseOSRelease() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDetectTerminal(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "term program with version",
			env:  map[string]string{"TERM_PROGRAM": "ghostty", "TERM_PROGRAM_VERSION": "1.0.0", "TERM": "xterm-256color"},
			want: "ghostty 1.0.0",
		},
		{
			name: "term program without version",
			env:  map[string]string{"TERM_PROGRAM": "Apple_Terminal"},
			want: "Apple_Terminal",
		},
		{
			name: "vte fallback",
			env:  map[string]string{"VTE_VERSION": "7600", "TERM": "xterm-256color"},
			want: "VTE 7600",
		},
		{
			name: "term fallback",
			env:  map[string]string{"TERM": "screen"},
			want: "screen",
		},
		{
			name: "nothing set",
			env:  map[string]string{},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			getenv := func(k string) string { return c.env[k] }
			if got := detectTerminal(getenv); got != c.want {
				t.Errorf("detectTerminal() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestBuildIssueURL(t *testing.T) {
	env := bugEnv{
		Version:   "v0.2.0",
		OS:        "Linux",
		OSVersion: "Ubuntu 24.04 LTS",
		Terminal:  "ghostty 1.0.0",
	}
	got := buildIssueURL(env, "links don't open")

	base, query, ok := strings.Cut(got, "?")
	if !ok || base != repoURL+"/issues/new" {
		t.Fatalf("unexpected base URL: %q", got)
	}
	q, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("query did not parse: %v", err)
	}
	want := map[string]string{
		"template":      "bug_report.yml",
		"what-happened": "links don't open",
		"version":       "treetop v0.2.0",
		"os":            "Linux",
		"os-version":    "Ubuntu 24.04 LTS",
		"terminal":      "ghostty 1.0.0",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("query[%q] = %q, want %q", k, q.Get(k), v)
		}
	}
}

func TestBuildIssueURLOmitsEmptyFields(t *testing.T) {
	// Empty detection results must not appear as blank query params, so the
	// form's required fields keep prompting the reporter.
	got := buildIssueURL(bugEnv{Version: "v0.2.0"}, "")

	_, query, ok := strings.Cut(got, "?")
	if !ok {
		t.Fatalf("missing query string: %q", got)
	}
	q, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("query did not parse: %v", err)
	}
	for _, k := range []string{"what-happened", "os", "os-version", "terminal"} {
		if _, ok := q[k]; ok {
			t.Errorf("query unexpectedly contains empty field %q", k)
		}
	}
	if q.Get("version") != "treetop v0.2.0" {
		t.Errorf("version = %q, want %q", q.Get("version"), "treetop v0.2.0")
	}
}

var errBugReportTest = errors.New("bug report test error")

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errBugReportTest
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errBugReportTest
}

func TestRunBugReportReturnsInputError(t *testing.T) {
	openerCalled := false
	err := runBugReport(errReader{}, &strings.Builder{}, func(string) error {
		openerCalled = true
		return nil
	})
	if !errors.Is(err, errBugReportTest) {
		t.Fatalf("runBugReport() error = %v, want %v", err, errBugReportTest)
	}
	if openerCalled {
		t.Fatal("opener was called after input error")
	}
}

func TestRunBugReportReturnsOutputError(t *testing.T) {
	openerCalled := false
	err := runBugReport(strings.NewReader("description\n"), errWriter{}, func(string) error {
		openerCalled = true
		return nil
	})
	if !errors.Is(err, errBugReportTest) {
		t.Fatalf("runBugReport() error = %v, want %v", err, errBugReportTest)
	}
	if openerCalled {
		t.Fatal("opener was called after output error")
	}
}
