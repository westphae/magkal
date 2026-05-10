package main

import (
	"math"
	"math/rand"
)

// Direction is a (theta, phi) pair in degrees. For n<3, phi is unused.
type Direction struct {
	Theta float64
	Phi   float64
}

// stepKind names which YAML step generated a Direction; flows into the record.
type stepKind string

const (
	kindSweep  stepKind = "sweep"
	kindHold   stepKind = "hold"
	kindRandom stepKind = "random"
)

// Generated is one (kind, label, direction) triple emitted by an iterator.
type Generated struct {
	Kind  stepKind
	Label string
	Dir   Direction
}

// expand turns one YAML Step into the sequence of Generated directions it
// produces, in order. Uses rng for any randomness so the script's seed
// fully determines output.
func expand(s Step, n int, rng *rand.Rand) []Generated {
	switch {
	case s.Sweep != nil:
		return expandSweep(s.Sweep, n)
	case s.Hold != nil:
		return expandHold(s.Hold, n, rng)
	case s.Random != nil:
		return expandRandom(s.Random, n, rng)
	}
	return nil
}

func expandSweep(s *SweepStep, n int) []Generated {
	out := []Generated{}
	// Inclusive walk on theta; if step[0] is 0 we just emit the start.
	thetaSteps := walk(s.From[0], s.To[0], s.Step[0])
	phiSteps := walk(s.From[1], s.To[1], s.Step[1])
	if n < 3 {
		phiSteps = []float64{0}
	}
	for _, theta := range thetaSteps {
		for _, phi := range phiSteps {
			out = append(out, Generated{
				Kind:  kindSweep,
				Label: s.Label,
				Dir:   Direction{Theta: theta, Phi: phi},
			})
		}
	}
	return out
}

// walk produces an inclusive arithmetic sequence from `from` to `to` with
// stride `step`. If step==0, returns just [from]. If step is the wrong sign
// for the direction of (to - from), step is flipped to match.
func walk(from, to, step float64) []float64 {
	if step == 0 {
		return []float64{from}
	}
	if to < from && step > 0 {
		step = -step
	}
	if to > from && step < 0 {
		step = -step
	}
	out := []float64{}
	if step > 0 {
		for v := from; v <= to+1e-9; v += step {
			out = append(out, v)
		}
	} else {
		for v := from; v >= to-1e-9; v += step {
			out = append(out, v)
		}
	}
	return out
}

func expandHold(s *HoldStep, n int, rng *rand.Rand) []Generated {
	out := make([]Generated, 0, s.Count)
	for i := 0; i < s.Count; i++ {
		theta := s.At[0] + s.JitterDeg*rng.NormFloat64()
		phi := 0.0
		if n == 3 {
			phi = s.At[1] + s.JitterDeg*rng.NormFloat64()
		}
		out = append(out, Generated{
			Kind:  kindHold,
			Label: s.Label,
			Dir:   Direction{Theta: theta, Phi: phi},
		})
	}
	return out
}

func expandRandom(s *RandomStep, n int, rng *rand.Rand) []Generated {
	out := make([]Generated, 0, s.Count)
	for i := 0; i < s.Count; i++ {
		var theta, phi float64
		switch n {
		case 1, 2:
			// Uniform on circle.
			theta = 360 * (rng.Float64() - 0.5)
		case 3:
			// Uniform on sphere (theta = azimuth in [-180,180), phi = inclination from cos sampling).
			theta = 360 * (rng.Float64() - 0.5)
			phi = math.Asin(2*rng.Float64()-1) * 180 / math.Pi
		}
		out = append(out, Generated{
			Kind:  kindRandom,
			Label: s.Label,
			Dir:   Direction{Theta: theta, Phi: phi},
		})
	}
	return out
}
