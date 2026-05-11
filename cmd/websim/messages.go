package main

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
	MaxSigmaK         float64 `json:"maxSigmaK"`
	MaxSigmaL         float64 `json:"maxSigmaL"`
	LockHysteresis    int     `json:"lockHysteresis"`
	NISWindow         int     `json:"nisWindow"`
	NISThreshold     float64 `json:"nisThreshold"`
	StateMachineOn   bool    `json:"stateMachineOn"`
}

// Some sensible default parameters to start the user off
var defaultParams = params{
	Source:  manual,
	N:       3,
	N0:      10000.0,
	KAct:    &[]float64{0.8, 0.7, 0.9},
	LAct:    &[]float64{1980, 1500, -1776},
	SigmaK0: 0.25,
	SigmaK:  0.00000001,
	SigmaM:  0.05,
}

type measureCmd struct {
	A direction `json:"a"` // Raw measurement (for manual), pre-noise
}

type estimateCmd struct {
	NN float64 `json:"nn"` // The actual measurement of N^2
}

// playbackCmd is the union of all scenario-playback control verbs.
type playbackCmd struct {
	Action string `json:"action"` // "play" | "pause" | "step" | "seek" | "reset" | "setRate"
	Step   int    `json:"step"`   // target step for seek (0-based)
	RateHz int    `json:"rateHz"` // ticks per second for play / setRate
}

type messageIn struct {
	Params       *params      `json:"params"`
	Measure      *measureCmd  `json:"measure"`
	Estimate     *estimateCmd `json:"estimate"`
	LoadScenario *string      `json:"loadScenario"` // basename in cmd/replay/scripts/
	PlaybackCmd  *playbackCmd `json:"playbackCmd"`
	SetMode      *string      `json:"setMode"` // "CAL" or "LCK"
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
	Mode        *string         `json:"mode"`      // "CAL" / "LCK"
	NIS         *float64        `json:"nis"`
	Converged   *bool           `json:"converged"`
	Playback    *playbackStatus `json:"playback"`
}
