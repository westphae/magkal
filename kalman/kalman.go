package kalman

type Filter struct {
	n int              // Number of dimensions
	x [][]float64      // Kalman Filter hidden state
	p [][]float64      // Kalman Filter hidden state covariance
	q [][]float64      // Kalman Filter state noise process
	r [][]float64      // Measurement noise
	u []float64      // Control vector, measured mag vector in this case
	z float64      // Measurement, earth's mag field strength **2
	U chan []float64 // Channel for sending new control values to Kalman Filter
	Z chan float64 // Channel for sending new measurements to Kalman Filter
}

/* NewKalmanFilter returns a Filter struct with Kalman Filter methods for calibrating a magnetometer.
   n is the number of dimensions (1, 2 for testing; 3 for reality)
   n0 is the strength of the Earth's magnetic field at the current location, 1.0 is fine for testing
   nSigma is the initial uncertainty scale for k (nSigma*n0 for l), 0.1 seems about right
   epsilon is a tiny noise scale, maybe 0.01
 */
func NewKalmanFilter(n int, n0 float64, nSigma float64, epsilon float64) (k *Filter) {
	k = new(Filter)
	k.n = n

	k.x = make([][]float64, 2*n)
	k.p = make([][]float64, 2*n)
	k.q = make([][]float64, 2*n)

	for i:=0; i<n; i++ {
		k.x[2*i] = []float64{1}
		k.x[2*i+1] = []float64{0}

		k.p[2*i] = make([]float64, 2*n)
		k.p[2*i+1] = make([]float64, 2*n)
		k.p[2*i][2*i] = nSigma * nSigma
		k.p[2*i+1][2*i+1] = nSigma * nSigma /(n0*n0)

		k.q[2*i] = make([]float64, 2*n)
		k.q[2*i+1] = make([]float64, 2*n)
		k.q[2*i][2*i] = epsilon * epsilon / (86400*0.01)
		k.q[2*i+1][2*i+1] = epsilon * epsilon /(n0*n0) / (86400*0.01)
	}

	k.r = [][]float64{{n0*n0*epsilon*epsilon}}

	k.U = make(chan []float64)
	k.Z = make(chan float64)

	go k.runFilter()

	return k
}

func (k *Filter) runFilter() {
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
			y = k.z
			for i:=0; i<k.n; i++ {
				y -= nHat[i][0]*nHat[i][0]
			}

			// Calculate Jacobian
			for i:=0; i<k.n; i++ {
				h[0][2*i] = 2*nHat[i][0]*k.u[i]
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

func calcMagField(x [][]float64, u []float64) (n [][]float64) {
	n = make([][]float64, len(u))
	for i:=0; i<len(u); i++ {
		n[i] = []float64{x[2*i][0]*u[i] + x[2*i+1][0]}
	}
	return n
}

func (k *Filter) State() (state [][]float64) {
	return k.x
}

func (k *Filter) StateCovariance() (cov [][]float64) {
	return k.p
}

func (k *Filter) ProcessNoise() (cov [][]float64) {
	return k.q
}

func (k *Filter) SetProcessNoise(q [][]float64) {
	k.q = q
}

func (k *Filter) K() (kk *[]float64) {
	v := make([]float64, k.n)
	for i:=0; i<k.n; i++ {
		v[i] = k.x[2*i][0]
	}
	return &v
}

func (k *Filter) L() (l *[]float64) {
	v := make([]float64, k.n)
	for i:=0; i<k.n; i++ {
		v[i] = k.x[2*i+1][0]
	}
	return &v
}

func (k *Filter) P() (p *[][]float64) {
	return &k.p
}
