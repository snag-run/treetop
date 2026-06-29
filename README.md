# treetop

*A project by [snag.run](https://snag.run).*

A `top`-style tracker for your git worktrees across projects. See every worktree,
its branch, which ones are in use, and when each last changed — in one view.

If you juggle lots of worktrees (multiple branches in flight, agents working in
parallel), it's easy to lose track of what's where. `treetop` scans your repos,
groups worktrees by project, marks the ones that have a session running inside
them, and shows how recently each one changed.

```
$ treetop snag
snag
  ● ~/snag-wt1       fix/234-surface-truncation-error  edited 8s   · changed 15h
  ● ~/snag-docs      feat/renderer-host-brand          edited 23m  · changed 23m
    ~/snag-wt-deps   chore/deps-batch-majors           edited 14h  · changed 14h
    ~/snag           (bare)                            edited —    · changed 1d
```

`●` marks a worktree that is **in use** (a live session is running there); a
worktree with no marker is **open**. The two time columns are distinct signals:
**edited** is the newest working-tree file change (including unstaged edits —
e.g. an agent mid-task), while **changed** is the last git activity (commit /
checkout / stage).

## Install

### Download a binary

Prebuilt binaries for Linux and macOS (Intel + Apple Silicon) are attached to
each [release](https://github.com/snag-run/treetop/releases/latest). Grab the one
for your platform, drop it on your `PATH`, and make it executable:

```sh
# Detect your platform (linux/darwin, amd64/arm64) and the latest release:
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
  https://github.com/snag-run/treetop/releases/latest | sed 's#.*/tag/v##')

curl -fsSL -o treetop \
  "https://github.com/snag-run/treetop/releases/download/v${VERSION}/treetop_${VERSION}_${OS}_${ARCH}"
chmod +x treetop
sudo mv treetop /usr/local/bin/
```

On macOS, Gatekeeper may quarantine the binary on first run; clear it with
`xattr -d com.apple.quarantine /usr/local/bin/treetop`.

Verify the download against the published checksums (optional):

```sh
curl -fsSL -O "https://github.com/snag-run/treetop/releases/latest/download/SHA256SUMS"
shasum -a 256 -c SHA256SUMS --ignore-missing
```

### Install with Go

```sh
go install github.com/snag-run/treetop@latest
```

### Build from source

```sh
git clone https://github.com/snag-run/treetop
cd treetop
go build -o treetop .
```

## Usage

`treetop` has two modes:

- **Snapshot** (default) — print once and exit. The plain CLI.
- **Live** (`-w` / `--watch`) — a full-screen, scrollable dashboard that
  refreshes in place, like `top`. It uses the terminal's alternate screen (your
  scrollback is left untouched on exit) and shows a header with live counts
  (projects / worktrees / in use).

  Scroll with the **mouse wheel**, **↑/↓** (or `j`/`k`), **PgUp/PgDn**, and
  **g**/**G** (top/bottom). Quit with **q** or **Ctrl-C**.

  Press **`/`** to open a filter box and type to narrow the projects live —
  using the same case-insensitive grep/regex syntax as the CLI pattern (so
  `snag|athanor` alternation works). **Enter** keeps the filter applied while
  you scroll, **Esc** clears it, **Backspace** edits. The box is unavailable
  when you launch with a CLI pattern (positional or `-e`) — the filter is
  already pinned, so there's nothing to type into.

```sh
treetop                  # snapshot of every project's worktrees
treetop snag             # filter to projects whose name matches "snag"
treetop 'snag|athanor'   # match either project (regex alternation)
treetop -e snag -e athanor  # same, grep-style; -e is repeatable
treetop -p               # collapse to one line per project
treetop -w               # live mode, refreshing every 2s
treetop -w -i 5          # live mode, every 5s
treetop -w -p            # live, collapsed to projects
treetop -w -e snag -e athanor  # live mode, multiple projects
treetop --in-use         # only worktrees with a live session
treetop --open           # only worktrees with no session
treetop --root ~/code    # scan a specific directory (repeatable)
treetop --pr -e snag     # show PR check status (needs gh; needs a filter)
```

The project filter is a **case-insensitive regular expression** matched against
project names. Pass it positionally or with `-e/--regexp`; give several patterns
(or use `|` alternation) to match more than one project. Quote any pattern
containing `|` so your shell doesn't treat it as a pipe.

A collapsed (`-p`) view shows the worktree count, how many are in use, and the
most recent change per project — handy when you have many projects:

```
$ treetop -p
  ● snag      3/10 in use   edited 14s
    athanor   0/7 in use    edited 2w
    ...
```

| Flag | Description |
|------|-------------|
| `-w`, `--watch` | Live mode: refresh continuously (also `--live`) |
| `-i`, `--interval N` | Refresh interval in seconds (with `--watch`, default 2) |
| `-p`, `--projects` | Collapse to one line per project (no worktrees) |
| `--pr` | Show a PR check-status glyph per worktree (needs `gh`; polls only when filtered, max 5 projects) |
| `--checks` | Expand `--pr` into one row per CI check under each worktree (implies `--pr`) |
| `--notify` | With `--watch`, raise a desktop notification when a PR is approved or sent back for changes, or CI fails (implies `--pr`) |
| `--in-use` | Show only worktrees with a live session |
| `--open` | Show only worktrees with no session |
| `--root DIR` | Directory to scan for repos (repeatable; default `$HOME`) |
| `--depth N` | Levels below each root to scan for repos (default 1, max 3) |
| `--no-color` | Disable ANSI color (also honors `NO_COLOR`) |

By default `treetop` scans `$HOME` one level deep for git worktrees and groups
them by repository. A bare repo is discovered via any of its linked worktrees.
For nested layouts like `~/src/<host>/<org>/<repo>`, raise `--depth` (a repo is
never descended into, so the cost of a deeper scan is just the directory stats).

### Vocabulary

- **in use** — a worktree with a live session running inside it (marked `●`).
- **open** — a worktree with no session.
- **edited** — the newest working-tree file change, including unstaged edits (so
  an agent editing files shows up immediately), in compact wording (`12s`, `5m`,
  `2d`). Shown as `—` when nothing is present (e.g. a bare repo).
- **changed** — the most recent git activity in the worktree (commit / checkout
  / stage).

## PR check status (`--pr`)

With `--pr`, each worktree gets a glyph for the rolled-up CI status of the open
pull request whose head is that worktree's branch. The status comes from the
[`gh`](https://cli.github.com) CLI, so `gh` must be installed and authenticated.
When `gh` is missing or unauthenticated, the column is blank (never an error) and
the header says why — e.g. `PR checks: gh not authenticated — run gh auth login`.
A repo with no GitHub remote stays quietly blank. In `--projects` view the glyph
is the worst status across the project's worktrees. The open PR's number is shown
in its own column after the branch as `#123`, coloured by its review state.

| Glyph | Meaning |
|-------|---------|
| `✓` (green) | all checks passed |
| `✗` (red) | at least one check failed |
| `●` (yellow) | at least one check still running / queued |
| `○` (dim) | a PR with only skipped/neutral checks, or none configured |
| (blank) | no open PR for this branch (or polling is off — see below) |

The status folds **worst-wins**: one failing check among many passing ones shows
`✗`. A PR with an empty check set is `○`, never `✓` — "no checks" is not "passing".

The `#123` number is coloured by the PR's review state, so a glance tells you what
needs attention:

| Number | Review state |
|--------|--------------|
| `#123` (green) | approved |
| `#123` (red) | changes requested |
| `#123` (yellow) | review requested, still undecided |
| `#123` (dim) | draft |
| `#123` (plain) | open, no review decision yet |

Only open PRs are tracked, so there are no merged/closed colours — once a PR
merges, its branch and worktree are usually gone.

### Expanding the checks (`--checks`)

`--checks` keeps the rollup glyph and adds one indented row per individual check
beneath the worktree, so you can see *which* check is red rather than just that
something is:

```
treetop
  ● ✗ ~/snag/feature   feature/login   edited 2m · changed 5m
      ✗ lint
      ● test (integration)
      ✓ build
```

Rows are sorted worst-first (failures lead) and reuse the same glyph palette as
the rollup. It implies `--pr` (same `gh` polling and gating) and applies to the
full view only — `--projects` stays one line per project. The per-check data
rides along on the same `gh` call, so expanding costs no extra requests.

In `--watch`, the `c` key toggles the expansion live (so you don't have to
relaunch), with `--checks` just setting the initial state. Expansion is **gated
to the poll cap**: it's only available once the view is narrowed to
`maxPRPollProjects` (5) or fewer projects — the same set that actually gets CI
data. Above the cap the `c` hint disappears and the header's "first 5 of N —
narrow further" note nudges you to filter, rather than rendering a half-populated
wall of rows.

**Polling is gated to avoid a request storm.** Each polled project costs one `gh`
call, so `--pr` only polls when the list is **filtered** (a pattern, the live `/`
box, `--in-use`, or `--open`); an unfiltered `$HOME` scan would otherwise fire a
`gh` call per repo on every refresh. Even when filtered, at most **5 projects**
are polled per refresh (the header says when more matched). Results are cached
for ~15s, so in `--watch` the table keeps refreshing at its normal interval while
`gh` is hit only occasionally.

### Desktop notifications (`--notify`)

Watch mode is passive — you have to be looking at it to catch a PR get "changes
requested" or CI go red. `--notify` (with `--watch`) pushes the events that
actually need a human as desktop notifications, so treetop can sit in a pane and
ping only when something crosses a meaningful boundary:

- a PR is **approved**,
- a PR is sent back for **changes**, or
- its **CI fails**.

CI notifies on the **rolled-up** status, not individual checks (one PR with a
dozen checks is one signal, not a dozen), and only once the run has **settled**
to a failure — so you don't get pinged the instant the first check goes red while
the rest are still running. Because a notification only fires on a real *change*
into one of these states, a steady red PR never re-pings — but a re-pushed branch
whose new run fails is a genuinely fresh failure and pings again. The state you're
already in when treetop launches never fires — only changes after that do.

`--notify` implies `--pr`, so the same `gh` polling and filter gating apply: it
notifies only for the worktrees actually being polled (a filtered list, max 5
projects). Notifications are delivered as `OSC 9` escape sequences, which
terminals such as **Ghostty**, iTerm2, WezTerm, and kitty render as real system
notifications (inside `tmux` they're wrapped for passthrough, which needs
`set -g allow-passthrough on`). Terminals without `OSC 9` support simply show
nothing.

## How in-use detection works (and its limits)

`treetop` decides a worktree is **in use** from two independent signals:

1. **A live-session scan** (best-effort; Linux via `/proc`, macOS via
   `ps`+`lsof`). It finds live Claude Code and Codex processes and marks a
   worktree if the process is working at or below it — either by its working
   directory, or by a file it currently holds open. The open-file check is what
   lets `treetop` catch **subagents**, which may not `chdir` into the worktree
   they target. Because an open descriptor is transient, a worktree stays marked
   for 30s after the signal last appeared, so the `●` doesn't flicker.
2. **A `.treetop-inuse` marker file** at the worktree root. This is the
   deterministic, cross-platform signal: whatever drops the marker — not
   `treetop` — reports the activity. The marker's first line may be the owning
   process's PID, in which case it's honoured only while that process is alive
   and still looks like an agent session — so a stale marker from a crashed
   writer is ignored, even if the OS later recycles its PID to something else.

Known limits:

- The live-session scan runs on **Linux and macOS**. Windows isn't supported —
  run `treetop` under WSL. (Elsewhere the scan is skipped and only the marker
  file works; a blank/`?` then means *unknown*, not *definitely open*.)
- A session whose working directory has drifted out of every worktree, and which
  holds no files open under one, won't be counted by the scan alone.
- Watch mode (`-w`) uses the terminal's alternate screen and restores it on a
  normal quit and on catchable signals (SIGINT/SIGTERM/SIGHUP/SIGQUIT). A
  `kill -9` or hard crash is untrappable, so it can leave your terminal on the
  alternate screen — run `reset` (or `tput rmcup`) to recover.

### Marking agent worktrees in use

The included hooks drop and remove the `.treetop-inuse` marker as agent sessions
or subagents start and stop, so worktrees an agent is working in light up even
when process scanning is unavailable or misses the activity:

```sh
# Claude Code global install (default provider)
hooks/install.sh --global

# Claude Code repo install, committable alongside the repo
hooks/install.sh --repo .

# Codex global install
hooks/install.sh --codex --global

# Codex repo install, committable alongside the repo
hooks/install.sh --codex --repo .

# Remove again (same provider + scope)
hooks/install.sh --codex --global --uninstall
```

Claude Code installs write to `~/.claude/settings.json` or
`<repo>/.claude/settings.json` and install scripts under the matching
`.claude/hooks/` directory. Codex installs write to `~/.codex/hooks.json` or
`<repo>/.codex/hooks.json` and install scripts under the matching
`.codex/hooks/` directory. The installer merges into existing settings (it never
clobbers other hooks), is idempotent, and for `--global` can also add
`.treetop-inuse` to your global gitignore so the marker doesn't litter your
repos. Requires `jq`.

Claude Code hooks key on `SubagentStart` / `SubagentStop`. Codex hooks key on
`SessionStart` / `Stop` and `SubagentStart` / `SubagentStop`; repo-local Codex
hooks only run after Codex trusts the project config. If several subagents share
one worktree, the first to stop clears the marker early — the live-session scan
covers that gap.

## License

[MIT](LICENSE)
