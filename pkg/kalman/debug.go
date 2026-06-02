package kalman

import "log"

// debug gates per-step EKF trace output (Innovation, Jacobian, S, gain, ...).
// Off by default; enable with SetDebug for filter development (cmd/replay --verbose).
var debug bool

// SetDebug enables or disables per-update log.Printf traces from the filter goroutine.
func SetDebug(on bool) {
	debug = on
}

// Debug reports whether per-step EKF traces are enabled.
func Debug() bool {
	return debug
}

func debugf(format string, args ...any) {
	if debug {
		log.Printf(format, args...)
	}
}
