package kalman

const (
	N0     = 1.0     // Strength of Earth's magnetic field at current location
	NSigma = 0.1     // Initial uncertainty scale
	Epsilon = 1e-6   // Some tiny noise scale
)

type KalmanFilter struct {
	n int              // Number of dimensions
	x [][]float64      // Kalman Filter hidden state
	p [][]float64      // Kalman Filter hidden state covariance
	q [][]float64      // Kalman Filter state noise process
	r [][]float64      // Measurement noise
	u [][]float64      // Control vector, measured mag vector in this case
	z [][]float64      // Measurement, earth's mag field strength **2
	U chan [][]float64 // Channel for sending new control values to Kalman Filter
	Z chan [][]float64 // Channel for sending new measurements to Kalman Filter
}

func NewKalmanFilter(n int) (k *KalmanFilter) {
	k = new(KalmanFilter)
	k.n = n

	k.x = make([][]float64, 2*n)
	k.p = make([][]float64, 2*n)
	k.q = make([][]float64, 2*n)

	for i:=0; i<n; i++ {
		k.x[2*i] = []float64{1}
		k.x[2*i+1] = []float64{0}

		k.p[2*i] = make([]float64, 2*n)
		k.p[2*i+1] = make([]float64, 2*n)
		k.p[2*i][2*i] = NSigma * NSigma
		k.p[2*i+1][2*i+1] = NSigma * NSigma /(N0*N0)

		k.q[2*i] = make([]float64, 2*n)
		k.q[2*i+1] = make([]float64, 2*n)
		k.q[2*i][2*i] = Epsilon * Epsilon
		k.q[2*i+1][2*i+1] = Epsilon * Epsilon /(N0*N0)
	}

	k.r = [][]float64{{N0*N0*NSigma*NSigma}}

	k.U = make(chan [][]float64)
	k.Z = make(chan [][]float64)

	go k.runFilter()

	return k
}

func (k *KalmanFilter) runFilter() {
	var (
		y              float64
		h, s, kk, nHat [][]float64
	)

	h = make([][]float64, 1)
	h[0] = make([]float64, 2*k.n)
	id := make([][]float64, 2*k.n)
	for i:=0; i<2*k.n; i++ {
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
			for i:=0; i<2*k.n; i++ {
				for j:=0; j<2*k.n; j++ {
					k.p[i][j] += k.q[i][j]
				}
			}
		case k.z = <-k.Z:
			// Calculate measurement residual
			y = k.z[0][0]
			for i:=0; i<k.n; i++ {
				y -= nHat[i][0]*nHat[i][0]
			}

			// Calculate Jacobian
			for i:=0; i<k.n; i++ {
				h[0][2*i] = 2*nHat[i][0]*k.u[i][0]
				h[0][2*i+1] = 2 * nHat[i][0]
			}

			// Calculate S
			s = matAdd(k.r, matMul(h, matMul(k.p, matTranspose(h))))

			// Kalman Gain
			kk = matSMul(1/s[0][0], matMul(k.p, matTranspose(h)))

			// State correction
			k.x = matAdd(k.x, matSMul(y, kk))

			// State covariance correction
			k.p = matMul(matAdd(id, matSMul(-1, matMul(kk, h))), k.p)
		}
	}
}

func calcMagField(x, u [][]float64) (n [][]float64) {
	n = make([][]float64, len(u))
	for i:=0; i<len(u); i++ {
		n[i] = []float64{x[2*i][0]*u[i][0] + x[2*i+1][0]}
	}
	return n
}

func (k *KalmanFilter) State() (state [][]float64) {
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

func matAdd(a, b [][]float64) (x[][]float64) {
	x = make([][]float64, len(a))
	for i:=0; i<len(a); i++ {
		x[i] = make([]float64, len(a[0]))
		for j := 0; j < len(b[0]); j++ {
			x[i][j] = a[i][j] + b[i][j]
		}
	}
	return x
}

func matSMul(k float64, a [][]float64) (x [][]float64) {
	x = make([][]float64, len(a))
	for i:=0; i<len(a); i++ {
		x[i] = make([]float64, len(a[0]))
		for j:=0; j<len(a[0]); j++ {
			x[i][j] = k*a[i][j]
		}
	}
	return x
}

func matMul(a, b [][]float64) (x [][]float64) {
	x = make([][]float64, len(a))
	for i:=0; i<len(a); i++ {
		x[i] = make([]float64, len(b[0]))
		for j:=0; j<len(b[0]); j++ {
			for k:=0; k<len(b); k++ {
				x[i][j] += a[i][k]*b[k][j]
			}
		}
	}
	return x
}

func matTranspose(a [][]float64) (x [][]float64) {
	x = make([][]float64, len(a[0]))
	for i:=0; i<len(x); i++ {
		x[i] = make([]float64, len(a))
		for j:=0; j<len(x[0]); j++ {
			x[i][j] = a[j][i]
		}
	}
	return x
}
