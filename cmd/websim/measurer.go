package main

import (
	"math"
	"math/rand"

	"github.com/westphae/goflying/mpu9250"
)

type measurement []float64 // A magnetometer measurement like [m1, m2, m3]
type direction []float64   // Angles pointing in a direction like [theta, phi], in degrees
type measurer func(a direction) (m measurement)

// makeRandomMeasurer creates a function that returns a new measurement of m, the magnetometer measurement.
// Inputs:
//   n: number of dimensions (1, 2, or 3)
//   n0: Earth's magnetic field (1.0 is fine for testing)
//   k: n-vector of the scaling factors
//   l: n-vector of the additive factors
//   r: noise level
//   equation is n = k*(m-l)
// The returned function takes a rough measurement just to satisfy the interface, but doesn't use it.
func makeRandomMeasurer(n int, n0 float64, k, l []float64, r float64) (m measurer, err error) {
	if n == 1 {
		return func(a direction) (m measurement) {
			theta := 2 * math.Pi * (rand.Float64() - 0.5)
			if theta < 0 {
				return []float64{-n0/k[0] + l[0] + r*rand.NormFloat64()}
			}
			return []float64{n0/k[0] + l[0] + r*rand.NormFloat64()}
		}, nil
	}
	if n == 2 {
		return func(a direction) (m measurement) {
			theta := 2 * math.Pi * (rand.Float64() - 0.5)
			nx := n0 * math.Cos(theta)
			ny := n0 * math.Sin(theta)
			return []float64{
				nx/k[0] + l[0] + r*rand.NormFloat64(),
				ny/k[1] + l[1] + r*rand.NormFloat64(),
			}
		}, nil
	}
	return func(a direction) (m measurement) {
		theta := 2 * math.Pi * (rand.Float64() - 0.5)
		phi := math.Acos(2*rand.Float64() - 1)
		nx := n0 * math.Cos(theta) * math.Cos(phi)
		ny := n0 * math.Sin(theta) * math.Cos(phi)
		nz := n0 * math.Sin(phi)
		return []float64{
			nx/k[0] + l[0] + r*rand.NormFloat64(),
			ny/k[1] + l[1] + r*rand.NormFloat64(),
			nz/k[2] + l[2] + r*rand.NormFloat64(),
		}
	}, nil
}

// makeManualMeasurer creates a function that returns a new measurement of m, the magnetometer measurement.
// Inputs:
//   n: number of dimensions (1, 2, or 3)
//   n0: Earth's magnetic field (1.0 is fine for testing)
//   k: n-vector of the scaling factors
//   l: n-vector of the additive factors
//   r: noise level
//   equation is n = k*(m-l)
// The returned function takes a rough measurement and computes the corresponding angles, then computes
//   a corrected measurement including noise.
func makeManualMeasurer(n int, n0 float64, k, l []float64, r float64) (m measurer, err error) {
	if n == 1 {
		return func(a direction) (m measurement) {
			var theta float64
			if a != nil && len(a) >= 1 {
				theta = a[0] * math.Pi / 180
			} else {
				theta = 2 * math.Pi * rand.Float64()
			}
			if theta > math.Pi/2 && theta < 3*math.Pi/2 {
				return []float64{-n0/k[0] + l[0] + r*rand.NormFloat64()}
			}
			return []float64{n0/k[0] + l[0] + r*rand.NormFloat64()}
		}, nil
	}
	if n == 2 {
		return func(a direction) (m measurement) {
			var theta float64
			if a != nil && len(a) >= 1 {
				theta = a[0] * math.Pi / 180
			} else {
				theta = 2 * math.Pi * rand.Float64()
			}
			nx := n0 * math.Cos(theta)
			ny := n0 * math.Sin(theta)
			return []float64{
				nx/k[0] + l[0] + r*rand.NormFloat64(),
				ny/k[1] + l[1] + r*rand.NormFloat64(),
			}
		}, nil
	}
	return func(a direction) (m measurement) {
		var theta, phi float64
		if a != nil && len(a) >= 2 {
			theta = a[0] * math.Pi / 180
			phi = a[1] * math.Pi / 180
		} else {
			theta = 2 * math.Pi * rand.Float64()
			phi = math.Acos(2*rand.Float64() - 1)
		}
		nx := n0 * math.Cos(theta) * math.Cos(phi)
		ny := n0 * math.Sin(theta) * math.Cos(phi)
		nz := n0 * math.Sin(phi)
		return []float64{
			nx/k[0] + l[0] + r*rand.NormFloat64(),
			ny/k[1] + l[1] + r*rand.NormFloat64(),
			nz/k[2] + l[2] + r*rand.NormFloat64(),
		}
	}, nil
}

// makeActualMeasurer creates a function that returns a new measurement of m, the magnetometer measurement.
// Inputs:
//   r: noise level
// The returned function takes a rough measurement just to satisfy the interface, but doesn't use it.
func makeActualMeasurer() (m measurer, err error) {
	mpu, err := mpu9250.NewMPU9250(250, 4, 50, true, false)
	if err != nil {
		return nil, err
	}
	// defer mpu.CloseMPU() // This really should be closed. Move into goroutine.

	var data *mpu9250.MPUData

	return func(a direction) (m measurement) {
		data = <-mpu.C
		return []float64{data.M1, data.M2, data.M3}
	}, nil
}
