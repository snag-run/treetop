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
  ● ~/snag-wt1   fix/234-surface-truncation-error  15 hours ago
  ● ~/snag-docs  feat/renderer-host-brand          23 minutes ago
    ~/snag-wt-deps   chore/deps-batch-majors        14 hours ago
    ~/snag           (bare)                         1 day ago
```

`●` marks a worktree that is **in use** (a live session is running there); a
worktree with no marker is **open**. The last column shows when the worktree
last changed (last commit / checkout / stage).

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
- **Live** (`-w` / `--watch`) — a full-screen dashboard that refreshes in place,
  like `top`. It uses the terminal's alternate screen (your scrollback is left
  untouched on exit), shows a header with live counts (projects / worktrees /
  in use), and exits cleanly on Ctrl-C.

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
  ● snag      3/10 in use   29 minutes ago
    athanor   0/7 in use    2 weeks ago
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
- **last changed** — the most recent git activity in the worktree (commit /
  checkout / stage), shown in natural wording ("just now", "5 minutes ago").

## How in-use detection works (and its limits)

Detection is **best-effort and Linux-only** — it reads `/proc` to find top-level
`claude` processes and maps each to its working directory. A worktree is marked
in use if a session's working directory is at or below it.

Known limits:

- **Linux only.** On other platforms the in-use marker shows `?` (unknown).
- **No subagents.** A Claude Code subagent runs in-process inside its parent and
  never `chdir`s into the worktree it targets, so it leaves no per-worktree
  footprint. `treetop` can't see it.
- A blank/`?` means *unknown*, not *definitely open* — a session whose working
  directory has drifted out of the tree won't be counted.

## License

[MIT](LICENSE)
