package kalman

import "math"

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

// matInverse returns the inverse of an n×n matrix via Gauss-Jordan
// elimination with partial pivoting. Returns nil if the matrix is
// singular (no non-zero pivot in some column). Intended for the small
// matrices that show up in covariance bootstrapping (≤ 6×6); not
// optimised for large inputs.
func matInverse(a Matrix) Matrix {
	n := len(a)
	aug := make([][]float64, n)
	for i := 0; i < n; i++ {
		aug[i] = make([]float64, 2*n)
		copy(aug[i], a[i])
		aug[i][n+i] = 1
	}
	for i := 0; i < n; i++ {
		piv := math.Abs(aug[i][i])
		bestRow := i
		for k := i + 1; k < n; k++ {
			if math.Abs(aug[k][i]) > piv {
				piv = math.Abs(aug[k][i])
				bestRow = k
			}
		}
		if piv < 1e-12 {
			return nil
		}
		if bestRow != i {
			aug[i], aug[bestRow] = aug[bestRow], aug[i]
		}
		div := aug[i][i]
		for k := 0; k < 2*n; k++ {
			aug[i][k] /= div
		}
		for j := 0; j < n; j++ {
			if j == i {
				continue
			}
			factor := aug[j][i]
			if factor == 0 {
				continue
			}
			for k := 0; k < 2*n; k++ {
				aug[j][k] -= factor * aug[i][k]
			}
		}
	}
	inv := make(Matrix, n)
	for i := 0; i < n; i++ {
		inv[i] = make([]float64, n)
		copy(inv[i], aug[i][n:])
	}
	return inv
}
