package main

import (
	"context"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/westphae/magkal/internal/scenario"
	"github.com/westphae/magkal/pkg/kalman"
)

// scriptsDir is where the server looks for YAML scenarios. Relative to the
// CWD which (per CLAUDE.md) is `cmd/websim/` when running the server.
const scriptsDir = "../replay/scripts"

// scenarioPicks returns the .yaml filenames available in scriptsDir,
// sorted alphabetically. Errors are logged and an empty list returned —
// the dropdown will just be empty in the UI.
func scenarioPicks() []string {
	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		log.Printf("scenarioPicks: %v", err)
		return nil
	}
	out := []string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".yaml" && filepath.Ext(name) != ".yml" {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// displayCapHz caps how often state pushes go to the client. The filter
// ticks at the user-selected rateHz server-side; sends are downsampled
// so the websocket and browser-side render loop can keep up. The user
// reported that 100 Hz play ended up rendering at ~4 Hz because the
// queue clogged with state messages; this divisor keeps the queue
// drained at a rate the client can actually consume.
const displayCapHz = 10

// checkpointEvery is the stride between cached filter snapshots saved
// during normal play. On seek, we restore from the nearest cached
// step ≤ target rather than rewinding to step 0. Cost: one Snapshot()
// per checkpoint plus ~(matrix + nisBuf) bytes of memory each.
const checkpointEvery = 1000

// checkpoint captures everything needed to resume playback at a given
// step: the filter's internal state, the truth.l value at that point
// (perturbs mutate it), and enough rng information to reconstruct the
// playbackRng. Since the playbackRng is fully determined by the seed
// and the number of measurement steps consumed, the latter is enough.
type checkpoint struct {
	snap     kalman.Snapshot
	truthL   []float64
	measured int // number of measurement steps consumed in [0, step)
}

// playback owns a loaded scenario and its expanded direction stream, plus
// the per-step iteration state. Methods are intended to run on the main
// connection goroutine; an internal seekCtx cancels in-flight seeks when
// a new playback command arrives.
type playback struct {
	script *scenario.Script
	gens   []scenario.Generated // pre-expanded; len(gens) is the total step count
	truth  scenario.Truth       // working copy; mutated by perturb steps
	loaded string               // basename of the loaded file

	step    int
	playing bool
	rateHz  int

	// Display-rate downsampling. sendEvery is recomputed when rateHz
	// changes; sinceLastSend counts ticks since the last state push.
	// lastSentLabel forces a send when the segment label changes so the
	// user always sees transitions even at high rates.
	sendEvery     int
	sinceLastSend int
	lastSentLabel string

	// Seek concurrency. seekCancel is non-nil while a seek is running;
	// invoking it preempts that seek so a new one can start.
	mu         sync.Mutex
	seeking    bool
	seekCancel context.CancelFunc

	kf *kalman.Filter

	// Checkpoints saved during normal play, keyed by step. checkpoint[0]
	// is the initial state (saved at loadScenario). Subsequent entries
	// at steps that are multiples of checkpointEvery.
	checkpoints map[int]*checkpoint
	// Running count of measurement steps consumed; needed for rng
	// reconstruction in checkpoints.
	measuredSoFar int
}

// recomputeSendEvery updates the send-every-Nth gate based on the
// current rateHz. Called by loadScenario, setRate, and play.
func (p *playback) recomputeSendEvery() {
	if p.rateHz <= displayCapHz {
		p.sendEvery = 1
	} else {
		p.sendEvery = p.rateHz / displayCapHz
		if p.sendEvery < 1 {
			p.sendEvery = 1
		}
	}
	p.sinceLastSend = 0
}

// loadScenario reads & parses the YAML file, pre-expands the direction
// stream, and constructs a fresh kalman.Filter wired with the script's
// thresholds and state machine. Replaces any prior playback state.
func loadScenario(name string) (*playback, error) {
	path := filepath.Join(scriptsDir, filepath.Base(name))
	s, err := scenario.Load(path)
	if err != nil {
		return nil, err
	}
	pb := &playback{
		script:      s,
		gens:        scenario.ExpandAll(s),
		truth:       cloneTruth(s.Truth),
		loaded:      name,
		rateHz:      10,
		checkpoints: map[int]*checkpoint{},
	}
	pb.recomputeSendEvery()
	pb.rebuildFilter()
	// Step-0 checkpoint: the fresh filter state plus initial truth.
	pb.checkpoints[0] = &checkpoint{
		snap:     pb.kf.Snapshot(),
		truthL:   append([]float64(nil), pb.truth.L...),
		measured: 0,
	}
	return pb, nil
}

func cloneTruth(t scenario.Truth) scenario.Truth {
	out := t
	out.K = append([]float64(nil), t.K...)
	out.L = append([]float64(nil), t.L...)
	return out
}

func (p *playback) rebuildFilter() {
	fc := p.script.Filter
	kf := kalman.NewKalmanFilter(p.script.Truth.N, p.script.Truth.N0, fc.SigmaK0, fc.SigmaK, fc.SigmaM)
	if fc.MaxSigmaK > 0 || fc.MaxSigmaL > 0 {
		kf.SetConvergenceThresholds(fc.MaxSigmaK, fc.MaxSigmaL)
	}
	if sm := fc.StateMachine; sm != nil {
		kf.EnableStateMachine(sm.LockHysteresis, sm.NISWindow, sm.NISThreshold)
	}
	p.kf = kf
}

// reset returns to step 0 with a fresh filter and truth from the script.
// Cancels any running seek. Does not change playing state. Resets the
// checkpoint cache to just the step-0 snapshot since subsequent seeks
// will re-fill it.
func (p *playback) reset() {
	p.cancelSeek()
	p.rebuildFilter()
	p.truth = cloneTruth(p.script.Truth)
	p.step = 0
	p.measuredSoFar = 0
	p.checkpoints = map[int]*checkpoint{
		0: {
			snap:     p.kf.Snapshot(),
			truthL:   append([]float64(nil), p.truth.L...),
			measured: 0,
		},
	}
}

// rngAt returns a freshly-seeded rng advanced to a state matching the
// given measurement-step count. Equivalent to running playback from
// step 0 to that point and consuming the noise rng exactly as
// SynthMeasurement would, just without computing or applying the
// measurements. Used during seek to reconstruct rng state.
func (p *playback) rngAt(measured int) *rand.Rand {
	r := rand.New(rand.NewSource(p.script.Seed))
	// Each measurement step consumes truth.N NormFloat64 calls.
	skip := measured * p.truth.N
	for i := 0; i < skip; i++ {
		r.NormFloat64()
	}
	return r
}

// CurrentRng returns a freshly-built rng matching playback's current
// position. Called by main.go to resync c.playbackRng after a seek.
func (p *playback) CurrentRng() *rand.Rand {
	return p.rngAt(p.measuredSoFar)
}

// applyGen consumes one Generated entry: for perturb, mutates truth in
// place; for measurement kinds, synthesizes a measurement using the given
// rng and drives the filter. step is advanced. Returns the synthesized
// measurement (nil for perturb steps) for downstream rendering.
//
// Also saves a checkpoint when the post-update step is a multiple of
// checkpointEvery, so subsequent seeks can restore from the nearest
// checkpoint ≤ target instead of replaying from step 0.
func (p *playback) applyGen(g scenario.Generated, rng *rand.Rand) []float64 {
	if g.Kind == scenario.KindPerturb {
		for i := range p.truth.L {
			p.truth.L[i] += g.DeltaL[i]
		}
		p.step++
		p.maybeCheckpoint()
		return nil
	}
	var m []float64
	if g.Kind == scenario.KindSamples {
		// Recorded sensor data — push straight through, no synthesis,
		// no rng draw (so measuredSoFar is unaffected and seek's rng
		// reconstruction still lines up with the synthesis steps).
		m = append([]float64(nil), g.Raw...)
	} else {
		m = scenario.SynthMeasurement(
			p.truth.N, g.Dir.Theta, g.Dir.Phi,
			p.truth.K, p.truth.L, p.truth.N0, p.truth.Noise, rng,
		)
		p.measuredSoFar++
	}
	// Drain any stale Done signal so the post-Z <-kf.Done can't
	// pick up a leftover from a prior iteration.
	select {
	case <-p.kf.Done:
	default:
	}
	p.kf.U <- kalman.Matrix{m}
	p.kf.Z <- p.truth.N0 * p.truth.N0
	<-p.kf.Done
	p.step++
	p.maybeCheckpoint()
	return m
}

func (p *playback) maybeCheckpoint() {
	if p.step%checkpointEvery != 0 {
		return
	}
	if _, exists := p.checkpoints[p.step]; exists {
		return // already cached from a prior traversal
	}
	p.checkpoints[p.step] = &checkpoint{
		snap:     p.kf.Snapshot(),
		truthL:   append([]float64(nil), p.truth.L...),
		measured: p.measuredSoFar,
	}
}

// nearestCheckpoint returns the cached checkpoint at the highest step
// ≤ target. Step 0 is always present, so this never returns nil.
func (p *playback) nearestCheckpoint(target int) (atStep int, cp *checkpoint) {
	atStep = 0
	cp = p.checkpoints[0]
	for k, v := range p.checkpoints {
		if k <= target && k > atStep {
			atStep = k
			cp = v
		}
	}
	return
}

// tickOne advances exactly one step from the current position. Returns
// false if the scenario is already finished.
func (p *playback) tickOne(rng *rand.Rand) (g scenario.Generated, m []float64, ok bool) {
	if p.step >= len(p.gens) {
		return scenario.Generated{}, nil, false
	}
	g = p.gens[p.step]
	m = p.applyGen(g, rng)
	return g, m, true
}

// seekTo cancels any prior seek and starts a new seek goroutine that
// restores from the nearest cached checkpoint ≤ target and advances the
// residual. While the seek is running, p.seeking is true. The provided
// onComplete is invoked after the seek finishes (whether by completion
// or cancellation) so the caller can push a state update.
func (p *playback) seekTo(target int, onComplete func(success bool)) {
	if target < 0 {
		target = 0
	}
	if target > len(p.gens) {
		target = len(p.gens)
	}
	p.cancelSeek()

	ctx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	p.seeking = true
	p.seekCancel = cancel
	p.mu.Unlock()

	go func() {
		// Clear seeking flag BEFORE invoking the callback so the
		// status pushed inside it reports seeking=false. Use defer so
		// this happens whether the seek completes or is cancelled.
		success := false
		defer func() {
			p.mu.Lock()
			p.seeking = false
			p.seekCancel = nil
			p.mu.Unlock()
			onComplete(success)
		}()

		// Pick the nearest checkpoint ≤ target. If we're already past
		// that checkpoint and ≤ target, advance forward from where we
		// are; otherwise restore to the checkpoint and advance from
		// there.
		ckStep, cp := p.nearestCheckpoint(target)
		if p.step >= ckStep && p.step <= target {
			// Already in the right neighborhood; advance forward.
		} else {
			p.kf.Restore(cp.snap)
			p.truth.L = append(p.truth.L[:0], cp.truthL...)
			p.step = ckStep
			p.measuredSoFar = cp.measured
		}
		rng := p.rngAt(p.measuredSoFar)
		for p.step < target {
			select {
			case <-ctx.Done():
				return // defer fires onComplete(false)
			default:
			}
			_ = p.applyGen(p.gens[p.step], rng)
		}
		success = true
		// defer fires onComplete(true)
	}()
}

func (p *playback) cancelSeek() {
	p.mu.Lock()
	if p.seekCancel != nil {
		p.seekCancel()
	}
	p.mu.Unlock()
}

func (p *playback) isSeeking() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.seeking
}

// status snapshots the playback for transmission to the UI.
func (p *playback) status() playbackStatus {
	segment := ""
	if p.step > 0 && p.step <= len(p.gens) {
		segment = p.gens[p.step-1].Label
	} else if p.step < len(p.gens) {
		segment = p.gens[p.step].Label
	}
	return playbackStatus{
		Step:    p.step,
		Total:   len(p.gens),
		Segment: segment,
		Playing: p.playing,
		Seeking: p.isSeeking(),
		RateHz:  p.rateHz,
		Loaded:  p.loaded,
	}
}

// asParams returns a params struct populated from the loaded script so the
// UI can display the scenario's truth and filter config alongside its own
// scenario controls.
func (p *playback) asParams() params {
	k := append([]float64(nil), p.truth.K...)
	l := append([]float64(nil), p.truth.L...)
	fc := p.script.Filter
	out := params{
		Source:         scenarioSrc,
		N:              p.truth.N,
		N0:             p.truth.N0,
		KAct:           &k,
		LAct:           &l,
		SigmaK0:        fc.SigmaK0,
		SigmaK:         fc.SigmaK,
		SigmaM:         fc.SigmaM,
		MaxSigmaK:      fc.MaxSigmaK,
		MaxSigmaL:      fc.MaxSigmaL,
		InclinationDeg: p.truth.InclinationDeg,
		Noise:          p.truth.Noise,
	}
	if sm := fc.StateMachine; sm != nil {
		out.StateMachineOn = true
		out.LockHysteresis = sm.LockHysteresis
		out.NISWindow = sm.NISWindow
		out.NISThreshold = sm.NISThreshold
	}
	return out
}

// buildState reads the current filter and assembles a state + mode/nis/
// converged payload for the UI.
func (p *playback) buildState() (st state, mode string, nis float64, converged bool) {
	st = state{
		K: p.kf.K(),
		L: p.kf.L(),
		P: p.kf.P(),
	}
	mode = p.kf.Mode().String()
	nis = p.kf.NIS()
	converged = p.kf.Converged()
	return
}

