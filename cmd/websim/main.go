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
		outCh:  make(chan messageOut, 64),
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
	}
	if msg.Measure != nil && c.measurer != nil {
		c.measurement = c.measurer(msg.Measure.A)
		c.outCh <- messageOut{Measurement: &c.measurement}
	}
	if msg.Estimate != nil {
		nn := msg.Estimate.NN
		// Drain stale Done so the post-Z read is fresh.
		select {
		case <-c.estimator.Done:
		default:
		}
		c.estimator.U <- kalman.Matrix{c.measurement}
		c.estimator.Z <- nn
		<-c.estimator.Done
		pushManualState(c)
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
	if msg.SetMode != nil && c.pb != nil {
		switch *msg.SetMode {
		case "LCK":
			c.pb.kf.ForceLock()
		case "CAL":
			c.pb.kf.ForceUnlock()
		}
		pushPlaybackState(c, nil)
	}
	return ticker
}

func applyParams(c *connectionState, p params) {
	c.params = p
	switch p.Source {
	case manual:
		c.measurer, _ = makeManualMeasurer(p.N, p.N0, *p.KAct, *p.LAct, p.N0*p.SigmaM)
	case random:
		c.measurer, _ = makeRandomMeasurer(p.N, p.N0, *p.KAct, *p.LAct, p.N0*p.SigmaM)
	case scenarioSrc:
		// scenario source uses the playback object; no measurer assigned.
		c.measurer = nil
	case actual:
		// MPU9250 path is currently commented out in measurer.go; leave
		// measurer at its previous value rather than nilling it.
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
}

// applyPlaybackCmd updates the playback state per the command and returns
// the (possibly new) ticker that doTick will fire on.
func applyPlaybackCmd(c *connectionState, cmd playbackCmd, ticker *time.Ticker) *time.Ticker {
	switch cmd.Action {
	case "play":
		if cmd.RateHz > 0 {
			c.pb.rateHz = cmd.RateHz
		}
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
		// Seek runs in its own goroutine; on completion we resync the rng
		// and push a state update via outCh (writer goroutine handles it).
		target := cmd.Step
		c.pb.seekTo(target, func(success bool) {
			c.playbackRng = rand.New(rand.NewSource(c.pb.script.Seed))
			// Advance rng to match the post-seek step count. The seek goroutine
			// drives the filter with a fresh rng of its own; this just keeps
			// our outer rng in sync for any follow-up play ticks.
			for i := 0; i < c.pb.step; i++ {
				if c.pb.gens[i].Kind == scenario.KindPerturb {
					continue
				}
				// One call per measurement step matches SynthMeasurement's
				// rng consumption (n NormFloat64 calls, but we don't care
				// about exact byte parity here — we just need any deterministic
				// state for follow-up play to be reproducible.).
				_ = c.playbackRng.Float64()
			}
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
	if rateHz > 100 {
		rateHz = 100 // UI render cap; seek bypasses this.
	}
	return time.NewTicker(time.Second / time.Duration(rateHz))
}

// doTick advances the playback by one step and pushes a state update.
func doTick(c *connectionState) {
	g, m, ok := c.pb.tickOne(c.playbackRng)
	if !ok {
		return
	}
	// For perturb, truth changed — also refresh params on the wire.
	if g.Kind == scenario.KindPerturb {
		c.params = c.pb.asParams()
		c.outCh <- messageOut{Params: &c.params}
	}
	pushPlaybackState(c, m)
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
	mode := c.estimator.Mode().String()
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
	mode := c.estimator.Mode().String()
	nis := c.estimator.NIS()
	conv := c.estimator.Converged()
	c.outCh <- messageOut{
		State:     &st,
		Mode:      &mode,
		NIS:       &nis,
		Converged: &conv,
	}
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
