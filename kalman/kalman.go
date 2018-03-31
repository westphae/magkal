package kalman

const (
	N0     = 1.0     // Strength of Earth's magnetic field at current location
	NSigma = 0.1     // Initial uncertainty scale
	Epsilon = 1e-6   // Some tiny noise scale
)

type KalmanFilter struct {
	x [2]float64       // Kalman Filter hidden state
	p [2][2]float64    // Kalman Filter hidden state covariance
	q [2][2]float64    // Kalman Filter state noise process
	r float64          // Measurement noise
	u float64          // Control vector, measured mag vector in this case
	z float64          // Measurement vector, earth's mag field strength **2
	U chan float64   // Channel for sending new control values to Kalman Filter
	Z chan float64   // Channel for sending new measurements to Kalman Filter
}

func NewKalmanFilter() (k *KalmanFilter) {
	k = new(KalmanFilter)

	k.x = [2]float64{1, 0}
	k.p = [2][2]float64{{NSigma * NSigma, 0}, {0, NSigma * NSigma /(N0*N0)}}
	k.q = [2][2]float64{{Epsilon * Epsilon, 0}, {0, Epsilon * Epsilon /(N0*N0)}}
	k.r = N0*N0*NSigma*NSigma

	k.U = make(chan float64)
	k.Z = make(chan float64)

	go k.runFilter()

	return k
}

func (k *KalmanFilter) runFilter() {
	var (
		a, nHat, s, w1, w2, y float64
	)

	for {
		select {
		case k.u = <-k.U:
			// Calculate estimated measurement
			nHat = calcMagField(k.x, k.u)
			// No evolution for x
			// Evolve p
			for i:=0; i<2; i++ {
				for j:=0; j<2; j++ {
					k.p[i][j] += k.q[i][j]
				}
			}
		case k.z = <-k.Z:
			// Calculate measurement residual
			w1 = k.u*k.p[0][0] + k.p[0][1]
			w2 = k.u*k.p[0][1] + k.p[1][1]
			s = k.r + 4*nHat*nHat*(k.u*w1 + w2)
			y = k.z - nHat*nHat
			a = 2*nHat/s
			// State correction
			k.x[0] += a*y*w1
			k.x[1] += a*y*w2
			// State covariance correction
			a = 2*nHat*a
			k.p[0][0] -= a*w1*w1
			k.p[0][1] -= a*w1*w2
			k.p[1][0] -= a*w2*w1
			k.p[1][1] -= a*w2*w2
		}
	}
}

func calcMagField(x [2]float64, u float64) (n float64) {
		return x[0]*u + x[1]
}

func (k *KalmanFilter) State() (state [2]float64) {
	return k.x
}

func (k *KalmanFilter) StateCovariance() (cov [2][2]float64) {
	return k.p
}

func (k *KalmanFilter) ProcessNoise() (cov [2][2]float64) {
	return k.q
}

func (k *KalmanFilter) SetProcessNoise(q [2][2]float64) {
	k.q = q
}
