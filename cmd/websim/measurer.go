package main

import (
	"math"
	"math/rand"
)

type measurement []float64
type measurer func(m0 measurement) (m measurement)

// makeRandomMeasurer creates a function that returns a new measurement of m, the magnetometer measurement.
// Inputs:
//   n: number of dimensions (1, 2, or 3)
//   n0: Earth's magnetic field (1.0 is fine for testing)
//   k: n-vector of the scaling factors
//   l: n-vector of the additive factors
//   r: noise level
//   equations is n = k*m + l
// The returned function takes a rough measurement just to satisfy the interface, but doesn't use it.
func makeRandomMeasurer(n int, n0 float64, k, l []float64, r float64) measurer {
	if n == 1 {
		return func(m0 measurement) (m measurement) {
			theta := 2 * math.Pi * (rand.Float64() - 0.5)
			if theta < 0 {
				return []float64{(-n0-l[0])/k[0] + r*rand.NormFloat64()}
			}
			return []float64{(n0-l[0])/k[0] + r*rand.NormFloat64()}
		}
	}
	if n == 2 {
		return func(m0 measurement) (m measurement) {
			theta := 2 * math.Pi * (rand.Float64() - 0.5)
			nx := n0 * math.Cos(theta)
			ny := n0 * math.Sin(theta)
			return []float64{
				(nx-l[0])/k[0] + r*rand.NormFloat64(),
				(ny-l[1])/k[1] + r*rand.NormFloat64(),
			}
		}
	}
	return func(m0 measurement) (m measurement) {
		theta := 2 * math.Pi * (rand.Float64() - 0.5)
		phi := math.Acos(2*rand.Float64() - 1)
		nx := n0 * math.Cos(theta)*math.Cos(phi)
		ny := n0 * math.Sin(theta)*math.Cos(phi)
		nz := n0 * math.Sin(phi)
		return []float64{
			(nx-l[0])/k[0] + r*rand.NormFloat64(),
			(ny-l[1])/k[1] + r*rand.NormFloat64(),
			(nz-l[2])/k[2] + r*rand.NormFloat64(),
		}
	}
}

// makeManualMeasurer creates a function that returns a new measurement of m, the magnetometer measurement.
// Inputs:
//   n: number of dimensions (1, 2, or 3)
//   n0: Earth's magnetic field (1.0 is fine for testing)
//   k: n-vector of the scaling factors
//   l: n-vector of the additive factors
//   r: noise level
//   equations is n = k*m + l
// The returned function takes a rough measurement and computes the corresponding angles, then computes
//   a corrected measurement including noise.
func makeManualMeasurer(n int, n0 float64, k, l []float64, r float64) measurer {
	if n == 1 {
		return func(m0 measurement) (m measurement) {
			if m0[0] < -l[0]/k[0] {
				return []float64{(-n0-l[0])/k[0] + r*rand.NormFloat64()}
			}
			return []float64{(n0-l[0])/k[0] + r*rand.NormFloat64()}
		}
	}
	if n == 2 {
		return func(m0 measurement) (m measurement) {
			theta := math.Atan2(k[1]*m0[1]+l[1], k[0]*m0[0]+l[0])
			nx := n0 * math.Cos(theta)
			ny := n0 * math.Sin(theta)
			return []float64{
				(nx-l[0])/k[0] + r*rand.NormFloat64(),
				(ny-l[1])/k[1] + r*rand.NormFloat64(),
			}
		}
	}
	return func(m0 measurement) (m measurement) {
		mx := k[0]*m0[0]+l[0]
		my := k[1]*m0[1]+l[1]
		mz := k[2]*m0[2]+l[2]
		theta := math.Atan2(mx, my)
		phi := math.Atan2(mz, math.Sqrt(mx*mx+my*my))
		nx := n0 * math.Cos(theta)*math.Cos(phi)
		ny := n0 * math.Sin(theta)*math.Cos(phi)
		nz := n0 * math.Sin(phi)
		return []float64{
			(nx-l[0])/k[0] + r*rand.NormFloat64(),
			(ny-l[1])/k[1] + r*rand.NormFloat64(),
			(nz-l[2])/k[2] + r*rand.NormFloat64(),
		}
	}
}