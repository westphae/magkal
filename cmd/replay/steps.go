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
	kindSweep     stepKind = "sweep"
	kindHold      stepKind = "hold"
	kindRandom    stepKind = "random"
	kindBodyFrame stepKind = "body_frame"
)

// Generated is one (kind, label, direction) triple emitted by an iterator.
type Generated struct {
	Kind  stepKind
	Label string
	Dir   Direction
}

// expand turns one YAML Step into the sequence of Generated directions it
// produces, in order. Uses rng for any randomness so the script's seed
// fully determines output. Takes Truth so body_frame can read inclination.
func expand(s Step, t Truth, rng *rand.Rand) []Generated {
	switch {
	case s.Sweep != nil:
		return expandSweep(s.Sweep, t.N)
	case s.Hold != nil:
		return expandHold(s.Hold, t.N, rng)
	case s.Random != nil:
		return expandRandom(s.Random, t.N, rng)
	case s.BodyFrame != nil:
		return expandBodyFrame(s.BodyFrame, t, rng)
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

// expandBodyFrame models an aircraft holding nominal (heading, pitch, roll)
// with small Gaussian jitter. The Earth field is taken to be a unit vector
// in NED of (cos I, 0, sin I) where I = truth.inclination_deg (positive
// down, mid-northern-latitude ≈ 65°). Per sample, jittered Euler angles
// rotate that vector into body frame, and the result is converted back to
// the (theta, phi) convention the rest of the harness uses.
func expandBodyFrame(s *BodyFrameStep, t Truth, rng *rand.Rand) []Generated {
	out := make([]Generated, 0, s.Count)
	I := t.InclinationDeg * deg
	for i := 0; i < s.Count; i++ {
		psi := (s.HeadingDeg + s.JitterHeadingDeg*rng.NormFloat64()) * deg
		pit := (s.PitchDeg + s.JitterPitchDeg*rng.NormFloat64()) * deg
		rol := (s.RollDeg + s.JitterRollDeg*rng.NormFloat64()) * deg
		x, y, z := nedFieldToBody(I, psi, pit, rol)
		// (x,y,z) is a unit vector. Convert to (theta, phi) such that
		// (cos θ cos φ, sin θ cos φ, sin φ) = (x, y, z), which is the
		// convention measure.go expects.
		phi := math.Asin(z)
		theta := math.Atan2(y, x)
		out = append(out, Generated{
			Kind: kindBodyFrame, Label: s.Label,
			Dir: Direction{Theta: theta * 180 / math.Pi, Phi: phi * 180 / math.Pi},
		})
	}
	return out
}

// nedFieldToBody returns the unit Earth-field vector in body frame, given
// inclination I (rad, positive down) and aviation Euler angles (heading
// psi, pitch, roll, all rad). Standard ZYX intrinsic: yaw about NED z,
// then pitch about new y, then roll about new x. The transformation NED→body
// is the transpose: R_x(-roll) R_y(-pitch) R_z(-psi).
func nedFieldToBody(I, psi, pitch, roll float64) (x, y, z float64) {
	cI, sI := math.Cos(I), math.Sin(I)
	// R_z(-psi) applied to (cI, 0, sI):
	cP, sP := math.Cos(psi), math.Sin(psi)
	x1 := cP * cI
	y1 := -sP * cI
	z1 := sI
	// R_y(-pitch):
	cT, sT := math.Cos(pitch), math.Sin(pitch)
	x2 := cT*x1 - sT*z1
	y2 := y1
	z2 := sT*x1 + cT*z1
	// R_x(-roll):
	cR, sR := math.Cos(roll), math.Sin(roll)
	x = x2
	y = cR*y2 + sR*z2
	z = -sR*y2 + cR*z2
	return
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
