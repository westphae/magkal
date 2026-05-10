package main

import (
	"math"
	"math/rand"
)

const deg = math.Pi / 180

// synthMeasurement turns a true-direction (theta, phi in degrees) plus the
// truth calibration (k, l, n0) and noise std-dev into a noisy raw
// magnetometer reading m of length n. The model is the inverse of the
// filter's: n = k*(m - l), so m = n/k + l + noise.
//
// Adapted from cmd/websim/measurer.go's makeManualMeasurer; kept here so
// cmd/replay can evolve independently. We'll factor a shared package once
// websim wants to play the same scripts.
func synthMeasurement(n int, theta, phi float64, k, l []float64, n0, noise float64, rng *rand.Rand) []float64 {
	t := theta * deg
	p := phi * deg
	m := make([]float64, n)
	switch n {
	case 1:
		// 1-D: just use theta to choose +n0 or -n0 along the single axis.
		nx := n0 * math.Cos(t)
		m[0] = nx/k[0] + l[0] + noise*rng.NormFloat64()
	case 2:
		nx := n0 * math.Cos(t)
		ny := n0 * math.Sin(t)
		m[0] = nx/k[0] + l[0] + noise*rng.NormFloat64()
		m[1] = ny/k[1] + l[1] + noise*rng.NormFloat64()
	case 3:
		// theta = azimuth, phi = inclination from horizontal plane.
		// (matches cmd/websim/measurer.go:114-130 convention.)
		nx := n0 * math.Cos(t) * math.Cos(p)
		ny := n0 * math.Sin(t) * math.Cos(p)
		nz := n0 * math.Sin(p)
		m[0] = nx/k[0] + l[0] + noise*rng.NormFloat64()
		m[1] = ny/k[1] + l[1] + noise*rng.NormFloat64()
		m[2] = nz/k[2] + l[2] + noise*rng.NormFloat64()
	}
	return m
}
