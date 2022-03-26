package kalman

type Matrix [][]float64

func matAdd(a, b Matrix) (x Matrix) {
	x = make(Matrix, len(a))
	for i := 0; i < len(a); i++ {
		x[i] = make([]float64, len(a[0]))
		for j := 0; j < len(b[0]); j++ {
			x[i][j] = a[i][j] + b[i][j]
		}
	}
	return x
}

func matSMul(k float64, a Matrix) (x Matrix) {
	x = make(Matrix, len(a))
	for i := 0; i < len(a); i++ {
		x[i] = make([]float64, len(a[0]))
		for j := 0; j < len(a[0]); j++ {
			x[i][j] = k * a[i][j]
		}
	}
	return x
}

func matMul(a, b Matrix) (x Matrix) {
	x = make(Matrix, len(a))
	for i := 0; i < len(a); i++ {
		x[i] = make([]float64, len(b[0]))
		for j := 0; j < len(b[0]); j++ {
			for k := 0; k < len(b); k++ {
				x[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return x
}

func matTranspose(a Matrix) (x Matrix) {
	x = make(Matrix, len(a[0]))
	for i := 0; i < len(x); i++ {
		x[i] = make([]float64, len(a))
		for j := 0; j < len(x[0]); j++ {
			x[i][j] = a[j][i]
		}
	}
	return x
}
