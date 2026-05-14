package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/westphae/magkal/internal/scenario"
	"gopkg.in/yaml.v3"
)

// recorder collects raw magnetometer samples taken from the Actual data
// source and flushes them to a scenario YAML on demand. One recorder is
// tied to one filename + (n, n0) tuple; if the user changes any of these,
// the old recorder is flushed and a new one started.
type recorder struct {
	filename string  // basename, with .yaml extension applied
	n        int
	n0       float64
	sigmaK0  float64
	sigmaK   float64
	sigmaM   float64
	label    string  // assigned at construction time; one label per session
	data     [][]float64
}

// newRecorder builds a recorder. filename is sanitized to a basename and
// forced to end in .yaml. label is auto-generated from the current time so
// stop/restart appends produce distinguishable segments in the loaded
// scenario.
func newRecorder(filename string, n int, n0, sigmaK0, sigmaK, sigmaM float64) *recorder {
	base := filepath.Base(filename)
	if ext := strings.ToLower(filepath.Ext(base)); ext != ".yaml" && ext != ".yml" {
		base += ".yaml"
	}
	return &recorder{
		filename: base,
		n:        n,
		n0:       n0,
		sigmaK0:  sigmaK0,
		sigmaK:   sigmaK,
		sigmaM:   sigmaM,
		label:    "actual_" + time.Now().Format("2006-01-02T15-04-05"),
	}
}

// append stores one raw measurement. Silently ignores rows whose length
// doesn't match n — that would only happen on a programming error.
func (r *recorder) append(m []float64) {
	if len(m) != r.n {
		return
	}
	row := make([]float64, r.n)
	copy(row, m)
	r.data = append(r.data, row)
}

// flush writes the buffered samples to disk. If the target file already
// exists, the recorder's session is appended to the existing scenario as
// a new `samples` step (provided truth.n / truth.n0 match); otherwise a
// fresh scenario is written. No-op if no samples have been collected.
func (r *recorder) flush() {
	if len(r.data) == 0 {
		return
	}
	path := filepath.Join(scriptsDir, r.filename)
	script, err := r.loadOrInit(path)
	if err != nil {
		ui.Logf("recorder: %v (discarded %d samples)", err, len(r.data))
		return
	}
	script.Steps = append(script.Steps, scenario.Step{
		Samples: &scenario.SamplesStep{
			Data:  r.data,
			Label: r.label,
		},
	})
	if err := writeScript(path, script); err != nil {
		ui.Logf("recorder: write %s: %v", path, err)
		return
	}
	ui.Logf("recorder: wrote %d samples to %s (label=%q)", len(r.data), path, r.label)
}

// loadOrInit returns the existing scenario at path with the recorder's
// (n, n0) verified to match, or a fresh scenario seeded from the recorder
// if the file doesn't exist. Returns an error if the file exists but is
// incompatible.
func (r *recorder) loadOrInit(path string) (*scenario.Script, error) {
	if _, err := os.Stat(path); err == nil {
		s, err := scenario.Load(path)
		if err != nil {
			return nil, fmt.Errorf("load %s for append: %w", path, err)
		}
		if s.Truth.N != r.n {
			return nil, fmt.Errorf("%s has truth.n=%d but recorder is n=%d; refusing to append", path, s.Truth.N, r.n)
		}
		if s.Truth.N0 != r.n0 {
			return nil, fmt.Errorf("%s has truth.n0=%g but recorder is n0=%g; refusing to append", path, s.Truth.N0, r.n0)
		}
		return s, nil
	}
	// Build a fresh scenario. k=[1,1,1], l=[0,0,0] are placeholders —
	// recorded samples don't go through SynthMeasurement so these aren't
	// used during replay, but the YAML schema requires them.
	k := make([]float64, r.n)
	l := make([]float64, r.n)
	for i := range k {
		k[i] = 1
	}
	return &scenario.Script{
		Truth: scenario.Truth{
			N:  r.n,
			N0: r.n0,
			K:  k,
			L:  l,
		},
		Filter: scenario.FilterCfg{
			SigmaK0: r.sigmaK0,
			SigmaK:  r.sigmaK,
			SigmaM:  r.sigmaM,
		},
		Seed: 0,
	}, nil
}

func writeScript(path string, s *scenario.Script) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
