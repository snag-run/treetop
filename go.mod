module github.com/snag-run/treetop

go 1.26

require golang.org/x/term v0.45.0

require golang.org/x/sys v0.47.0 // indirect

// v0.1.0 reads and executes a scanned repository's git config (core.fsmonitor),
// so running treetop over a directory containing a hostile repo could execute
// arbitrary code. Fixed in v0.1.1.
retract v0.1.0
