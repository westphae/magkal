// Package kalman defines a data structure that aggregates measurements.
// Averages/counts measurements that are "close."
// Can quickly apply the current K & L and return the associated N^2.
package kalman

import (
	"log"
	"math"
)

type Filter struct {
	n int               // Number of dimensions
	x Matrix            // Kalman Filter hidden state
	p Matrix            // Kalman Filter hidden state covariance
	q Matrix            // Kalman Filter state noise process
	r Matrix            // Measurement noise
	u Matrix            // Control vector, measured mag vector in this case
	z float64           // Measurement, earth's mag field strength **2
	U chan Matrix       // Channel for sending new control values to Kalman Filter
	Z chan float64      // Channel for sending new measurements to Kalman Filter
	Done chan struct{}  // Signalled (non-blocking) after each Z update completes; size-1 buffer

	// Convergence thresholds; zero means "not configured, Converged() returns
	// false". See SetConvergenceThresholds and Converged.
	maxSigmaK float64
	maxSigmaL float64
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
			// Calculate measurement residual
			y = k.z
			for i := 0; i < k.n; i++ {
				y -= nHat[i][0] * nHat[i][0]
			}
			log.Printf("Innovation y = %f\n", y)

			// Calculate Jacobian
			for i := 0; i < k.n; i++ {
				h[0][2*i] = 2 * nHat[i][0] * nHat[i][0] / k.x[2*i][0]
				h[0][2*i+1] = -2 * nHat[i][0] * k.x[2*i][0]
			}
			log.Printf("Jacobian H = %v\n", h)

			// Calculate S
			s = matAdd(k.r, matMul(h, matMul(k.p, matTranspose(h))))
			log.Printf("Inn Cov s = %v\n", s)

			// Kalman Gain
			kk = matSMul(1/s[0][0], matMul(k.p, matTranspose(h)))
			log.Printf("Gain kk = %v\n", kk)

			// State correction
			k.x = matAdd(k.x, matSMul(y, kk))
			log.Printf("State Update y*kk = %v\n", matSMul(y, kk))

			// State covariance correction
			k.p = matMul(matAdd(id, matSMul(-1, matMul(kk, h))), k.p)
			log.Printf("Cov Update kk*h = %v\n\n", matMul(matSMul(-1, matMul(kk, h)), k.p))

			// Non-blocking signal that the update is complete; observers
			// who care about post-update state read from Done after sending
			// to Z. Drain Done before sending Z to avoid stale signals.
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
