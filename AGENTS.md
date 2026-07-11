# AGENTS.md

Guidance for AI agents (and humans) working in this repo. `treetop` is a
small Go CLI — a `top`-style tracker for git worktrees across projects.

## Planning workflow (SDLC)

Non-trivial features move through a staged flow before implementation, each
stage producing a durable doc the next stage builds on:

1. **Brief** — `grill-brief` → `docs/brief/<slug>.md`. The *why*: problem,
   users, alternatives, cut-line, success criteria, and assumptions to test.
2. **Spec** — `grill-spec` (interactive) or `to-spec` (no-interview synthesis)
   → `docs/spec/<slug>.md`. The *how*: solution, user stories, and
   implementation + testing decisions; draws its problem/stories from the brief.
3. **Tickets** — `to-tickets` → tracer-bullet vertical slices with blocking
   edges, sized by breadth and tagged HITL/AFK, published to the tracker.
4. **Run plan** — `to-run-plan` → orchestrates the tickets into merge-gated
   waves for multi-agent execution.

Decision & doc homes:

- Glossary / ubiquitous language → `CONTEXT.md` (repo root).
- Cross-cutting, durable decisions → an ADR in `docs/adr/`; feature-scoped
  decisions → the spec's *Implementation Decisions* section; project-wide
  terms → `CONTEXT.md`.

Small changes skip the flow — it earns its keep on features big enough to
warrant a written why/how. All docs are created lazily, when the first
decision in them resolves.

## Build, test, format

CI runs these on Linux and macOS; keep them green before pushing:

```sh
gofmt -l .        # must print nothing (run `gofmt -w .` to fix)
go vet ./...
go test ./...
go build ./...    # or: go build -o treetop .
```

## Commits

- Use **Conventional Commits** (`feat:`, `fix:`, `docs:`, `chore:`, `ci:`,
  `refactor:`, …). release-please derives the version bump and `CHANGELOG.md`
  from these prefixes, so the type is load-bearing — not just a style choice.
- **No trailers.** Do not add `Co-Authored-By:` or `🤖 Generated with …`
  footers to commit messages or PR descriptions. (They add noise to the
  history and bias the LLM reviewer.)

## Pull requests

- `main` is protected — never push to it. Branch, push, and open a PR.
- **Squash-merge only.** The squash title becomes the changelog-facing commit,
  so write it as a clean Conventional Commit summary.
- CodeRabbit auto-review is **off** (see `.coderabbit.yaml`); summon it with a
  `@coderabbitai review` comment. Then babysit the review: for each comment,
  fix it or briefly justify skipping, reply in-thread, and resolve it.
- Make the **smallest change** that satisfies the task. Propose worthwhile
  adjacent work rather than bundling it into the same PR.

## Releases

Releases are automated with release-please (`release-type: go`). Merging the
release PR cuts the `vX.Y.Z` tag, and the release workflow builds and attaches
prebuilt Linux/macOS binaries (amd64 + arm64). Don't tag or build releases by
hand.
