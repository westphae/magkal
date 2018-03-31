package kalman

const (
	N0     = 1.0     // Strength of Earth's magnetic field at current location
	NSigma = 0.1     // Initial uncertainty scale
	Epsilon = 1e-6   // Some tiny noise scale
)

type KalmanFilter struct {
	x []float64       // Kalman Filter hidden state
	p [][]float64    // Kalman Filter hidden state covariance
	q [][]float64    // Kalman Filter state noise process
	r float64          // Measurement noise
	u []float64          // Control vector, measured mag vector in this case
	z float64          // Measurement vector, earth's mag field strength **2
	U chan []float64   // Channel for sending new control values to Kalman Filter
	Z chan float64   // Channel for sending new measurements to Kalman Filter
}

func NewKalmanFilter() (k *KalmanFilter) {
	k = new(KalmanFilter)

	k.x = []float64{1, 0}
	k.p = [][]float64{{NSigma * NSigma, 0}, {0, NSigma * NSigma /(N0*N0)}}
	k.q = [][]float64{{Epsilon * Epsilon, 0}, {0, Epsilon * Epsilon /(N0*N0)}}
	k.r = N0*N0*NSigma*NSigma

	k.U = make(chan []float64)
	k.Z = make(chan float64)

	go k.runFilter()

	return k
}

func (k *KalmanFilter) runFilter() {
	var (
		a, s, w1, w2, y float64
		nHat            []float64
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
			w1 = k.u[0]*k.p[0][0] + k.p[0][1]
			w2 = k.u[0]*k.p[0][1] + k.p[1][1]
			s = k.r + 4*nHat[0]*nHat[0]*(k.u[0]*w1 + w2)
			y = k.z - nHat[0]*nHat[0]
			a = 2*nHat[0]/s
			// State correction
			k.x[0] += a*y*w1
			k.x[1] += a*y*w2
			// State covariance correction
			a = 2*nHat[0]*a
			k.p[0][0] -= a*w1*w1
			k.p[0][1] -= a*w1*w2
			k.p[1][0] -= a*w2*w1
			k.p[1][1] -= a*w2*w2
		}
	}
}

func calcMagField(x, u []float64) (n []float64) {
	n = make([]float64, len(u))
	for i:=0; i<len(u); i++ {
		n[i] = x[2*i]*u[i] + x[2*i+1]
	}
	return n
}

func (k *KalmanFilter) State() (state []float64) {
	return k.x
}

func (k *KalmanFilter) StateCovariance() (cov [][]float64) {
	return k.p
}

func (k *KalmanFilter) ProcessNoise() (cov [][]float64) {
	return k.q
}

func (k *KalmanFilter) SetProcessNoise(q [][]float64) {
	k.q = q
}
