package main

import "time"

// Wire protocol for the websim websocket. Each messageIn/messageOut may
// contain any subset of the optional fields below; recipients ignore the
// fields they don't recognize.

type source int

const (
	manual   source = iota // User sends measurements through websocket
	random                 // Measurements are made randomly
	file                   // Measurements come from a file
	actual                 // Measurements come from an actual MPU sensor
	scenarioSrc            // Server-driven playback of a YAML scenario
)

type params struct {
	Source  source     `json:"source"`  // Source of the magnetometer data
	N       int        `json:"n"`       // Number of dimensions
	N0      float64    `json:"n0"`      // Value of Earth's magnetic field at location
	KAct    *[]float64 `json:"kAct"`    // Actual K for manual, random measurement sources
	LAct    *[]float64 `json:"lAct"`    // Actual L for manual, random, scenario sources
	SigmaK0 float64    `json:"sigmaK0"` // Initial noise scale for k
	SigmaK  float64    `json:"sigmaK"`  // Process noise scale for k
	SigmaM  float64    `json:"sigmaM"`  // Noise scale for measurement

	// Convergence + state-machine tuning. All optional (zeros disable).
	MaxSigmaK      float64 `json:"maxSigmaK"`
	MaxSigmaL      float64 `json:"maxSigmaL"`
	LockHysteresis int     `json:"lockHysteresis"`
	NISWindow      int     `json:"nisWindow"`
	NISThreshold   float64 `json:"nisThreshold"`
	StateMachineOn bool    `json:"stateMachineOn"`

	// Scenario-only descriptive info; ignored on inbound params.
	InclinationDeg float64 `json:"inclinationDeg,omitempty"`
	Noise          float64 `json:"noise,omitempty"`

	// When non-empty AND Source == actual, the server records each raw
	// measurement to ../replay/scripts/<RecordFile>.yaml as a `samples`
	// step. If the file already exists, a new step is appended (provided
	// truth.n / truth.n0 still match); otherwise it's created fresh.
	RecordFile string `json:"recordFile,omitempty"`
}

// Some sensible default parameters to start the user off.
// MaxSigmaK / MaxSigmaL / state-machine triple are pre-set so a fresh
// connection auto-locks once the filter converges, instead of leaving
// Converged() at false forever (zero thresholds disable the check).
// Values match cmd/replay/scripts/cruise_realistic.yaml.
var defaultParams = params{
	Source:         manual,
	N:              3,
	N0:             50.0,
	KAct:           &[]float64{0.8, 0.7, 0.9},
	LAct:           &[]float64{9.9, 7.5, -8.88},
	SigmaK0:        0.25,
	SigmaK:         0.00000001,
	SigmaM:         0.05,
	MaxSigmaK:      1e-3,
	MaxSigmaL:      5.0,
	StateMachineOn: true,
	LockHysteresis: 10,
	NISWindow:      100,
	NISThreshold:   4.0,
}

type measureCmd struct {
	A direction `json:"a"` // Raw measurement (for manual), pre-noise
}

type estimateCmd struct {
	NN float64 `json:"nn"` // The actual measurement of N^2
}

// playbackCmd is the union of all scenario-playback control verbs.
type playbackCmd struct {
	Action string `json:"action"` // "play" | "pause" | "step" | "seek" | "reset" | "setRate" | "applyFilter"
	Step   int    `json:"step"`   // target step for seek (0-based)
	RateHz int    `json:"rateHz"` // ticks per second for play / setRate
	// Fields used by "applyFilter": reconfigures the loaded scenario's
	// filter mid-run. Convergence thresholds and state-machine settings.
	MaxSigmaK      float64 `json:"maxSigmaK"`
	MaxSigmaL      float64 `json:"maxSigmaL"`
	StateMachineOn bool    `json:"stateMachineOn"`
	LockHysteresis int     `json:"lockHysteresis"`
	NISWindow      int     `json:"nisWindow"`
	NISThreshold   float64 `json:"nisThreshold"`
}

type messageIn struct {
	Params       *params      `json:"params"`
	Measure      *measureCmd  `json:"measure"`
	Estimate     *estimateCmd `json:"estimate"`
	LoadScenario *string      `json:"loadScenario"` // basename in cmd/replay/scripts/
	PlaybackCmd  *playbackCmd `json:"playbackCmd"`
	SetMode      *string      `json:"setMode"` // "CAL" or "LCK"
	// Guided manual-calibration controls. StartInit enters the buffering
	// phase (per-axis min/max accumulation); FinishInit computes a (k,l)
	// seed and applies it to the filter via SeedKL before resuming CAL.
	StartInit  *bool `json:"startInit"`
	FinishInit *bool `json:"finishInit"`
	// SaveBest snapshots the current filter state (k, l, P) and persists
	// it as the server-side "known best" model. ResetBest deletes that
	// file. Both are explicit user actions — the auto-save on Finish/
	// Force Lock was removed in favour of these.
	SaveBest  *bool `json:"saveBest"`
	ResetBest *bool `json:"resetBest"`
	// SaveRecording forces the active recorder to flush its buffered
	// samples to disk as a new labelled segment in the target YAML.
	// No-op if no recording is active or the buffer is empty.
	SaveRecording *bool `json:"saveRecording"`
}

// initStats reports the per-axis min/max/range and sample count gathered
// during INIT mode. Pushed to the UI on every measurement received while
// the calibration buffer is active.
type initStats struct {
	Min   []float64 `json:"min"`
	Max   []float64 `json:"max"`
	Range []float64 `json:"range"`
	Count int       `json:"count"`
}

type state struct {
	K []float64   `json:"k"`
	L []float64   `json:"l"`
	P [][]float64 `json:"p"`
}

// playbackStatus mirrors the server-side playback state to the UI so it can
// render the scrubber, play/pause button, and segment label correctly.
type playbackStatus struct {
	Step     int    `json:"step"`
	Total    int    `json:"total"`
	Segment  string `json:"segment"`
	Playing  bool   `json:"playing"`
	Seeking  bool   `json:"seeking"`
	RateHz   int    `json:"rateHz"`
	Loaded   string `json:"loaded"` // currently loaded scenario filename (or "")
}

type messageOut struct {
	Params      *params         `json:"params"`
	Measurement *measurement    `json:"measurement"`
	State       *state          `json:"state"`
	Scenarios   *[]string       `json:"scenarios"` // sent on initial connect
	Mode        *string         `json:"mode"`      // "INIT" / "CAL" / "LCK"
	NIS         *float64        `json:"nis"`
	Converged   *bool           `json:"converged"`
	Playback    *playbackStatus `json:"playback"`
	InitStats   *initStats      `json:"initStats,omitempty"`
	// Rejected is the running count of measurements the outlier filter
	// dropped before they reached the EKF (NaN/Inf, n_est > 10·n0, or
	// >2× step change vs. last accepted). Surfaces in the UI so a glitch
	// burst doesn't silently disappear.
	Rejected *int `json:"rejected,omitempty"`
	// Best is the server-side persisted "known best" snapshot, shipped
	// on connect and after Save/Reset Best. Empty K/L slices (or N=0)
	// signal "no saved best" — the client should fall back to (1, 0)
	// for the dark-ellipse reference. P stays server-side; the client
	// only needs (k, l) for display.
	Best *bestSnapshot `json:"best,omitempty"`
	// Recording mirrors the live recorder's state (filename, buffered
	// sample count, last flush timestamp) so the UI can show progress
	// and confirm saves. Nil when no recording is active.
	Recording *recordingStatusSnapshot `json:"recording,omitempty"`
}

// bestSnapshot is the client-visible view of the saved best estimate.
// P is omitted from the wire since the client never displays it; the
// server uses the on-disk P when seeding the filter on Restart.
type bestSnapshot struct {
	N       int       `json:"n"`
	K       []float64 `json:"k"`
	L       []float64 `json:"l"`
	SavedAt time.Time `json:"savedAt"`
}
