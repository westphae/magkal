package main

import "math"

// initBuffer tracks per-axis min/max of raw measurements during the
// hand-rotation INIT phase of guided calibration. The midpoint of
// (min, max) is the sphere-center estimate seeded into the filter as l;
// n0 divided by the half-range is the seeded k. See SeedKL on the
// kalman.Filter for what the seed is used for downstream.
type initBuffer struct {
	n     int
	min   []float64
	max   []float64
	count int
}

func newInitBuffer(n int) *initBuffer {
	mn := make([]float64, n)
	mx := make([]float64, n)
	for i := 0; i < n; i++ {
		mn[i] = math.Inf(1)
		mx[i] = math.Inf(-1)
	}
	return &initBuffer{n: n, min: mn, max: mx}
}

func (b *initBuffer) add(m []float64) {
	if len(m) != b.n {
		return
	}
	for i := 0; i < b.n; i++ {
		if m[i] < b.min[i] {
			b.min[i] = m[i]
		}
		if m[i] > b.max[i] {
			b.max[i] = m[i]
		}
	}
	b.count++
}

// stats returns the current per-axis min/max/range and sample count in
// wire-friendly form. Infinite bounds (no samples yet) are reported as 0
// so the UI doesn't have to special-case JSON Infinity.
func (b *initBuffer) stats() initStats {
	mn := make([]float64, b.n)
	mx := make([]float64, b.n)
	rg := make([]float64, b.n)
	for i := 0; i < b.n; i++ {
		if b.count == 0 {
			mn[i], mx[i], rg[i] = 0, 0, 0
			continue
		}
		mn[i] = b.min[i]
		mx[i] = b.max[i]
		rg[i] = b.max[i] - b.min[i]
	}
	return initStats{Min: mn, Max: mx, Range: rg, Count: b.count}
}

// seed returns (k, l) computed from the buffered min/max:
//
//	l_i = (max_i + min_i) / 2          (sphere center)
//	k_i = n0 / ((max_i - min_i) / 2)   (inverse half-range)
//
// If an axis has zero range (degenerate — no rotation about it), k_i is
// left at 1 to avoid a divide-by-zero; the caller is responsible for
// detecting this and warning the user. Returns nil/nil if the buffer is
// empty.
func (b *initBuffer) seed(n0 float64) (k, l []float64) {
	if b.count == 0 {
		return nil, nil
	}
	k = make([]float64, b.n)
	l = make([]float64, b.n)
	for i := 0; i < b.n; i++ {
		half := (b.max[i] - b.min[i]) / 2
		l[i] = (b.max[i] + b.min[i]) / 2
		if half > 0 {
			k[i] = n0 / half
		} else {
			k[i] = 1
		}
	}
	return k, l
}
