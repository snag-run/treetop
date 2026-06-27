# treetop

A `top`-style tracker for your git worktrees across projects. See every worktree,
its branch, and which ones have a live session — in one view.

If you juggle lots of worktrees (multiple branches in flight, agents working in
parallel), it's easy to lose track of what's where. `treetop` scans your repos,
groups worktrees by project, and flags the ones that have an active session
running inside them.

```
$ treetop snag
snag
  ● ~/snag-wt1   fix/234-surface-truncation-error
  ● ~/snag-docs  feat/renderer-host-brand
    ~/snag-wt-deps   chore/deps-batch-majors
    ~/snag           (bare)
```

`●` marks a worktree with a live session.

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
- **Live** (`-w` / `--watch`) — refresh continuously, like `top`.

```sh
treetop                 # snapshot of every project's worktrees
treetop snag            # filter to projects whose name contains "snag"
treetop -w              # live mode, refreshing every 2s
treetop -w -i 5         # live mode, every 5s
treetop --active        # only worktrees with a live session
treetop --inactive      # only idle worktrees
treetop --root ~/code   # scan a specific directory (repeatable)
```

| Flag | Description |
|------|-------------|
| `-w`, `--watch` | Refresh continuously (live mode) |
| `-i`, `--interval N` | Refresh interval in seconds (with `--watch`, default 2) |
| `--active` | Show only worktrees with a live session |
| `--inactive` | Show only idle worktrees |
| `--root DIR` | Directory to scan for repos (repeatable; default `$HOME`) |
| `--no-color` | Disable ANSI color (also honors `NO_COLOR`) |

By default `treetop` scans `$HOME` one level deep for git worktrees and groups
them by repository. A bare repo is discovered via any of its linked worktrees.

## How session detection works (and its limits)

Detection is **best-effort and Linux-only** — it reads `/proc` to find top-level
`claude` processes and maps each to its working directory. A worktree is marked
active if a session's working directory is at or below it.

Known limits:

- **Linux only.** On other platforms the active column shows `?` (unknown).
- **No subagents.** A Claude Code subagent runs in-process inside its parent and
  never `chdir`s into the worktree it targets, so it leaves no per-worktree
  footprint. `treetop` can't see it.
- A blank/`?` means *unknown*, not *definitely idle* — a session whose working
  directory has drifted out of the tree won't be counted.

## License

[MIT](LICENSE)
