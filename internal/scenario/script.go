// Package scenario defines the YAML scenario format consumed by the
// magkal test harnesses (cmd/replay and cmd/websim) and the pure
// functions that turn a scenario into a sequence of (theta, phi)
// directions and synthesized magnetometer measurements.
//
// The package is internal because it's dev-time tooling, not part of
// the reusable pkg/kalman API external consumers depend on.
package scenario

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Truth struct {
	N              int       `yaml:"n"`
	N0             float64   `yaml:"n0"`
	K              []float64 `yaml:"k"`
	L              []float64 `yaml:"l"`
	Noise          float64   `yaml:"noise"`
	InclinationDeg float64   `yaml:"inclination_deg,omitempty"` // magnetic inclination, positive down. Defaults to 0 (horizontal field).
}

type FilterCfg struct {
	SigmaK0 float64 `yaml:"sigmaK0"`
	SigmaK  float64 `yaml:"sigmaK"`
	SigmaM  float64 `yaml:"sigmaM"`
	// Convergence thresholds for Converged(). Both default to 0 (disabled).
	// MaxSigmaK is dimensionless; MaxSigmaL is in the same units as truth.n0.
	MaxSigmaK float64 `yaml:"maxSigmaK,omitempty"`
	MaxSigmaL float64 `yaml:"maxSigmaL,omitempty"`
	// Optional CAL↔LCK state machine. Absent → filter always updates (today's
	// behavior). Present → EnableStateMachine is called and the filter locks
	// when Converged() sustains, unlocks when windowed NIS exceeds threshold.
	StateMachine *StateMachineCfg `yaml:"stateMachine,omitempty"`
}

type StateMachineCfg struct {
	LockHysteresis int     `yaml:"lockHysteresis"`
	NISWindow      int     `yaml:"nisWindow"`
	NISThreshold   float64 `yaml:"nisThreshold"`
}

type SweepStep struct {
	From  [2]float64 `yaml:"from"`
	To    [2]float64 `yaml:"to"`
	Step  [2]float64 `yaml:"step"`
	Label string     `yaml:"label"`
}

type HoldStep struct {
	At        [2]float64 `yaml:"at"`
	JitterDeg float64    `yaml:"jitter_deg"`
	Count     int        `yaml:"count"`
	Label     string     `yaml:"label"`
}

type RandomStep struct {
	Count int    `yaml:"count"`
	Label string `yaml:"label"`
}

// BodyFrameStep models an aircraft holding a nominal attitude with small
// Gaussian jitter on heading/pitch/roll. The Earth field (using truth.n0
// and truth.inclination_deg) is rotated into body frame per sample.
// Only valid for n=3.
type BodyFrameStep struct {
	HeadingDeg       float64 `yaml:"heading_deg"`
	PitchDeg         float64 `yaml:"pitch_deg"`
	RollDeg          float64 `yaml:"roll_deg"`
	JitterHeadingDeg float64 `yaml:"jitter_heading_deg"`
	JitterPitchDeg   float64 `yaml:"jitter_pitch_deg"`
	JitterRollDeg    float64 `yaml:"jitter_roll_deg"`
	Count            int     `yaml:"count"`
	Label            string  `yaml:"label"`
}

// PerturbStep mutates truth.l in place by adding delta_l. Does not
// produce any filter measurements; used to simulate an external magnetic
// disturbance (iPad in cabin, etc.) so we can exercise the LCK→CAL
// transition of the state machine.
type PerturbStep struct {
	DeltaL []float64 `yaml:"delta_l"`
	Label  string    `yaml:"label"`
}

type Step struct {
	Sweep     *SweepStep     `yaml:"sweep,omitempty"`
	Hold      *HoldStep      `yaml:"hold,omitempty"`
	Random    *RandomStep    `yaml:"random,omitempty"`
	BodyFrame *BodyFrameStep `yaml:"body_frame,omitempty"`
	Perturb   *PerturbStep   `yaml:"perturb,omitempty"`
}

type Script struct {
	Truth  Truth     `yaml:"truth"`
	Filter FilterCfg `yaml:"filter"`
	Seed   int64     `yaml:"seed"`
	Steps  []Step    `yaml:"steps"`
}

// Load reads, parses, and validates a scenario YAML from disk.
func Load(path string) (*Script, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var s Script
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	return &s, nil
}

// Parse parses a scenario YAML from an in-memory byte slice. Useful for
// tests and for clients that already have the bytes.
func Parse(data []byte) (*Script, error) {
	var s Script
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid: %w", err)
	}
	return &s, nil
}

func (s *Script) Validate() error {
	if s.Truth.N < 1 || s.Truth.N > 3 {
		return fmt.Errorf("truth.n must be 1, 2, or 3 (got %d)", s.Truth.N)
	}
	if s.Truth.N0 <= 0 {
		return fmt.Errorf("truth.n0 must be positive (got %g)", s.Truth.N0)
	}
	if len(s.Truth.K) != s.Truth.N {
		return fmt.Errorf("truth.k must have length %d (got %d)", s.Truth.N, len(s.Truth.K))
	}
	if len(s.Truth.L) != s.Truth.N {
		return fmt.Errorf("truth.l must have length %d (got %d)", s.Truth.N, len(s.Truth.L))
	}
	for i, k := range s.Truth.K {
		if k == 0 {
			return fmt.Errorf("truth.k[%d] must be non-zero", i)
		}
	}
	if s.Filter.SigmaK0 <= 0 || s.Filter.SigmaM <= 0 {
		return fmt.Errorf("filter.sigmaK0 and filter.sigmaM must be positive")
	}
	if sm := s.Filter.StateMachine; sm != nil {
		if sm.LockHysteresis < 1 {
			return fmt.Errorf("filter.stateMachine.lockHysteresis must be >= 1 (got %d)", sm.LockHysteresis)
		}
		if sm.NISWindow < 1 {
			return fmt.Errorf("filter.stateMachine.nisWindow must be >= 1 (got %d)", sm.NISWindow)
		}
		if sm.NISThreshold <= 0 {
			return fmt.Errorf("filter.stateMachine.nisThreshold must be > 0 (got %g)", sm.NISThreshold)
		}
		if s.Filter.MaxSigmaK <= 0 || s.Filter.MaxSigmaL <= 0 {
			return fmt.Errorf("filter.stateMachine requires filter.maxSigmaK and filter.maxSigmaL to be set (otherwise the lock transition can never fire)")
		}
	}
	for i, st := range s.Steps {
		n := 0
		if st.Sweep != nil {
			n++
		}
		if st.Hold != nil {
			n++
		}
		if st.Random != nil {
			n++
		}
		if st.BodyFrame != nil {
			n++
			if s.Truth.N != 3 {
				return fmt.Errorf("steps[%d]: body_frame requires truth.n=3 (got %d)", i, s.Truth.N)
			}
		}
		if st.Perturb != nil {
			n++
			if len(st.Perturb.DeltaL) != s.Truth.N {
				return fmt.Errorf("steps[%d]: perturb.delta_l must have length %d (got %d)", i, s.Truth.N, len(st.Perturb.DeltaL))
			}
		}
		if n != 1 {
			return fmt.Errorf("steps[%d] must have exactly one of sweep/hold/random/body_frame/perturb (got %d)", i, n)
		}
	}
	return nil
}
