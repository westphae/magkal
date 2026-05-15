package main

import (
	"math"
	"math/rand"
)

// initBufferSampleCap bounds the in-memory raw-sample buffer at INIT time.
// A typical INIT session at 50 Hz × 30 s collects ~1500 samples; the cap
// keeps very long INITs from growing the buffer without bound. Once full,
// further samples still update min/max and the count but are not stored
// for the principled-P calculation or the EKF replay.
const initBufferSampleCap = 5000

// initBuffer tracks per-axis min/max of raw measurements during the
// hand-rotation INIT phase of guided calibration, plus a capped log of
// the raw samples themselves. The midpoint of (min, max) is the sphere-
// center estimate seeded into the filter as l; n0 divided by the half-
// range is the seeded k. The stored samples drive two refinements at
// Finish time: a linear-regression covariance estimate fed to the filter
// via SeedKLWithP, and a randomised replay through the EKF that warms it
// up against the actual calibration data.
type initBuffer struct {
	n       int
	min     []float64
	max     []float64
	count   int
	samples [][]float64
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
	if len(b.samples) < initBufferSampleCap {
		row := make([]float64, b.n)
		copy(row, m)
		b.samples = append(b.samples, row)
	}
}

// shuffledCopy returns up to `cap` of the buffered samples in random
// order. Used by FinishInit to replay calibration measurements through
// the EKF after seeding, in a sequence the filter hasn't seen so it
// exercises every direction it converged from. If cap >= len(samples)
// the full buffer is returned (still shuffled).
func (b *initBuffer) shuffledCopy(rng *rand.Rand, cap int) [][]float64 {
	if len(b.samples) == 0 {
		return nil
	}
	out := make([][]float64, len(b.samples))
	copy(out, b.samples)
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	if cap > 0 && len(out) > cap {
		out = out[:cap]
	}
	return out
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
