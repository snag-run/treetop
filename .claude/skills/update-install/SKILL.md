---
name: update-install
description: Build treetop from the current checkout and install it over the existing on-PATH binary, on Linux or macOS. Use when the user wants to update, refresh, or reinstall their local treetop with the latest build from source (e.g. after merging a fix), rather than waiting for a tagged release.
---

# Update the local treetop install

Rebuild `treetop` from the current source tree and replace the binary already on
the user's `PATH`. Works the same on Linux and macOS.

This installs an **unreleased dev build straight from source**. For a tagged
release, download the prebuilt binary from the GitHub release instead.

## Steps

1. **Build from the repo root** (you must be inside the treetop checkout):

   ```sh
   go build -trimpath -o /tmp/treetop-build .
   ```

   If the build fails, stop and surface the error — never install a stale or
   partial binary over a working one.

2. **Find the current install path** — reinstall over the existing copy so you
   don't leave two `treetop` binaries on `PATH`:

   ```sh
   TARGET="$(command -v treetop || true)"
   ```

   If empty, treetop isn't installed yet; default to `/usr/local/bin/treetop`
   unless the user names another directory that's on their `PATH`.

3. **Install, handling permissions.** `install` ships on both Linux and macOS
   and sets the mode in one step:

   ```sh
   install -m 0755 /tmp/treetop-build "$TARGET"
   ```

   If the target directory isn't writable, it needs `sudo`. **Don't run `sudo`
   yourself** — passwordless sudo usually isn't configured and the password
   prompt is interactive. Check writability, and if it fails, ask the user to
   run the command in their own session with the `!` prefix:

   ```sh
   test -w "$(dirname "$TARGET")" || echo "needs sudo: ! sudo install -m 0755 /tmp/treetop-build $TARGET"
   ```

4. **Verify** the new binary is live:

   ```sh
   treetop --version
   ```

   A source build stamps a pseudo-version from Go's VCS build info, like
   `v0.2.1-0.20260627222925-71b992a2a45a` — the trailing short commit
   (`71b992a`) is the `HEAD` you built from, so it confirms the update took. It
   only falls back to `treetop dev` when the build carries no VCS info (built
   outside the git checkout, or with `-buildvcs=false`). Also confirm the binary
   resolves to the path you installed (`command -v treetop`), and report the
   version back to the user.

## Platform notes

- `/usr/local/bin` typically needs `sudo` on Linux and on Intel macOS.
- Homebrew on Apple Silicon uses `/opt/homebrew/bin`, which is usually
  user-writable — no `sudo` needed there.
- The version string comes from Go build info (`main.go`): a plain `go build`
  in the checkout already stamps the commit as a pseudo-version, so no extra
  `-ldflags` are needed for a local install. Tagged release builds carry a real
  `vX.Y.Z` instead.
