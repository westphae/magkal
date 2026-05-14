package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/westphae/magkal/internal/scenario"
	"github.com/westphae/magkal/pkg/kalman"
)

func (s state) String() string {
	var r strings.Builder

	r.WriteString("K: [")
	for i := 0; i < len(s.K); i++ {
		r.WriteString(fmt.Sprintf("%12.3g", (s.K)[i]))
		if i < len(s.K)-1 {
			r.WriteString(" ")
		}
	}
	r.WriteString("]\nL: [")

	for i := 0; i < len(s.K); i++ {
		r.WriteString(fmt.Sprintf("%12.3g", (s.L)[i]))
		if i < len(s.K)-1 {
			r.WriteString(" ")
		}
	}
	r.WriteString("]\nP: [")

	for i := 0; i < len(s.K); i++ {
		r.WriteString("[")
		for j := 0; j < len(s.K); j++ {
			r.WriteString(fmt.Sprintf("%12.3g", (s.P)[i][j]))
			if j < len(s.K)-1 {
				r.WriteString(" ")
			}
		}
		r.WriteString("]")
		if i < len(s.K)-1 {
			r.WriteString("\n    ")
		}
	}
	r.WriteString("]")

	return r.String()
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func main() {
	fs := http.FileServer(http.Dir("www"))
	http.Handle("/", fs)
	http.HandleFunc("/websocket", handleConnections)
	log.Println("Listening for connections on port 8000")
	log.Fatal(http.ListenAndServe(":8000", nil))
}

// connectionState holds everything the per-connection loop touches.
// Splitting it out keeps the main loop readable.
type connectionState struct {
	conn          *websocket.Conn
	params        params
	measurer      measurer
	estimator     *kalman.Filter
	measurement   measurement
	pb            *playback     // non-nil only when params.Source == scenario
	playbackRng   *rand.Rand    // measurement-noise rng for playback ticks; reset on load/reset/seek
	outCh         chan messageOut
	recorder      *recorder     // non-nil only when params.Source == actual && params.RecordFile != ""
	initBuf       *initBuffer   // non-nil while the user is in guided INIT mode
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to websocket: %s\n", err)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing websocket: %s\n", err.Error())
		}
	}()
	log.Println("A client opened a connection")

	// Single writer goroutine, fed by outCh. Multiple producers (read loop,
	// playback ticks, seek completions) are safe because conn.WriteJSON
	// is not goroutine-safe but the channel serializes us to one writer.
	c := &connectionState{
		conn:   conn,
		params: defaultParams,
		// Small buffer keeps the pause-to-visible-pause lag bounded; at
		// displayCapHz the worst-case backlog is ~1 second.
		outCh: make(chan messageOut, 16),
	}
	c.measurer, _ = makeManualMeasurer(c.params.N, c.params.N0, *c.params.KAct, *c.params.LAct, c.params.N0*c.params.SigmaM)
	c.estimator = kalman.NewKalmanFilter(c.params.N, c.params.N0, c.params.SigmaK0, c.params.SigmaK, c.params.SigmaM)

	closing := make(chan struct{})
	go writeLoop(conn, c.outCh, closing)

	// Initial message: params + state + scenarios.
	scenarios := scenarioPicks()
	initState := state{K: c.estimator.K(), L: c.estimator.L(), P: c.estimator.P()}
	mode := c.estimator.Mode().String()
	nis := c.estimator.NIS()
	converged := c.estimator.Converged()
	c.outCh <- messageOut{
		Params:    &c.params,
		State:     &initState,
		Scenarios: &scenarios,
		Mode:      &mode,
		NIS:       &nis,
		Converged: &converged,
	}

	// Ping-pong goroutine (unchanged in spirit; tolerant of conn closure).
	startPinger(conn)

	// Incoming-message channel; the read goroutine pushes parsed messages
	// onto it so the main loop can select against tickers too.
	inCh := make(chan messageIn, 16)
	go readLoop(conn, inCh, closing)

	var playTicker *time.Ticker
	defer func() {
		close(closing)
		if playTicker != nil {
			playTicker.Stop()
		}
		if c.pb != nil {
			c.pb.cancelSeek()
		}
		if c.recorder != nil {
			c.recorder.flush()
			c.recorder = nil
		}
	}()

	log.Println("Listening for messages from a new client")
	for {
		var tickC <-chan time.Time
		if playTicker != nil {
			tickC = playTicker.C
		}
		select {
		case msg, ok := <-inCh:
			if !ok {
				return
			}
			playTicker = handleMessage(c, msg, playTicker)
		case <-tickC:
			if c.pb != nil && c.pb.playing && !c.pb.isSeeking() {
				doTick(c)
				if c.pb.step >= len(c.pb.gens) {
					c.pb.playing = false
					playTicker.Stop()
					playTicker = nil
					pushPlaybackStatus(c)
				}
			}
		}
	}
}

// handleMessage processes one incoming client message. May reconfigure the
// playback ticker as a side effect; returns the (possibly new) ticker.
func handleMessage(c *connectionState, msg messageIn, ticker *time.Ticker) *time.Ticker {
	if msg.Params != nil {
		applyParams(c, *msg.Params)
		pushParamsAndState(c)
		// Re-scan the scripts dir each time the user picks the Scenario
		// source so freshly-recorded files appear in the dropdown without
		// requiring a page refresh.
		if c.params.Source == scenarioSrc {
			scenarios := scenarioPicks()
			c.outCh <- messageOut{Scenarios: &scenarios}
		}
	}
	if msg.Measure != nil && c.measurer != nil {
		c.measurement = c.measurer(msg.Measure.A)
		if c.recorder != nil {
			c.recorder.append(c.measurement)
		}
		c.outCh <- messageOut{Measurement: &c.measurement}
	}
	if msg.Estimate != nil {
		// Need a real measurement to drive the filter. The scenario path
		// pushes measurements directly through pb.kf; estimates here are
		// only meaningful for manual/random sources where a prior Measure
		// command populated c.measurement.
		if len(c.measurement) == 0 {
			log.Printf("Estimate received with no prior measurement; ignoring")
		} else if c.initBuf != nil {
			// INIT mode: accumulate min/max only; the filter sits idle so
			// we don't gradient-descend into a local minimum before the
			// hand-rotation seed lands.
			c.initBuf.add(c.measurement)
			stats := c.initBuf.stats()
			c.outCh <- messageOut{InitStats: &stats}
		} else {
			nn := msg.Estimate.NN
			select {
			case <-c.estimator.Done:
			default:
			}
			c.estimator.U <- kalman.Matrix{c.measurement}
			c.estimator.Z <- nn
			<-c.estimator.Done
			pushManualState(c)
		}
	}
	if msg.StartInit != nil && *msg.StartInit {
		c.initBuf = newInitBuffer(c.params.N)
		stats := c.initBuf.stats()
		mode := effectiveMode(c)
		c.outCh <- messageOut{Mode: &mode, InitStats: &stats}
	}
	if msg.FinishInit != nil && *msg.FinishInit {
		if c.initBuf == nil {
			log.Printf("FinishInit received with no active INIT buffer; ignoring")
		} else if c.initBuf.count == 0 {
			log.Printf("FinishInit received with empty buffer; ignoring")
			c.initBuf = nil
			pushManualState(c)
		} else {
			kSeed, lSeed := c.initBuf.seed(c.params.N0)
			log.Printf("INIT seed: k=%v l=%v (from %d samples)", kSeed, lSeed, c.initBuf.count)
			c.estimator.SeedKL(kSeed, lSeed)
			c.initBuf = nil
			pushManualState(c)
		}
	}
	if msg.LoadScenario != nil {
		pb, err := loadScenario(*msg.LoadScenario)
		if err != nil {
			log.Printf("loadScenario %q: %v", *msg.LoadScenario, err)
			return ticker
		}
		if c.pb != nil {
			c.pb.cancelSeek()
		}
		c.pb = pb
		c.playbackRng = rand.New(rand.NewSource(pb.script.Seed))
		c.params = pb.asParams()
		if ticker != nil {
			ticker.Stop()
			ticker = nil
		}
		pushParamsAndState(c)
		pushPlaybackStatus(c)
	}
	if msg.PlaybackCmd != nil && c.pb != nil {
		ticker = applyPlaybackCmd(c, *msg.PlaybackCmd, ticker)
	}
	if msg.SetMode != nil {
		// Route the manual override to whichever filter is live for the
		// current source: the playback's filter in Scenario mode, the
		// top-level estimator otherwise. ForceLock/ForceUnlock work
		// independent of EnableStateMachine, so Actual-mode users can
		// freeze the filter without setting up the state-machine knobs.
		var kf *kalman.Filter
		if c.pb != nil {
			kf = c.pb.kf
		} else {
			kf = c.estimator
		}
		switch *msg.SetMode {
		case "LCK":
			kf.ForceLock()
		case "CAL":
			kf.ForceUnlock()
		}
		if c.pb != nil {
			pushPlaybackState(c, nil)
		} else {
			pushManualState(c)
		}
	}
	return ticker
}

func applyParams(c *connectionState, p params) {
	c.params = p
	reconcileRecorder(c, p)
	// Restart blows away any in-progress INIT calibration. The user has
	// to re-enter INIT after a Restart if they want to seed again.
	c.initBuf = nil
	switch p.Source {
	case manual:
		c.measurer, _ = makeManualMeasurer(p.N, p.N0, *p.KAct, *p.LAct, p.N0*p.SigmaM)
	case random:
		c.measurer, _ = makeRandomMeasurer(p.N, p.N0, *p.KAct, *p.LAct, p.N0*p.SigmaM)
	case scenarioSrc:
		// scenario source uses the playback object; no measurer assigned.
		c.measurer = nil
	case actual:
		m, err := makeActualMeasurer()
		if err != nil {
			log.Printf("makeActualMeasurer failed (keeping previous source): %v", err)
			break
		}
		c.measurer = m
	case file:
		// TODO: implement file source.
	default:
		c.measurer, _ = makeManualMeasurer(p.N, p.N0, *p.KAct, *p.LAct, p.N0*p.SigmaM)
		log.Printf("Received bad source: %d, setting Manual measurer", p.Source)
	}
	// Rebuild estimator. For scenario source, the playback object owns its
	// own filter so the top-level estimator is unused there but we keep
	// it valid for manual/random source transitions.
	c.estimator = kalman.NewKalmanFilter(p.N, p.N0, p.SigmaK0, p.SigmaK, p.SigmaM)
	if p.MaxSigmaK > 0 || p.MaxSigmaL > 0 {
		c.estimator.SetConvergenceThresholds(p.MaxSigmaK, p.MaxSigmaL)
	}
	if p.StateMachineOn && p.LockHysteresis > 0 && p.NISWindow > 0 && p.NISThreshold > 0 {
		c.estimator.EnableStateMachine(p.LockHysteresis, p.NISWindow, p.NISThreshold)
	}
	// Actual source: seed the freshly-built filter from the client's
	// persisted best estimate so Restart resumes from the last known
	// calibration instead of (k=1, l=0). The seed defaults to (1, 0) on
	// the client side, so first-time users still get the cold-start
	// behavior; only matters after the user has run INIT (or edited the
	// best-estimate values directly).
	if p.Source == actual && p.SeedK != nil && p.SeedL != nil &&
		len(*p.SeedK) >= p.N && len(*p.SeedL) >= p.N {
		c.estimator.SeedKL((*p.SeedK)[:p.N], (*p.SeedL)[:p.N])
	}
}

// reconcileRecorder starts, flushes, or rotates c.recorder so that exactly
// one recorder is alive iff source==actual and RecordFile is non-empty,
// keyed by (filename, n, n0). Any (filename, n, n0) change closes the prior
// recording session and starts a new one — producing a new labeled
// `samples` step in the YAML.
func reconcileRecorder(c *connectionState, p params) {
	want := p.Source == actual && p.RecordFile != ""
	if c.recorder != nil {
		stale := !want ||
			c.recorder.filename != normalizeRecordFile(p.RecordFile) ||
			c.recorder.n != p.N ||
			c.recorder.n0 != p.N0
		if stale {
			c.recorder.flush()
			c.recorder = nil
		}
	}
	if want && c.recorder == nil {
		c.recorder = newRecorder(p.RecordFile, p.N, p.N0, p.SigmaK0, p.SigmaK, p.SigmaM)
	}
}

// normalizeRecordFile mirrors newRecorder's basename+extension handling so
// reconcileRecorder can compare the requested filename to the live one
// without false rotations.
func normalizeRecordFile(s string) string {
	r := newRecorder(s, 0, 0, 0, 0, 0)
	return r.filename
}

// applyPlaybackCmd updates the playback state per the command and returns
// the (possibly new) ticker that doTick will fire on.
func applyPlaybackCmd(c *connectionState, cmd playbackCmd, ticker *time.Ticker) *time.Ticker {
	switch cmd.Action {
	case "play":
		if cmd.RateHz > 0 {
			c.pb.rateHz = cmd.RateHz
		}
		c.pb.recomputeSendEvery()
		c.pb.playing = true
		ticker = restartTicker(ticker, c.pb.rateHz)
		pushPlaybackStatus(c)
	case "pause":
		c.pb.playing = false
		if ticker != nil {
			ticker.Stop()
		}
		ticker = nil
		pushPlaybackStatus(c)
	case "step":
		c.pb.playing = false
		if ticker != nil {
			ticker.Stop()
		}
		ticker = nil
		if c.pb.step < len(c.pb.gens) && !c.pb.isSeeking() {
			doTick(c)
		}
		pushPlaybackStatus(c)
	case "setRate":
		if cmd.RateHz > 0 {
			c.pb.rateHz = cmd.RateHz
		}
		c.pb.recomputeSendEvery()
		if c.pb.playing {
			ticker = restartTicker(ticker, c.pb.rateHz)
		}
		pushPlaybackStatus(c)
	case "seek":
		c.pb.playing = false
		if ticker != nil {
			ticker.Stop()
		}
		ticker = nil
		// Seek runs in its own goroutine; on completion we resync the
		// outer playbackRng so subsequent play ticks consume from where
		// the seek left off. Push a state update via outCh.
		target := cmd.Step
		c.pb.seekTo(target, func(success bool) {
			c.playbackRng = c.pb.CurrentRng()
			pushPlaybackState(c, nil)
			pushPlaybackStatus(c)
		})
		pushPlaybackStatus(c)
	case "reset":
		c.pb.playing = false
		if ticker != nil {
			ticker.Stop()
		}
		ticker = nil
		c.pb.reset()
		c.playbackRng = rand.New(rand.NewSource(c.pb.script.Seed))
		c.params = c.pb.asParams()
		pushParamsAndState(c)
		pushPlaybackStatus(c)
	case "applyFilter":
		// Reconfigure the loaded scenario's filter mid-run. Lets the
		// user toggle the state machine and tune thresholds from the
		// UI without editing the YAML. Only affects pb.kf; the
		// scenario script's stored config is untouched so Reset still
		// uses the YAML values.
		c.pb.kf.SetConvergenceThresholds(cmd.MaxSigmaK, cmd.MaxSigmaL)
		if cmd.StateMachineOn && cmd.LockHysteresis > 0 && cmd.NISWindow > 0 && cmd.NISThreshold > 0 {
			c.pb.kf.EnableStateMachine(cmd.LockHysteresis, cmd.NISWindow, cmd.NISThreshold)
		} else {
			c.pb.kf.DisableStateMachine()
		}
		// Reflect the new effective config back to the client so the
		// scenario-info card stays accurate.
		c.params.MaxSigmaK = cmd.MaxSigmaK
		c.params.MaxSigmaL = cmd.MaxSigmaL
		c.params.StateMachineOn = cmd.StateMachineOn
		c.params.LockHysteresis = cmd.LockHysteresis
		c.params.NISWindow = cmd.NISWindow
		c.params.NISThreshold = cmd.NISThreshold
		c.outCh <- messageOut{Params: &c.params}
		pushPlaybackState(c, nil)
	}
	return ticker
}

func restartTicker(old *time.Ticker, rateHz int) *time.Ticker {
	if old != nil {
		old.Stop()
	}
	if rateHz < 1 {
		rateHz = 1
	}
	if rateHz > 1000 {
		rateHz = 1000 // sanity cap. Display is downsampled to displayCapHz.
	}
	return time.NewTicker(time.Second / time.Duration(rateHz))
}

// doTick advances the playback by one step and (conditionally) pushes a
// state update. To keep the client renderable when rateHz is high, sends
// are downsampled to ~displayCapHz; segment boundaries, perturb events,
// and the final step always push so the user doesn't miss them.
func doTick(c *connectionState) {
	g, m, ok := c.pb.tickOne(c.playbackRng)
	if !ok {
		return
	}
	// For perturb, truth changed — refresh params on the wire and always
	// push state so the UI shows the new environment immediately.
	if g.Kind == scenario.KindPerturb {
		c.params = c.pb.asParams()
		c.outCh <- messageOut{Params: &c.params}
		pushPlaybackState(c, m)
		c.pb.sinceLastSend = 0
		c.pb.lastSentLabel = g.Label
		return
	}
	c.pb.sinceLastSend++
	atBoundary := g.Label != c.pb.lastSentLabel
	atEnd := c.pb.step >= len(c.pb.gens)
	if c.pb.sinceLastSend >= c.pb.sendEvery || atBoundary || atEnd {
		pushPlaybackState(c, m)
		c.pb.sinceLastSend = 0
		c.pb.lastSentLabel = g.Label
	}
}

// pushPlaybackState pushes a messageOut with the current filter state,
// mode/nis/converged, and playback status. If m is non-nil it's included
// as a measurement too (so the existing magXS plot still updates).
func pushPlaybackState(c *connectionState, m []float64) {
	st, mode, nis, conv := c.pb.buildState()
	status := c.pb.status()
	out := messageOut{
		State:     &st,
		Mode:      &mode,
		NIS:       &nis,
		Converged: &conv,
		Playback:  &status,
	}
	if m != nil {
		meas := measurement(m)
		out.Measurement = &meas
	}
	c.outCh <- out
}

func pushPlaybackStatus(c *connectionState) {
	if c.pb == nil {
		return
	}
	status := c.pb.status()
	c.outCh <- messageOut{Playback: &status}
}

func pushParamsAndState(c *connectionState) {
	if c.pb != nil {
		st, mode, nis, conv := c.pb.buildState()
		c.outCh <- messageOut{
			Params:    &c.params,
			State:     &st,
			Mode:      &mode,
			NIS:       &nis,
			Converged: &conv,
		}
		return
	}
	st := state{K: c.estimator.K(), L: c.estimator.L(), P: c.estimator.P()}
	mode := effectiveMode(c)
	nis := c.estimator.NIS()
	conv := c.estimator.Converged()
	c.outCh <- messageOut{
		Params:    &c.params,
		State:     &st,
		Mode:      &mode,
		NIS:       &nis,
		Converged: &conv,
	}
}

func pushManualState(c *connectionState) {
	st := state{K: c.estimator.K(), L: c.estimator.L(), P: c.estimator.P()}
	mode := effectiveMode(c)
	nis := c.estimator.NIS()
	conv := c.estimator.Converged()
	c.outCh <- messageOut{
		State:     &st,
		Mode:      &mode,
		NIS:       &nis,
		Converged: &conv,
	}
}

// effectiveMode returns "INIT" while the guided-calibration buffer is
// active, otherwise the EKF's own mode ("CAL" / "LCK"). The scenario
// path uses pb.kf.Mode() directly — INIT only applies to the top-level
// estimator (manual/random/actual sources).
func effectiveMode(c *connectionState) string {
	if c.initBuf != nil {
		return "INIT"
	}
	return c.estimator.Mode().String()
}

func readLoop(conn *websocket.Conn, inCh chan<- messageIn, closing <-chan struct{}) {
	defer close(inCh)
	for {
		var msg messageIn
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("Error reading from websocket: %v", err)
			return
		}
		select {
		case inCh <- msg:
		case <-closing:
			return
		}
	}
}

func writeLoop(conn *websocket.Conn, outCh <-chan messageOut, closing <-chan struct{}) {
	for {
		select {
		case msg, ok := <-outCh:
			if !ok {
				return
			}
			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("Error writing to websocket: %v", err)
				return
			}
		case <-closing:
			return
		}
	}
}

func startPinger(conn *websocket.Conn) {
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for range t.C {
			if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(10*time.Second)); err != nil {
				log.Printf("ws error sending ping: %s", err)
				return
			}
		}
	}()
	conn.SetPongHandler(func(string) error { return nil })
}
