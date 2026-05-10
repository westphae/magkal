package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Truth struct {
	N     int       `yaml:"n"`
	N0    float64   `yaml:"n0"`
	K     []float64 `yaml:"k"`
	L     []float64 `yaml:"l"`
	Noise float64   `yaml:"noise"`
}

type FilterCfg struct {
	SigmaK0 float64 `yaml:"sigmaK0"`
	SigmaK  float64 `yaml:"sigmaK"`
	SigmaM  float64 `yaml:"sigmaM"`
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

type Step struct {
	Sweep  *SweepStep  `yaml:"sweep,omitempty"`
	Hold   *HoldStep   `yaml:"hold,omitempty"`
	Random *RandomStep `yaml:"random,omitempty"`
}

type Script struct {
	Truth  Truth     `yaml:"truth"`
	Filter FilterCfg `yaml:"filter"`
	Seed   int64     `yaml:"seed"`
	Steps  []Step    `yaml:"steps"`
}

func loadScript(path string) (*Script, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var s Script
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	return &s, nil
}

func (s *Script) validate() error {
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
		if n != 1 {
			return fmt.Errorf("steps[%d] must have exactly one of sweep/hold/random (got %d)", i, n)
		}
	}
	return nil
}
