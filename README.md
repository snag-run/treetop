# treetop

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

```sh
go install github.com/snag-run/treetop@latest
```

Or build from source:

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

```sh
treetop                 # snapshot of every project's worktrees
treetop snag            # filter to projects whose name contains "snag"
treetop -p              # collapse to one line per project
treetop -w              # live mode, refreshing every 2s
treetop -w -i 5         # live mode, every 5s
treetop -w -p           # live, collapsed to projects
treetop --in-use        # only worktrees with a live session
treetop --open          # only worktrees with no session
treetop --root ~/code   # scan a specific directory (repeatable)
```

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
| `--in-use` | Show only worktrees with a live session |
| `--open` | Show only worktrees with no session |
| `--root DIR` | Directory to scan for repos (repeatable; default `$HOME`) |
| `--no-color` | Disable ANSI color (also honors `NO_COLOR`) |

By default `treetop` scans `$HOME` one level deep for git worktrees and groups
them by repository. A bare repo is discovered via any of its linked worktrees.

### Vocabulary

- **in use** — a worktree with a live session running inside it (marked `●`).
- **open** — a worktree with no session.
- **edited** — the newest working-tree file change, including unstaged edits (so
  an agent editing files shows up immediately), in compact wording (`12s`, `5m`,
  `2d`). Shown as `—` when nothing is present (e.g. a bare repo).
- **changed** — the most recent git activity in the worktree (commit / checkout
  / stage).

## How in-use detection works (and its limits)

`treetop` decides a worktree is **in use** from two independent signals:

1. **A `/proc` scan** (best-effort, Linux-only). It finds live `claude`
   processes and marks a worktree if the process is working at or below it —
   either by its working directory, or by a file it currently holds open. The
   open-file check is what lets `treetop` catch **subagents**, which run
   in-process and never `chdir` into the worktree they target. Because an open
   descriptor is transient, a worktree stays marked for 30s after the signal
   last appeared, so the `●` doesn't flicker.
2. **A `.treetop-inuse` marker file** at the worktree root. This is the
   deterministic, cross-platform signal: whatever drops the marker — not
   `treetop` — reports the activity. The marker's first line may be the owning
   process's PID, in which case it's honoured only while that process is alive
   (a stale marker from a crashed writer is ignored).

Known limits:

- The `/proc` scan is **Linux only**; elsewhere the marker file still works. A
  blank/`?` means *unknown*, not *definitely open*.
- A session whose working directory has drifted out of every worktree, and which
  holds no files open under one, won't be counted by the `/proc` scan alone.

### Marking agent (subagent) worktrees in use

The included Claude Code hooks drop and remove the `.treetop-inuse` marker as
subagents start and stop, so worktrees an agent is working in light up even
though they leave no other footprint:

```sh
# Global: fires in every project (recommended for cross-project tracking)
hooks/install.sh --global

# Repo: scoped to one repository, committable alongside it
hooks/install.sh --repo .

# Remove again (per scope)
hooks/install.sh --global --uninstall
```

Claude Code **merges hooks from every scope**, so global and repo installs are
independent and additive — pick one or run both. `--global` writes to
`~/.claude/settings.json` and installs the scripts under `~/.claude/hooks/`;
`--repo` writes to `<repo>/.claude/settings.json` and references the scripts via
`$CLAUDE_PROJECT_DIR`, so committing `.claude/` shares them with everyone who
clones the repo. The installer merges into existing settings (it never clobbers
other hooks), is idempotent, and for `--global` also adds `.treetop-inuse` to
your global gitignore so the marker doesn't litter your repos. Requires `jq`.

The hooks key on `SubagentStart` / `SubagentStop`. If several subagents share one
worktree, the first to stop clears the marker early — the `/proc` scan covers
that gap.

## License

[MIT](LICENSE)
