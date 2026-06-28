package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// repoURL is treetop's GitHub home, used to build the prefilled bug-report URL.
// It mirrors the module path in go.mod.
const repoURL = "https://github.com/snag-run/treetop"

// bugEnv is the best-effort environment snapshot prefilled into a bug report.
// Any field may be empty if detection fails; the issue form leaves it editable.
type bugEnv struct {
	Version   string // treetop version, e.g. "v0.2.0"
	OS        string // "Linux", "macOS", or the raw GOOS
	OSVersion string // distro+version on Linux, product version on macOS
	Terminal  string // emulator + version from the environment
}

// collectBugEnv gathers what treetop can detect about the host for a bug report.
func collectBugEnv() bugEnv {
	return bugEnv{
		Version:   versionString(),
		OS:        osName(runtime.GOOS),
		OSVersion: osVersion(),
		Terminal:  detectTerminal(os.Getenv),
	}
}

// osName maps a GOOS to the label used by the bug-report form's OS dropdown.
func osName(goos string) string {
	switch goos {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return goos
	}
}

// osVersion best-effort detects the OS version string. On Linux it reads the
// distro's PRETTY_NAME (e.g. "Ubuntu 24.04 LTS"); on macOS it shells out to
// sw_vers for the product version (e.g. "14.5"). Returns "" if undetectable —
// the reporter fills it in via the editable form field.
func osVersion() string {
	switch runtime.GOOS {
	case "linux":
		if b, err := os.ReadFile("/etc/os-release"); err == nil {
			return parseOSRelease(string(b))
		}
	case "darwin":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

// parseOSRelease pulls the PRETTY_NAME value out of an /etc/os-release file.
func parseOSRelease(content string) string {
	for line := range strings.Lines(content) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// detectTerminal identifies the terminal emulator (and version when available)
// from environment variables. getenv is injected for testability.
//
// $TERM_PROGRAM/$TERM_PROGRAM_VERSION cover iTerm2, Apple Terminal, ghostty, and
// VS Code; $VTE_VERSION covers GNOME Terminal and other VTE-based emulators; we
// fall back to $TERM. Returns "" when nothing is set.
func detectTerminal(getenv func(string) string) string {
	if p := getenv("TERM_PROGRAM"); p != "" {
		if v := getenv("TERM_PROGRAM_VERSION"); v != "" {
			return p + " " + v
		}
		return p
	}
	if v := getenv("VTE_VERSION"); v != "" {
		return "VTE " + v
	}
	if t := getenv("TERM"); t != "" {
		return t
	}
	return ""
}

// buildIssueURL constructs a prefilled "new issue" URL against the bug-report
// form. Empty fields are omitted so they stay blank (and required ones keep
// nagging the reporter) rather than being filled with a stray value.
func buildIssueURL(env bugEnv, description string) string {
	q := url.Values{}
	q.Set("template", "bug_report.yml")
	if description != "" {
		q.Set("what-happened", description)
	}
	if env.Version != "" {
		q.Set("version", "treetop "+env.Version)
	}
	if env.OS != "" {
		q.Set("os", env.OS)
	}
	if env.OSVersion != "" {
		q.Set("os-version", env.OSVersion)
	}
	if env.Terminal != "" {
		q.Set("terminal", env.Terminal)
	}
	return repoURL + "/issues/new?" + q.Encode()
}

// openBrowser opens url in the host's default browser without blocking. It's the
// production opener passed to runBugReport.
func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

// runBugReport drives the `treetop bug` flow: show the detected environment,
// take a one-line description, then open (and always print) a prefilled issue
// URL. opener may be nil to skip the browser launch (the URL is still printed).
func runBugReport(stdin io.Reader, stdout io.Writer, opener func(string) error) error {
	env := collectBugEnv()

	fmt.Fprintln(stdout, "Filing a treetop bug report. Detected environment:")
	fmt.Fprintf(stdout, "  treetop:  %s\n", env.Version)
	fmt.Fprintf(stdout, "  OS:       %s\n", strings.TrimSpace(env.OS+" "+env.OSVersion))
	fmt.Fprintf(stdout, "  terminal: %s\n", env.Terminal)
	fmt.Fprintln(stdout)

	fmt.Fprint(stdout, "Briefly describe the bug (Enter to skip and write it in the browser): ")
	description := ""
	if sc := bufio.NewScanner(stdin); sc.Scan() {
		description = strings.TrimSpace(sc.Text())
	}

	target := buildIssueURL(env, description)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Opening a prefilled bug report in your browser. Review and submit it there.")
	fmt.Fprintln(stdout, "If it doesn't open, copy this URL:")
	fmt.Fprintln(stdout, "  "+target)

	if opener != nil {
		if err := opener(target); err != nil {
			fmt.Fprintln(stdout, "\n(couldn't launch a browser automatically — use the URL above)")
		}
	}
	return nil
}
