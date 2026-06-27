//go:build !linux && !darwin

package main

// scanSessions has no implementation on this platform, so live-session
// detection is unavailable. The .treetop-inuse marker file (see marker.go)
// still works everywhere, so hook-driven in-use tracking is unaffected.
func scanSessions() sessionScan {
	return sessionScan{supported: false}
}
