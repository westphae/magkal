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
		script: s,
		gens:   scenario.ExpandAll(s),
		truth:  cloneTruth(s.Truth),
		loaded: name,
		rateHz: 10,
	}
	pb.recomputeSendEvery()
	pb.rebuildFilter()
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
// Cancels any running seek. Does not change playing state.
func (p *playback) reset() {
	p.cancelSeek()
	p.rebuildFilter()
	p.truth = cloneTruth(p.script.Truth)
	p.step = 0
}

// applyGen consumes one Generated entry: for perturb, mutates truth in
// place; for measurement kinds, synthesizes a measurement using the given
// rng and drives the filter. step is advanced. Returns the synthesized
// measurement (nil for perturb steps) for downstream rendering.
func (p *playback) applyGen(g scenario.Generated, rng *rand.Rand) []float64 {
	if g.Kind == scenario.KindPerturb {
		for i := range p.truth.L {
			p.truth.L[i] += g.DeltaL[i]
		}
		p.step++
		return nil
	}
	m := scenario.SynthMeasurement(
		p.truth.N, g.Dir.Theta, g.Dir.Phi,
		p.truth.K, p.truth.L, p.truth.N0, p.truth.Noise, rng,
	)
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
	return m
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

// seekTo cancels any prior seek, starts a new seek goroutine that
// resets the playback to step 0 and advances to target. While the seek
// is running, p.seeking is true. The provided onComplete is invoked
// after the seek finishes (whether by completion or cancellation) so
// the caller can push a state update. The bool argument is true for
// successful completion, false if cancelled.
//
// rng is created fresh inside the goroutine so seeks are deterministic
// w.r.t. the script.Seed.
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
		defer func() {
			p.mu.Lock()
			p.seeking = false
			p.seekCancel = nil
			p.mu.Unlock()
		}()
		p.reset() // includes its own cancelSeek which is harmless now
		rng := rand.New(rand.NewSource(p.script.Seed))
		for p.step < target {
			select {
			case <-ctx.Done():
				onComplete(false)
				return
			default:
			}
			_ = p.applyGen(p.gens[p.step], rng)
		}
		onComplete(true)
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
		Source:    scenarioSrc,
		N:         p.truth.N,
		N0:        p.truth.N0,
		KAct:      &k,
		LAct:      &l,
		SigmaK0:   fc.SigmaK0,
		SigmaK:    fc.SigmaK,
		SigmaM:    fc.SigmaM,
		MaxSigmaK: fc.MaxSigmaK,
		MaxSigmaL: fc.MaxSigmaL,
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

