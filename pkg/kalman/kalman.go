// Package kalman defines a data structure that aggregates measurements.
// Averages/counts measurements that are "close."
// Can quickly apply the current K & L and return the associated N^2.
package kalman

import (
	"log"
	"math"
)

// Mode is the state-machine phase of the filter.
type Mode int

const (
	// ModeCalibrating: every Z applies a full EKF update. Default after
	// NewKalmanFilter.
	ModeCalibrating Mode = iota
	// ModeLocked: Z is observed (innovation and S still computed for NIS
	// monitoring) but the state x and covariance P are NOT updated.
	// Reached only if the state machine has been enabled via
	// EnableStateMachine.
	ModeLocked
)

func (m Mode) String() string {
	switch m {
	case ModeCalibrating:
		return "CAL"
	case ModeLocked:
		return "LCK"
	}
	return "?"
}

// forceCmd is the internal command type for ForceLock / ForceUnlock,
// dispatched through the runFilter goroutine to avoid racing with state
// reads/writes that happen there.
type forceCmd int

const (
	forceCmdLock forceCmd = iota + 1
	forceCmdUnlock
	forceCmdResetSM
)

// Snapshot is a serializable copy of the filter's full internal state.
// Use Snapshot() to capture and Restore() to reapply (e.g., for
// scenario-replay checkpoints in cmd/websim).
//
// All slices/matrices in a Snapshot are independent copies — modifying
// them does not affect the live filter.
type Snapshot struct {
	X                    Matrix
	P                    Matrix
	Mode                 Mode
	ConsecutiveConverged int
	NISBuf               []float64
	NISIdx               int
	NISCount             int
	NISSum               float64
}

type Filter struct {
	n int               // Number of dimensions
	x Matrix            // Kalman Filter hidden state
	p Matrix            // Kalman Filter hidden state covariance
	q Matrix            // Kalman Filter state noise process
	r Matrix            // Measurement noise
	u Matrix            // Control vector, measured mag vector in this case
	z float64           // Measurement, earth's mag field strength **2
	U chan Matrix        // Channel for sending new control values to Kalman Filter
	Z chan float64       // Channel for sending new measurements to Kalman Filter
	Done chan struct{}   // Signalled (non-blocking) after each Z update or Force*/Restore completes; size-1 buffer
	force chan forceCmd  // Internal channel for ForceLock/ForceUnlock; unbuffered
	restore chan Snapshot // Internal channel for Restore; unbuffered

	// Saved init params so unlockAndInflate can rebuild P's initial diagonals.
	n0      float64
	sigmaK0 float64

	// Convergence thresholds; zero means "not configured, Converged() returns
	// false". See SetConvergenceThresholds and Converged.
	maxSigmaK float64
	maxSigmaL float64

	// State machine; activated by EnableStateMachine. While disabled, the
	// filter always updates and Mode() always returns ModeCalibrating.
	stateMachineEnabled  bool
	mode                 Mode
	lockHysteresis       int     // consecutive Converged()=true samples needed to lock
	consecutiveConverged int     // counter against lockHysteresis
	nisBuf               []float64
	nisIdx               int     // next write position in circular buffer
	nisCount             int     // values currently in buffer (<= len(nisBuf))
	nisSum               float64 // sum of values in buffer (for O(1) mean)
	nisThreshold         float64 // rolling mean above this triggers unlock
}

// NewKalmanFilter returns a Filter struct with Kalman Filter methods for calibrating a magnetometer.
// n is the number of dimensions (1, 2 for testing; 3 for reality)
// n0 is the strength of the Earth's magnetic field at the current location, 1.0 is fine for testing
// sigmaK0 is the initial uncertainty for k (n0*sigmaK0 for l)
// sigmaK is the (small) process uncertainty for k (n0*sigmaK for l)
// sigmaM is the fractional magnetometer measurement noise, so the magnetometer noise is n0*sigmaM
func NewKalmanFilter(n int, n0, sigmaK0, sigmaK, sigmaM float64) (k *Filter) {
	k = new(Filter)
	k.n = n
	k.n0 = n0
	k.sigmaK0 = sigmaK0

	k.x = make(Matrix, 2*n)
	k.p = make(Matrix, 2*n)
	k.q = make(Matrix, 2*n)

	for i := 0; i < n; i++ {
		k.x[2*i] = []float64{1}
		k.x[2*i+1] = []float64{0}

		k.p[2*i] = make([]float64, 2*n)
		k.p[2*i+1] = make([]float64, 2*n)
		k.p[2*i][2*i] = sigmaK0 * sigmaK0
		k.p[2*i+1][2*i+1] = (n0 * sigmaK0) * (n0 * sigmaK0)

		k.q[2*i] = make([]float64, 2*n)
		k.q[2*i+1] = make([]float64, 2*n)
		k.q[2*i][2*i] = sigmaK * sigmaK
		k.q[2*i+1][2*i+1] = (n0 * sigmaK) * (n0 * sigmaK)
	}

	// var(z) where z = ‖n̂‖² and only m is noisy (z itself we feed as n0²
	// exactly). Linearizing ‖n̂‖² in m around truth: var(z) ≈ 4·n0²·(n0·sigmaM)²
	// when k≈1 and Σnᵢ²≈n0². The previous (n0·sigmaM)² value undersized r
	// by a factor of (2·n0)², making the filter wildly overconfident in its
	// measurement under any observation regime.
	sigmaZ := 2 * n0 * n0 * sigmaM
	k.r = Matrix{{sigmaZ * sigmaZ}}

	k.U = make(chan Matrix)
	k.Z = make(chan float64)
	k.Done = make(chan struct{}, 1)
	k.force = make(chan forceCmd)
	k.restore = make(chan Snapshot)

	go k.runFilter()

	return k
}

func (k *Filter) runFilter() {
	var (
		y              float64
		h, s, kk, nHat Matrix
	)

	h = make(Matrix, 1)
	h[0] = make([]float64, 2*k.n)
	id := make(Matrix, 2*k.n)
	for i := 0; i < 2*k.n; i++ {
		id[i] = make([]float64, 2*k.n)
		id[i][i] = 1
	}

	for {
		select {
		case k.u = <-k.U:
			// Calculate estimated measurement
			nHat = calcMagField(k.x, k.u)

			// No evolution for x

			// Evolve p
			for i := 0; i < 2*k.n; i++ {
				for j := 0; j < 2*k.n; j++ {
					k.p[i][j] += k.q[i][j]
				}
			}
		case k.z = <-k.Z:
			// Calculate measurement residual (always — needed for both modes)
			y = k.z
			for i := 0; i < k.n; i++ {
				y -= nHat[i][0] * nHat[i][0]
			}
			log.Printf("Innovation y = %f\n", y)

			// Calculate Jacobian (always — needed for S in both modes)
			for i := 0; i < k.n; i++ {
				h[0][2*i] = 2 * nHat[i][0] * nHat[i][0] / k.x[2*i][0]
				h[0][2*i+1] = -2 * nHat[i][0] * k.x[2*i][0]
			}
			log.Printf("Jacobian H = %v\n", h)

			// Calculate S (always — used for gain in CAL, for NIS in LCK)
			s = matAdd(k.r, matMul(h, matMul(k.p, matTranspose(h))))
			log.Printf("Inn Cov s = %v\n", s)

			// NIS = y²/S is meaningful as a fit-quality metric in CAL too
			// (after each update, "did the filter agree with this sample?"),
			// so we accumulate it regardless of mode. The auto-unlock check
			// still only fires in LCK, and CAL→LCK clears the buffer so
			// post-lock samples drive the unlock decision in isolation.
			if k.stateMachineEnabled && len(k.nisBuf) > 0 {
				k.nisPush(y * y / s[0][0])
			}

			if k.mode == ModeLocked {
				// LOCKED: don't update state or P. The auto-unlock check is
				// gated on the state machine being enabled, so a Force-Lock
				// without a full state machine still freezes the filter
				// (manual override the UI exposes).
				if k.stateMachineEnabled &&
					k.nisCount >= len(k.nisBuf) &&
					k.nisSum/float64(k.nisCount) > k.nisThreshold {
					k.unlockAndInflate()
				}
			} else {
				// CALIBRATING (or state machine disabled): normal EKF update.
				kk = matSMul(1/s[0][0], matMul(k.p, matTranspose(h)))
				log.Printf("Gain kk = %v\n", kk)
				k.x = matAdd(k.x, matSMul(y, kk))
				log.Printf("State Update y*kk = %v\n", matSMul(y, kk))
				k.p = matMul(matAdd(id, matSMul(-1, matMul(kk, h))), k.p)
				log.Printf("Cov Update kk*h = %v\n\n", matMul(matSMul(-1, matMul(kk, h)), k.p))

				// Check for CAL → LCK transition.
				if k.stateMachineEnabled {
					if k.Converged() {
						k.consecutiveConverged++
						if k.consecutiveConverged >= k.lockHysteresis {
							k.mode = ModeLocked
							// Reset the NIS rolling buffer so the LCK→CAL
							// auto-unlock check only sees post-lock samples;
							// the CAL-era values that just accumulated would
							// otherwise dominate the mean and could fire an
							// immediate unlock.
							for i := range k.nisBuf {
								k.nisBuf[i] = 0
							}
							k.nisIdx = 0
							k.nisCount = 0
							k.nisSum = 0
						}
					} else {
						k.consecutiveConverged = 0
					}
				}
			}

			// Non-blocking signal that the update is complete; observers
			// who care about post-update state read from Done after sending
			// to Z. Drain Done before sending Z to avoid stale signals.
			select {
			case k.Done <- struct{}{}:
			default:
			}
		case cmd := <-k.force:
			// External force-lock / force-unlock dispatched here so it
			// doesn't race with U/Z processing or with state reads.
			// Both work independent of whether the auto state machine
			// is enabled — they are the manual override the UI exposes.
			switch cmd {
			case forceCmdLock:
				k.mode = ModeLocked
				k.consecutiveConverged = 0
				// Mirror the natural CAL→LCK transition's NIS buffer
				// reset so the LCK→CAL auto-unlock check needs fresh
				// post-lock samples before it can fire. Without this,
				// pre-lock NIS residuals (the new in-CAL accumulation
				// behaviour) carry over and can trigger a surprise
				// auto-unlock moments after the user clicked Force Lock.
				for i := range k.nisBuf {
					k.nisBuf[i] = 0
				}
				k.nisIdx = 0
				k.nisCount = 0
				k.nisSum = 0
			case forceCmdUnlock:
				k.unlockAndInflate()
			case forceCmdResetSM:
				// Reset just the state-machine bookkeeping (mode, the
				// consecutive-converged counter, the NIS buffer). State
				// x and covariance P are preserved. Used after a bulk
				// feed (e.g. cmd/websim's INIT replay) so that subsequent
				// live measurements drive a fresh CAL phase instead of
				// inheriting the bulk feed's lock progress.
				k.mode = ModeCalibrating
				k.consecutiveConverged = 0
				for i := range k.nisBuf {
					k.nisBuf[i] = 0
				}
				k.nisIdx = 0
				k.nisCount = 0
				k.nisSum = 0
			}
			select {
			case k.Done <- struct{}{}:
			default:
			}
		case s := <-k.restore:
			// External Restore dispatched here so we overwrite x, p, and
			// state-machine fields without racing with anything else.
			k.applyRestoreLocked(s)
			select {
			case k.Done <- struct{}{}:
			default:
			}
		}
	}
}

func calcMagField(x Matrix, u Matrix) (n Matrix) {
	n = make(Matrix, len(u[0]))
	for i := 0; i < len(u[0]); i++ {
		n[i] = []float64{x[2*i][0] * (u[0][i] - x[2*i+1][0])}
	}
	return n
}

func (k *Filter) State() (state Matrix) {
	return k.x
}

func (k *Filter) StateCovariance() (cov Matrix) {
	return k.p
}

func (k *Filter) ProcessNoise() (cov Matrix) {
	return k.q
}

func (k *Filter) SetProcessNoise(q Matrix) {
	k.q = q
}

func (k *Filter) K() (kk []float64) {
	v := make([]float64, k.n)
	for i := 0; i < k.n; i++ {
		v[i] = k.x[2*i][0]
	}
	return v
}

func (k *Filter) L() (l []float64) {
	v := make([]float64, k.n)
	for i := 0; i < k.n; i++ {
		v[i] = k.x[2*i+1][0]
	}
	return v
}

func (k *Filter) P() (p Matrix) {
	return k.p
}

// SetConvergenceThresholds sets the per-axis bounds used by Converged.
// Pass (0, 0) to disable convergence checking (the default after
// NewKalmanFilter, so existing callers see no behavior change).
//
// Thresholds are in state-space units:
//   - maxSigmaK is dimensionless. The k_i are scale factors near 1, so a
//     value of 1.0e-3 means "trust each k_i to within 0.1%".
//   - maxSigmaL is in the same units as n0 (typically nT for geomagnetic
//     use). A value of 5.0 means "trust each l_i to within 5 nT".
//
// The right values depend on the operational target and on real
// magnetometer noise; treat these as tunables to ground against
// hardware data, not as load-bearing constants.
func (k *Filter) SetConvergenceThresholds(maxSigmaK, maxSigmaL float64) {
	k.maxSigmaK = maxSigmaK
	k.maxSigmaL = maxSigmaL
}

// Converged reports whether the calibration is satisfactory by the
// configured thresholds. Returns false if SetConvergenceThresholds has
// not been called (or was called with zeros).
//
// The criterion is per-axis P-diagonal bounds:
//
//	√P[2i][2i]     < maxSigmaK   for all i
//	√P[2i+1][2i+1] < maxSigmaL   for all i
//
// This catches the common "axis was never observed" failure (its
// diagonals stay large), but doesn't fully catch off-diagonal
// degeneracy along an unobservable (kᵢ, lᵢ) ridge — for that we'd need
// max-eigenvalue(P). Adequate for v1; upgrade if hardware data shows
// the diagonals lying about convergence.
func (k *Filter) Converged() bool {
	if k.maxSigmaK == 0 || k.maxSigmaL == 0 {
		return false
	}
	maxVarK := k.maxSigmaK * k.maxSigmaK
	maxVarL := k.maxSigmaL * k.maxSigmaL
	for i := 0; i < k.n; i++ {
		if k.p[2*i][2*i] > maxVarK || math.IsNaN(k.p[2*i][2*i]) {
			return false
		}
		if k.p[2*i+1][2*i+1] > maxVarL || math.IsNaN(k.p[2*i+1][2*i+1]) {
			return false
		}
	}
	return true
}

// EnableStateMachine activates the calibrate-then-lock state machine.
// SetConvergenceThresholds must also be called (and with non-zero
// values) for the CAL→LCK transition to ever fire — without it,
// Converged() always returns false.
//
//   lockHysteresis: consecutive Converged()=true samples required before
//                   transitioning CAL→LCK. 1 locks on first convergence;
//                   higher values reduce flicker. Reasonable default: 10.
//   nisWindow:      rolling-window length for the LCK→CAL trigger's NIS
//                   mean. 100 is a reasonable starting point.
//   nisThreshold:   rolling NIS mean above this unlocks. Under the
//                   filter's Gaussian assumption innovation²/S follows
//                   χ²(1) with expected mean 1; thresholds of 3–6 are
//                   typical. Tune against real-hardware noise.
//
// Invalid arguments (any <= 0) are silently rejected — the filter stays
// in its previous (typically not-enabled) mode. Replay-level validation
// catches and reports these to the user.
func (k *Filter) EnableStateMachine(lockHysteresis, nisWindow int, nisThreshold float64) {
	if lockHysteresis < 1 || nisWindow < 1 || nisThreshold <= 0 {
		return
	}
	k.stateMachineEnabled = true
	k.mode = ModeCalibrating
	k.lockHysteresis = lockHysteresis
	k.consecutiveConverged = 0
	k.nisBuf = make([]float64, nisWindow)
	k.nisIdx = 0
	k.nisCount = 0
	k.nisSum = 0
	k.nisThreshold = nisThreshold
}

// DisableStateMachine turns off the state machine, putting the filter
// back into "always update" behavior. Idempotent. Keeps any current x
// and P unchanged; only the mode-machinery is disabled.
func (k *Filter) DisableStateMachine() {
	k.stateMachineEnabled = false
	k.mode = ModeCalibrating
	k.consecutiveConverged = 0
}

// Mode returns the current state-machine phase. Always returns
// ModeCalibrating if the state machine was not enabled.
func (k *Filter) Mode() Mode {
	return k.mode
}

// NIS returns the most recent rolling-window mean of the normalized
// innovation squared (y²/S). Updated on every Z while the state machine
// is enabled, regardless of mode — usable as a fit-quality indicator in
// CAL as well as in LCK. The buffer is reset on each CAL→LCK transition
// so the auto-unlock decision draws from post-lock samples only.
// Returns 0 if the state machine isn't enabled or no samples are in the
// buffer yet.
func (k *Filter) NIS() float64 {
	if k.nisCount == 0 {
		return 0
	}
	return k.nisSum / float64(k.nisCount)
}

// nisPush adds a new NIS sample to the circular buffer, maintaining the
// running sum for O(1) mean computation.
func (k *Filter) nisPush(nis float64) {
	if k.nisCount == len(k.nisBuf) {
		k.nisSum -= k.nisBuf[k.nisIdx]
	} else {
		k.nisCount++
	}
	k.nisBuf[k.nisIdx] = nis
	k.nisSum += nis
	k.nisIdx = (k.nisIdx + 1) % len(k.nisBuf)
}

// ForceUnlock transitions to ModeCalibrating and re-inflates P,
// equivalent to the NIS-triggered automatic unlock. State x is
// preserved. Useful for UI "force recalibration" and for tests.
// Works independent of whether the auto state machine is enabled —
// the manual override doesn't require a configured state machine.
// Synchronous: returns only after the goroutine has applied the change.
func (k *Filter) ForceUnlock() {
	select {
	case <-k.Done:
	default:
	}
	k.force <- forceCmdUnlock
	<-k.Done
}

// ForceLock transitions to ModeLocked immediately, bypassing the
// Converged()/lockHysteresis check. Use cautiously — locking before
// the calibration is actually good will freeze it at a bad value. P
// is left as-is (unlike unlock, which re-inflates). The NIS buffer is
// cleared so the LCK→CAL auto-unlock check needs fresh post-lock
// samples before it can fire (otherwise pre-lock CAL-era NIS residuals
// could trigger an immediate auto-unlock). Works independent of whether
// the auto state machine is enabled. Synchronous.
func (k *Filter) ForceLock() {
	select {
	case <-k.Done:
	default:
	}
	k.force <- forceCmdLock
	<-k.Done
}

// ResetStateMachine clears the CAL→LCK transition bookkeeping (mode set
// to CAL, consecutive-converged counter zeroed, NIS rolling buffer
// cleared) without touching state x or covariance P. Used by callers
// that bulk-feed the filter (e.g. cmd/websim's post-INIT replay) and
// want subsequent live measurements to drive a fresh CAL phase instead
// of inheriting the bulk feed's lock progress. Synchronous.
func (k *Filter) ResetStateMachine() {
	select {
	case <-k.Done:
	default:
	}
	k.force <- forceCmdResetSM
	<-k.Done
}

// Snapshot returns an independent deep copy of the filter's full
// internal state, suitable for stashing as a checkpoint and reapplying
// later with Restore. Safe to call between filter operations (the
// caller should ensure no concurrent send to U/Z/force/restore is in
// flight; the cmd/websim playback main loop reads Snapshot only after
// receiving Done from the previous Z, which is sufficient).
func (k *Filter) Snapshot() Snapshot {
	return Snapshot{
		X:                    copyMatrix(k.x),
		P:                    copyMatrix(k.p),
		Mode:                 k.mode,
		ConsecutiveConverged: k.consecutiveConverged,
		NISBuf:               append([]float64(nil), k.nisBuf...),
		NISIdx:               k.nisIdx,
		NISCount:             k.nisCount,
		NISSum:               k.nisSum,
	}
}

// Restore overwrites the filter's internal state with s. Synchronous:
// blocks until the goroutine has applied the change. The Snapshot's
// dimensions must match the filter (panic otherwise to surface bugs
// quickly rather than silently corrupting state).
func (k *Filter) Restore(s Snapshot) {
	if len(s.X) != 2*k.n {
		panic("kalman.Restore: snapshot dimension mismatch")
	}
	select {
	case <-k.Done:
	default:
	}
	k.restore <- s
	<-k.Done
}

func (k *Filter) applyRestoreLocked(s Snapshot) {
	k.x = copyMatrix(s.X)
	k.p = copyMatrix(s.P)
	k.mode = s.Mode
	k.consecutiveConverged = s.ConsecutiveConverged
	// Resize/refresh NIS buffer to match snapshot. The filter was
	// constructed with a particular buffer length via EnableStateMachine;
	// if the snapshot's length differs the caller is restoring across
	// configurations, which we accommodate.
	if len(k.nisBuf) != len(s.NISBuf) {
		k.nisBuf = make([]float64, len(s.NISBuf))
	}
	copy(k.nisBuf, s.NISBuf)
	k.nisIdx = s.NISIdx
	k.nisCount = s.NISCount
	k.nisSum = s.NISSum
}

// copyMatrix returns an independent deep copy.
func copyMatrix(m Matrix) Matrix {
	out := make(Matrix, len(m))
	for i, row := range m {
		out[i] = append([]float64(nil), row...)
	}
	return out
}

// SeedKL writes (kSeed, lSeed) into the filter's state x and resets P to
// its initial diagonal values, dropping any accumulated correlations.
// Used to bootstrap the filter from a non-EKF preprocessing step, e.g.
// the INIT-mode hand-rotation calibration in cmd/websim: when the user
// hand-rotates through enough orientations to bracket each axis, the
// midpoint of (min, max) is a sphere-center estimate for l, and the
// half-range divided by n0 inverts to a k estimate. Seeding these
// values before the EKF starts puts it in the right basin of attraction
// instead of letting it gradient-descend into a local minimum.
//
// Synchronous; blocks until the runFilter goroutine applies the change.
// Panics on length mismatch to surface caller bugs immediately.
func (k *Filter) SeedKL(kSeed, lSeed []float64) {
	pInit := make(Matrix, 2*k.n)
	for i := 0; i < k.n; i++ {
		pInit[2*i] = make([]float64, 2*k.n)
		pInit[2*i+1] = make([]float64, 2*k.n)
		pInit[2*i][2*i] = k.sigmaK0 * k.sigmaK0
		pInit[2*i+1][2*i+1] = (k.n0 * k.sigmaK0) * (k.n0 * k.sigmaK0)
	}
	k.SeedKLWithP(kSeed, lSeed, pInit)
}

// SeedKLWithP is SeedKL with a caller-supplied initial covariance matrix.
// Used by the cmd/websim INIT calibration path: after computing (k, l)
// from buffered samples it also derives a principled P via linear-
// regression analysis (EstimateCovariance) and passes it here so the
// filter starts with covariance grounded in the calibration data rather
// than the conservative default.
//
// pInit must be 2n × 2n. Panics on size mismatch.
func (k *Filter) SeedKLWithP(kSeed, lSeed []float64, pInit Matrix) {
	if len(kSeed) != k.n || len(lSeed) != k.n {
		panic("kalman.SeedKLWithP: length mismatch on kSeed/lSeed")
	}
	if len(pInit) != 2*k.n {
		panic("kalman.SeedKLWithP: pInit size mismatch")
	}
	for i := range pInit {
		if len(pInit[i]) != 2*k.n {
			panic("kalman.SeedKLWithP: pInit row size mismatch")
		}
	}
	snap := k.Snapshot()
	for i := 0; i < k.n; i++ {
		snap.X[2*i][0] = kSeed[i]
		snap.X[2*i+1][0] = lSeed[i]
	}
	for i := 0; i < 2*k.n; i++ {
		for j := 0; j < 2*k.n; j++ {
			snap.P[i][j] = pInit[i][j]
		}
	}
	// Reset state-machine bookkeeping so a freshly-seeded filter starts
	// in CAL with a clean consecutive-converged counter and NIS window.
	snap.Mode = ModeCalibrating
	snap.ConsecutiveConverged = 0
	for i := range snap.NISBuf {
		snap.NISBuf[i] = 0
	}
	snap.NISIdx = 0
	snap.NISCount = 0
	snap.NISSum = 0
	k.Restore(snap)
}

// EstimateCovariance fits a linear-regression covariance matrix for the
// state (k_1, l_1, k_2, l_2, …) using the buffered raw measurements
// `samples` and the current point estimate (kSeed, lSeed). The math:
//
//	For each sample m_j and the seed (k, l):
//	  n̂[i] = k_i * (m_j[i] - l_i)
//	  z_pred = Σ n̂[i]²
//	  r_j = z_pred - n0²                  (residual)
//	  H[j][2i]   = 2 * n̂[i]² / k_i         (∂z/∂k_i)
//	  H[j][2i+1] = -2 * n̂[i] * k_i         (∂z/∂l_i)
//
//	σ² = max(Σ r_j² / (N − p), floor)     (p = 2n parameters)
//	P  = σ² * (HᵀH)⁻¹
//
// floor is the EKF's expected per-sample measurement variance
// (2·n0²·sigmaM)², so the principled P never goes below what the chip's
// noise alone would justify.
//
// Returns (P, true) on success or (nil, false) if N ≤ p (under-
// determined) or HᵀH is singular (e.g. the user rotated about only one
// axis). Callers should fall back to a conservative default in the
// failure case.
func EstimateCovariance(samples [][]float64, kSeed, lSeed []float64, n0, sigmaM float64) (Matrix, bool) {
	n := len(kSeed)
	if n == 0 || len(lSeed) != n {
		return nil, false
	}
	p := 2 * n
	N := len(samples)
	if N <= p {
		return nil, false
	}

	HtH := make(Matrix, p)
	for i := range HtH {
		HtH[i] = make([]float64, p)
	}
	var sumR2 float64
	hRow := make([]float64, p)
	for _, m := range samples {
		if len(m) != n {
			continue
		}
		r := -n0 * n0
		for i := 0; i < n; i++ {
			ni := kSeed[i] * (m[i] - lSeed[i])
			r += ni * ni
			hRow[2*i] = 2 * ni * ni / kSeed[i]
			hRow[2*i+1] = -2 * ni * kSeed[i]
		}
		sumR2 += r * r
		for i := 0; i < p; i++ {
			for j := 0; j < p; j++ {
				HtH[i][j] += hRow[i] * hRow[j]
			}
		}
	}

	inv := matInverse(HtH)
	if inv == nil {
		return nil, false
	}

	sigma2 := sumR2 / float64(N-p)
	floor := (2 * n0 * n0 * sigmaM) * (2 * n0 * n0 * sigmaM)
	if sigma2 < floor {
		sigma2 = floor
	}
	out := make(Matrix, p)
	for i := 0; i < p; i++ {
		out[i] = make([]float64, p)
		for j := 0; j < p; j++ {
			out[i][j] = sigma2 * inv[i][j]
		}
	}
	return out, true
}

// unlockAndInflate transitions LCK→CAL: resets the NIS window, resets
// the consecutive-converged counter, and re-inflates P to its initial
// diagonal values. State x is preserved (the prior calibration remains
// our best guess; we just acknowledge we're now uncertain about it).
func (k *Filter) unlockAndInflate() {
	k.mode = ModeCalibrating
	k.consecutiveConverged = 0
	for i := range k.nisBuf {
		k.nisBuf[i] = 0
	}
	k.nisIdx = 0
	k.nisCount = 0
	k.nisSum = 0
	for i := 0; i < k.n; i++ {
		for j := 0; j < 2*k.n; j++ {
			k.p[2*i][j] = 0
			k.p[2*i+1][j] = 0
		}
		k.p[2*i][2*i] = k.sigmaK0 * k.sigmaK0
		k.p[2*i+1][2*i+1] = (k.n0 * k.sigmaK0) * (k.n0 * k.sigmaK0)
	}
}
