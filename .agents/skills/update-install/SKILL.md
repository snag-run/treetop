---
name: update-install
description: Build treetop from the current checkout and reinstall it via the repo Makefile on Linux or macOS. Use when the user wants to update, refresh, or reinstall their local treetop with the latest build from source (e.g. after merging a fix), rather than waiting for a tagged release.
---

# Update the local treetop install

Rebuild `treetop` from the current source tree using the repo `Makefile`, then
reinstall it with the project's supported source-install path. Works the same on
Linux and macOS.

This installs an **unreleased dev build straight from source**. For a tagged
release, download the prebuilt binary from the GitHub release instead.

## Steps

1. **Install from the repo root** (you must be inside the treetop checkout):

   ```sh
   make install
   ```

   `make install` delegates to `go install .`, so it builds the current checkout
   and installs `treetop` into Go's bin directory. If it fails, stop and surface
   the error.

2. **Find the install target** so you know where the new binary landed:

   ```sh
   BIN_DIR="$(go env GOBIN)"
   if [ -z "$BIN_DIR" ]; then
     GOPATH="$(go env GOPATH)"
     BIN_DIR="${GOPATH%%:*}/bin"
   fi
   TARGET="$BIN_DIR/treetop"
   ```

3. **Check which binary the shell will run**:

   ```sh
   command -v treetop
   ```

   If this does not match `$TARGET`, the install succeeded but the shell is still
   resolving some other `treetop` earlier on `PATH`. Report both paths back to
   the user. Do not silently overwrite the other binary; that is now outside the
   Makefile-managed flow.

4. **Verify** the new binary is live:

   ```sh
   treetop --version
   ```

   A source build stamps a pseudo-version from Go's VCS build info, like
   `v0.2.1-0.20260627222925-71b992a2a45a` — the trailing short commit
   (`71b992a`) is the `HEAD` you built from, so it confirms the update took. It
   only falls back to `treetop dev` when the build carries no VCS info (built
   outside the git checkout, or with `-buildvcs=false`). Also report the
   resolved binary path (`command -v treetop`) back to the user.

## Platform notes

- `make install` uses Go's install location, not `/usr/local/bin`.
- If `GOBIN` is set, the binary lands in `$GOBIN/treetop`.
- Otherwise it lands in the `bin` directory under the first path entry from
  `go env GOPATH`, which is typically `$HOME/go/bin/treetop`.
- If that directory is not on `PATH`, the install can succeed while `treetop`
  still resolves to an older copy elsewhere.
- The version string comes from Go build info (`main.go`): a plain `go build`
  in the checkout already stamps the commit as a pseudo-version, so no extra
  `-ldflags` are needed for a local install. Tagged release builds carry a real
  `vX.Y.Z` instead.
