package main

import "math"

// outlierFilter screens raw measurements before they reach the EKF or the
// UI. Two checks layered on top of NaN/Inf rejection:
//
//	belt:       n_est > outlierMaxN * n0           absolute hard limit.
//	suspenders: n_est > outlierStep * prev_accepted  single-sample step
//	                                                 change vs. last good.
//
// where n_est = ‖k * (m − l)‖ using the filter's current (k, l). The belt
// is context-independent — values >> n0 are physically implausible. The
// suspenders allow gradual environment changes (a magnet brought closer
// over many samples passes) while catching the impulse-style glitches
// observed in practice (e.g. -1200 µT single-axis spike from an I²C
// transmission corruption).
//
// After outlierMaxConsec consecutive rejections the next sample is
// accepted as the new baseline so a real abrupt environment change
// (iPad suddenly placed in the cabin) eventually gets through and lets
// the state machine react.
type outlierFilter struct {
	prevNEst  float64
	havePrev  bool
	consec    int
	rejectedN int
}

const (
	outlierMaxN      = 10.0
	outlierStep      = 2.0
	outlierMaxConsec = 3
)

// check returns true when the measurement is accepted, false when it
// should be dropped. Updates internal state on every call.
func (f *outlierFilter) check(m, k, l []float64, n0 float64) bool {
	n := len(m)
	if n == 0 || len(k) != n || len(l) != n || n0 <= 0 {
		return true
	}
	var sumSq float64
	for i := 0; i < n; i++ {
		if math.IsNaN(m[i]) || math.IsInf(m[i], 0) {
			return f.recordReject(true)
		}
		ni := k[i] * (m[i] - l[i])
		sumSq += ni * ni
	}
	nEst := math.Sqrt(sumSq)
	if math.IsNaN(nEst) || math.IsInf(nEst, 0) {
		return f.recordReject(true)
	}

	if nEst > outlierMaxN*n0 {
		return f.recordReject(false, nEst)
	}
	if f.havePrev && nEst > outlierStep*f.prevNEst {
		return f.recordReject(false, nEst)
	}

	// Accept.
	f.prevNEst = nEst
	f.havePrev = true
	f.consec = 0
	return true
}

// recordReject increments the rejection counter and applies the
// accept-after-N-rejections fallback. resetBaseline is true when the bad
// sample was NaN/Inf (we can't use it as a baseline so we just clear the
// prev tracker); false when the bad sample had a meaningful nEst and we
// can promote it to baseline on overflow.
func (f *outlierFilter) recordReject(resetBaseline bool, nEst ...float64) bool {
	f.rejectedN++
	f.consec++
	if f.consec <= outlierMaxConsec {
		return false
	}
	// Sustained rejection — accept as new baseline so a real environment
	// change is eventually let through.
	if resetBaseline || len(nEst) == 0 {
		f.havePrev = false
	} else {
		f.prevNEst = nEst[0]
		f.havePrev = true
	}
	f.consec = 0
	return true
}

// resetBaseline clears the "last accepted" tracker but preserves the
// running rejection counter. Used when the filter's calibration changes
// (Restart, INIT-Finish seed) and the prior n_est is no longer a
// meaningful reference for the suspenders check.
func (f *outlierFilter) resetBaseline() {
	f.prevNEst = 0
	f.havePrev = false
	f.consec = 0
}
