//go:build !linux && !darwin

package main

// scanSessions has no implementation on this platform, so live-session
// detection is unavailable. The .treetop-inuse marker file (see marker.go)
// still works everywhere, so hook-driven in-use tracking is unaffected.
func scanSessions() sessionScan {
	return sessionScan{supported: false}
}

// pidIsAgent can't introspect processes on this platform, so it never
// disqualifies a PID marker: a marker whose PID is alive is honoured on
// existence alone (markerActive still checks liveness via pidAlive). This keeps
// the pre-existing behaviour where the .treetop-inuse marker is the only in-use
// signal available here.
func pidIsAgent(pid int) bool {
	return true
}
