package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"sync"
	"time"
)

// CheckState is the rolled-up status of a pull request's CI checks. The values
// are ordered by severity (higher = worse) so the worst entry wins a fold: one
// red check among many greens makes the whole PR red.
type CheckState int

const (
	StateNeutral CheckState = iota // only skipped/neutral checks, or none configured
	StateSuccess                   // at least one check, all passing
	StatePending                   // at least one check still running/queued
	StateFailure                   // at least one check failed
)

// maxPRPollProjects caps how many projects are queried for PR status on a single
// refresh. Each project costs one `gh` subprocess + network round-trip, so this
// bounds the per-tick request volume even when a loose filter matches many
// repos. Polling is already gated on an active filter (see shouldPollPR); this
// is the safety net for a filter that still matches a lot.
const maxPRPollProjects = 5

// prFetchConcurrency bounds how many `gh` calls run at once. The projects polled
// are few (<= maxPRPollProjects), but running them in parallel keeps a slow
// network from serialising into a refresh-stalling delay.
const prFetchConcurrency = 5

// prFetchTimeout bounds a single `gh pr list` call. It mirrors the bound on
// treetop's git calls: this runs on the dashboard refresh path and a wedged
// network or hung gh must not stall it.
const prFetchTimeout = 4 * time.Second

// prCacheTTL decouples PR polling from the dashboard refresh: the table refreshes
// every couple of seconds (local git + sessions), but gh is network I/O and CI
// status barely changes second-to-second, so a repo's PR status is re-fetched at
// most this often. Cached states are applied on every refresh in between.
const prCacheTTL = 15 * time.Second

// prCacheStale bounds how long a cache entry survives without a refresh before
// it's swept. A polled repo refreshes its entry every prCacheTTL, so anything
// much older belongs to a project no longer in view; sweeping keeps the cache
// roughly the size of the active set rather than growing across a long session.
const prCacheStale = 10 * prCacheTTL

// ghRollupEntry is one entry of a PR's statusCheckRollup. GitHub mixes two entry
// shapes with different field names; both are normalised by checkStateOf:
//   - CheckRun (GitHub Actions): __typename "CheckRun", status + conclusion
//   - StatusContext (external CI): __typename "StatusContext", state
type ghRollupEntry struct {
	Typename   string `json:"__typename"`
	Status     string `json:"status"`     // CheckRun: QUEUED/IN_PROGRESS/COMPLETED
	Conclusion string `json:"conclusion"` // CheckRun: SUCCESS/FAILURE/... (empty until COMPLETED)
	State      string `json:"state"`      // StatusContext: SUCCESS/PENDING/FAILURE/ERROR/EXPECTED
}

// ghPR is one open pull request as returned by `gh pr list --json`.
type ghPR struct {
	Number      int             `json:"number"`
	HeadRefName string          `json:"headRefName"`
	Rollup      []ghRollupEntry `json:"statusCheckRollup"`
}

// checkStateOf normalises a single rollup entry into a CheckState, folding the
// two GitHub entry shapes (CheckRun, StatusContext) into one enum.
func checkStateOf(e ghRollupEntry) CheckState {
	// A CheckRun that hasn't completed has no conclusion yet: it's pending,
	// regardless of the (empty) conclusion field.
	if e.Typename == "CheckRun" && e.Status != "COMPLETED" {
		return StatePending
	}
	v := e.Conclusion
	if v == "" {
		v = e.State // StatusContext (or a CheckRun with an unexpected empty conclusion)
	}
	switch v {
	case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED":
		return StateFailure
	case "PENDING", "QUEUED", "IN_PROGRESS", "EXPECTED":
		return StatePending
	case "SUCCESS":
		return StateSuccess
	default: // SKIPPED, NEUTRAL, or anything unrecognised
		return StateNeutral
	}
}

// rollupCheckState folds a PR's rollup entries into a single worst-wins state.
// An empty rollup is StateNeutral, never StateSuccess: a PR with no configured
// checks must not masquerade as passing.
func rollupCheckState(entries []ghRollupEntry) CheckState {
	worst := StateNeutral
	for _, e := range entries {
		if s := checkStateOf(e); s > worst {
			worst = s
		}
	}
	return worst
}

// ghFetchPRChecks runs `gh pr list` in dir and returns a map from each open PR's
// head branch to its rolled-up CheckState. ok is false (with the map nil) when
// gh is missing, unauthenticated, the directory has no GitHub remote, or the call
// times out: PR status is best-effort enrichment and must never break a refresh.
// A successful call with no open PRs returns an empty map with ok true, so the
// cache can remember "this repo has none" rather than retrying every tick.
func ghFetchPRChecks(dir string) (map[string]CheckState, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--state", "open", "--limit", "100",
		"--json", "number,headRefName,statusCheckRollup")
	cmd.Dir = dir
	// gh shells out to git to resolve the repo from its remote. Harden that child
	// git the same way git.go does: a scanned repo is untrusted, and git runs
	// config-named programs (notably core.fsmonitor) during ordinary operations.
	cmd.Env = hardenedGitEnv()

	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	var prs []ghPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, false
	}
	checks := make(map[string]CheckState, len(prs))
	for _, pr := range prs {
		if pr.HeadRefName == "" {
			continue
		}
		checks[pr.HeadRefName] = rollupCheckState(pr.Rollup)
	}
	return checks, true
}

type prCacheEntry struct {
	at     time.Time
	checks map[string]CheckState
}

var (
	prCacheMu sync.Mutex
	prCache   = map[string]prCacheEntry{}
)

// fetchPRChecks returns a repo's branch->CheckState map, served from a
// prCacheTTL cache so gh is hit at most once per TTL per repo regardless of how
// often the dashboard refreshes. On a fetch failure it falls back to the last
// cached value (stale is better than blank), or nil if there is none.
func fetchPRChecks(dir string) map[string]CheckState {
	now := time.Now()

	prCacheMu.Lock()
	if e, ok := prCache[dir]; ok && now.Sub(e.at) < prCacheTTL {
		prCacheMu.Unlock()
		return e.checks
	}
	prCacheMu.Unlock()

	checks, ok := ghFetchPRChecks(dir)
	if !ok {
		// Fetch failed: keep showing the last good data if we have any.
		prCacheMu.Lock()
		defer prCacheMu.Unlock()
		return prCache[dir].checks
	}

	prCacheMu.Lock()
	prCache[dir] = prCacheEntry{at: now, checks: checks}
	for path, e := range prCache { // sweep entries for repos no longer in view
		if now.Sub(e.at) > prCacheStale {
			delete(prCache, path)
		}
	}
	prCacheMu.Unlock()
	return checks
}

// enrichPRChecks stamps PR check status onto the worktrees of the first
// maxPRPollProjects projects (already sorted by name, so the selection is
// deterministic). It returns the number of projects polled, so the caller can
// tell the user when a filter matched more projects than were polled.
//
// Projects are queried concurrently (bounded by prFetchConcurrency) because each
// gh call is network-bound; serialising them could push a refresh past its
// interval.
func enrichPRChecks(projects []Project) (polled int) {
	polled = len(projects)
	if polled > maxPRPollProjects {
		polled = maxPRPollProjects
	}

	sem := make(chan struct{}, prFetchConcurrency)
	var wg sync.WaitGroup
	for i := 0; i < polled; i++ {
		p := projects[i]
		if len(p.Worktrees) == 0 {
			continue
		}
		wg.Add(1)
		go func(p Project) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// Any worktree path works: gh resolves the repo from the remote.
			checks := fetchPRChecks(p.Worktrees[0].Path)
			for j := range p.Worktrees {
				if s, ok := checks[p.Worktrees[j].Branch]; ok {
					p.Worktrees[j].Check = s
					p.Worktrees[j].HasPR = true
				}
			}
		}(p)
	}
	// Refresh the gh-health signal on the same (background) refresh path, so the
	// header can explain a blank column when gh is missing or unauthenticated
	// rather than leaving the user guessing. Cached, so it's at most one extra
	// `gh auth status` per ghHealthTTL.
	refreshGHHealth()
	wg.Wait()
	return polled
}

// ghHealthTTL bounds how often the gh-health probe runs. Auth state rarely
// changes, so a stale-by-this-much answer is fine and spares a subprocess on
// every refresh.
const ghHealthTTL = 30 * time.Second

var (
	ghHealthMu   sync.Mutex
	ghHealthAt   time.Time
	ghHealthNote string // "" when gh is usable; otherwise a header-ready note
)

// refreshGHHealth probes whether gh is usable (installed + authenticated) and
// caches a header note describing the problem, or "" when healthy. It runs on
// the refresh goroutine; the render loop only ever reads the cached note via
// ghProblemNote, so it never blocks the UI on a subprocess.
func refreshGHHealth() {
	ghHealthMu.Lock()
	fresh := !ghHealthAt.IsZero() && time.Since(ghHealthAt) < ghHealthTTL
	ghHealthMu.Unlock()
	if fresh {
		return
	}

	note := probeGHHealth()

	ghHealthMu.Lock()
	ghHealthNote, ghHealthAt = note, time.Now()
	ghHealthMu.Unlock()
}

// probeGHHealth returns "" when gh is installed and authenticated, otherwise a
// note explaining which is missing. A missing binary is checked first (cheap,
// no subprocess); auth is verified with `gh auth status`, which exits non-zero
// when no host is logged in.
func probeGHHealth() string {
	if _, err := exec.LookPath("gh"); err != nil {
		return "PR checks: gh not found on PATH — install the GitHub CLI"
	}
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "auth", "status")
	cmd.Env = hardenedGitEnv()
	if err := cmd.Run(); err != nil {
		return "PR checks: gh not authenticated — run `gh auth login`"
	}
	return ""
}

// ghProblemNote returns the cached gh-health note (empty when gh is usable or
// hasn't been probed yet). Safe to call from the render loop: it only reads.
func ghProblemNote() string {
	ghHealthMu.Lock()
	defer ghHealthMu.Unlock()
	return ghHealthNote
}

// shouldPollPR reports whether PR status should be fetched: the --pr flag is set
// and the project list has been narrowed by some filter. Without a filter, --pr
// is dormant — polling every repo under $HOME on each tick would be a request
// storm. live is the compiled live-mode filter (watch's "/" box); the other
// filters come from opts.
func shouldPollPR(opts options, live []*regexp.Regexp) bool {
	return opts.pr && prFilterActive(opts, len(live) > 0)
}

// prFilterActive reports whether any project-narrowing filter is in effect.
// liveActive is whether the watch-mode live filter currently holds a query.
func prFilterActive(opts options, liveActive bool) bool {
	return len(opts.patterns) > 0 || liveActive || opts.onlyInUse || opts.onlyOpen
}

// projectWorstCheck folds a project's worktree PR states into one worst-wins
// state for the compact (one-line-per-project) view. ok is false when no
// worktree in the project has a PR.
func projectWorstCheck(p Project) (state CheckState, ok bool) {
	for _, w := range p.Worktrees {
		if !w.HasPR {
			continue
		}
		if !ok || w.Check > state {
			state, ok = w.Check, true
		}
	}
	return state, ok
}
