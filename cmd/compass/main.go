package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/westphae/go-iio/icm20948"
)

var (
	addr        = flag.String("addr", "0.0.0.0:8001", "listen address for HTTP/websocket")
	rateHz      = flag.Int("hz", 10, "UI/CSV emit rate (Hz)")
	imuHz       = flag.Int("imu-hz", 100, "IIO trigger rate (Hz); 100 captures every fresh AK09916 mag sample for boxcar averaging")
	wwwDir      = flag.String("www", "", "path to www/ (defaults to ./www relative to binary)")
	magTau      = flag.Float64("mag-tau", 0.5, "EMA time constant (s) on the (already-averaged) mag vector feeding heading; 0 disables")
	accelDLPF   = flag.Float64("accel-dlpf-hz", 11.5, "on-chip accel DLPF cutoff (Hz); snapped to nearest kernel-available value")
	gyroDLPF    = flag.Float64("gyro-dlpf-hz", 11.6, "on-chip gyro DLPF cutoff (Hz); snapped to nearest kernel-available value")
	accelScaleG = flag.Int("accel-scale-g", 2, "accel full-scale range (G): 2/4/8/16")
	gyroScaleDps = flag.Int("gyro-scale-dps", 250, "gyro full-scale range (dps): 250/500/1000/2000")
)

// runtime holds the shared, mutable server-side state.
type runtime struct {
	k          []float64
	l          []float64
	n          int
	gps        *gpsSource
	geomag     *geomagState
	imuSample  atomic.Pointer[icm20948.Sample]
	tiltCompOn atomic.Bool
	alignMu    sync.RWMutex
	yawOffset  float64 // rad
	alignAt    time.Time
	bc         *broadcaster
	// magEMA smooths the (already boxcar-averaged) mag vector that feeds the
	// heading computation. The unsmoothed magCal still goes to UI/CSV; only
	// the heading path sees the EMA output.
	magEMA *vec3EMA
	// magAcc accumulates fresh mag-calibrated samples since the last emit
	// tick; emitFrame drains it for boxcar averaging at rateHz. Accel/gyro
	// already get their noise reduction from the on-chip DLPF, so they're
	// just sampled at the latest value (via rt.imuSample).
	magAcc magAccumulator
	// Latest values used by the heading display, so doAlign can capture the
	// same vectors the user sees on the dial.
	smoothAccel atomic.Pointer[vec3]
	smoothMag   atomic.Pointer[vec3]
}

// magAccumulator collects calibrated-mag vectors arriving at the trigger rate
// and returns their arithmetic mean. Drain resets the accumulator. Safe under
// concurrent Add (trigger goroutine) and Drain (emit goroutine).
type magAccumulator struct {
	mu  sync.Mutex
	sum vec3
	n   int
}

func (a *magAccumulator) add(v vec3) {
	a.mu.Lock()
	a.sum.X += v.X
	a.sum.Y += v.Y
	a.sum.Z += v.Z
	a.n++
	a.mu.Unlock()
}

// drain returns the mean of every sample added since the last drain, plus the
// sample count for diagnostics. Returns ok=false if no samples have arrived.
func (a *magAccumulator) drain() (vec3, int, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.n == 0 {
		return vec3{}, 0, false
	}
	n := float64(a.n)
	avg := vec3{a.sum.X / n, a.sum.Y / n, a.sum.Z / n}
	count := a.n
	a.sum, a.n = vec3{}, 0
	return avg, count, true
}

// vec3EMA is a single-pole low-pass filter on a 3-vector. alpha = dt/(tau+dt)
// is fixed at construction; tau<=0 disables filtering (alpha=1).
type vec3EMA struct {
	alpha float64
	prev  vec3
	init  bool
}

func newVec3EMA(dt, tau float64) *vec3EMA {
	a := 1.0
	if tau > 0 {
		a = dt / (tau + dt)
	}
	return &vec3EMA{alpha: a}
}

func (f *vec3EMA) update(v vec3) vec3 {
	if !f.init {
		f.prev = v
		f.init = true
		return v
	}
	f.prev.X = f.alpha*v.X + (1-f.alpha)*f.prev.X
	f.prev.Y = f.alpha*v.Y + (1-f.alpha)*f.prev.Y
	f.prev.Z = f.alpha*v.Z + (1-f.alpha)*f.prev.Z
	return f.prev
}

func (rt *runtime) setAlign(yawOffset float64, at time.Time) {
	rt.alignMu.Lock()
	rt.yawOffset = yawOffset
	rt.alignAt = at
	rt.alignMu.Unlock()
}

func (rt *runtime) getAlign() (float64, time.Time) {
	rt.alignMu.RLock()
	defer rt.alignMu.RUnlock()
	return rt.yawOffset, rt.alignAt
}

// broadcaster fans out one messageOut to any number of websocket subscribers.
// Each subscriber has a small buffer; slow consumers see frames dropped (the
// CSV log is the persistent record, so the UI dropping the occasional frame
// during a stall is acceptable).
type broadcaster struct {
	mu   sync.Mutex
	subs map[chan messageOut]struct{}
}

func newBroadcaster() *broadcaster {
	return &broadcaster{subs: map[chan messageOut]struct{}{}}
}

func (b *broadcaster) subscribe() chan messageOut {
	ch := make(chan messageOut, 8)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *broadcaster) unsubscribe(ch chan messageOut) {
	b.mu.Lock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *broadcaster) publish(m messageOut) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- m:
		default:
		}
	}
}

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	dt := 1.0 / float64(*rateHz)
	rt := &runtime{
		bc:     newBroadcaster(),
		magEMA: newVec3EMA(dt, *magTau),
	}
	rt.tiltCompOn.Store(true)
	log.Printf("compass: imu=%d Hz, emit=%d Hz, accel/gyro DLPF=%.1f/%.1f Hz, mag tau=%.2fs",
		*imuHz, *rateHz, *accelDLPF, *gyroDLPF, *magTau)

	best, err := loadBest()
	if err != nil {
		log.Fatalf("compass: load best_fit.json: %v", err)
	}
	if best == nil {
		log.Fatalf("compass: no best_fit.json in %s — run cmd/websim and Save Best first", mustDir())
	}
	rt.k = best.K
	rt.l = best.L
	rt.n = best.N
	if rt.n != 3 || len(rt.k) != 3 || len(rt.l) != 3 {
		log.Fatalf("compass: best_fit.json must be 3-axis (got n=%d, |k|=%d, |l|=%d)", rt.n, len(rt.k), len(rt.l))
	}

	fallbackN0 := 50.0
	if cfg, err := loadConfig(); err == nil && cfg != nil && cfg.N0 > 0 {
		fallbackN0 = cfg.N0
	}
	rt.geomag = newGeomagState(fallbackN0)
	rt.gps = &gpsSource{}

	if al, err := loadAlign(); err == nil && al != nil {
		rt.setAlign(al.YawOffsetRad, al.SavedAt)
		log.Printf("compass: loaded align.json: yaw_offset=%.2f° saved=%s",
			al.YawOffsetRad*180/math.Pi, al.SavedAt.Format(time.RFC3339))
	} else {
		log.Printf("compass: no align.json (yaw_offset = 0)")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	imuCh, err := runIMU(ctx, imuConfig{
		HzTrigger:    *imuHz,
		AccelDLPFHz:  *accelDLPF,
		GyroDLPFHz:   *gyroDLPF,
		AccelScaleG:  *accelScaleG,
		GyroScaleDps: *gyroScaleDps,
	})
	if err != nil {
		log.Fatalf("compass: imu: %v", err)
	}
	go func() {
		for s := range imuCh {
			ss := s
			rt.imuSample.Store(&ss)
			// Per-sample boxcar contribution: every fresh mag reading
			// gets averaged into the next emit's heading vector.
			magRaw := vec3{ss.MagX, ss.MagY, ss.MagZ}
			rt.magAcc.add(ApplyCal(magRaw, rt.k, rt.l))
		}
	}()

	go runGPS(ctx, rt.gps)
	go runGeomag(ctx, rt.gps, rt.geomag)

	rec, err := newRecorder(time.Now())
	if err != nil {
		log.Fatalf("compass: recorder: %v", err)
	}
	defer rec.Close()
	log.Printf("compass: recording to %s", rec.Path())
	go runFlusher(ctx, rec)

	// Server-driven tick loop: writes CSV regardless of client connections.
	go runTickLoop(ctx, rt, rec)

	wwwPath := resolveWWW(*wwwDir)
	http.Handle("/", http.FileServer(http.Dir(wwwPath)))
	http.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		handleConn(ctx, rt, w, r)
	})

	srv := &http.Server{Addr: *addr}
	srvErr := make(chan error, 1)
	go func() { srvErr <- srv.ListenAndServe() }()
	log.Printf("compass: listening on %s (www=%s)", *addr, wwwPath)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-srvErr:
		log.Printf("compass: http: %v", err)
	case <-sigCh:
		log.Printf("compass: shutdown signal received")
	}
	cancel()
	shutdownCtx, scancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer scancel()
	_ = srv.Shutdown(shutdownCtx)
}

func mustDir() string {
	d, _ := magkalDir()
	return d
}

func resolveWWW(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if _, err := os.Stat("www"); err == nil {
		return "www"
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Join(filepath.Dir(exe), "www")
		if _, err := os.Stat(d); err == nil {
			return d
		}
	}
	return "www"
}

// runTickLoop pulls the latest IMU + GPS + geomag + align state at rateHz,
// computes derived values, writes a CSV row, and broadcasts a messageOut to
// any connected websocket. Skips a tick (no CSV, no broadcast) until the
// first IMU sample arrives.
func runTickLoop(ctx context.Context, rt *runtime, rec *recorder) {
	t := time.NewTicker(time.Second / time.Duration(*rateHz))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			emitFrame(rt, rec)
		}
	}
}

func emitFrame(rt *runtime, rec *recorder) {
	smp := rt.imuSample.Load()
	if smp == nil {
		return
	}
	accel := vec3{smp.AccelX, smp.AccelY, smp.AccelZ}
	gyro := vec3{smp.GyroX, smp.GyroY, smp.GyroZ}
	magRaw := vec3{smp.MagX, smp.MagY, smp.MagZ}
	magCal := ApplyCal(magRaw, rt.k, rt.l)

	// Heading uses the boxcar mean of every mag sample since the last emit
	// (√N reduction in mag noise vs. taking the latest), then a single-pole
	// EMA to soften residual flicker. Accel comes through the on-chip DLPF,
	// so we use the latest sample without additional smoothing. UI/CSV
	// continue to show the raw single-sample magCal.
	magBoxcar, _, magBoxcarOK := rt.magAcc.drain()
	if !magBoxcarOK {
		magBoxcar = magCal
	}
	magCalF := rt.magEMA.update(magBoxcar)
	af, mf := accel, magCalF
	rt.smoothAccel.Store(&af)
	rt.smoothMag.Store(&mf)

	tco := rt.tiltCompOn.Load()
	var headingSensor float64
	var headingOK bool
	if tco {
		headingSensor, headingOK = HeadingSensorTiltOn(accel, magCalF)
	} else {
		headingSensor = HeadingSensorTiltOff(magCalF)
		headingOK = true
	}
	yaw, savedAt := rt.getAlign()
	headingVeh := VehicleHeading(headingSensor, yaw)

	gpsFix := rt.gps.latest()
	n0Ut, declDeg, inclDeg, fUt, hUt, xUt, yUt, zUt, fallback := rt.geomag.getAll()
	trackTrueDeg := gpsFix.TrackTrue
	trackMagDeg := math.NaN()
	if !math.IsNaN(trackTrueDeg) {
		trackMagDeg = wrapDeg(trackTrueDeg - declDeg)
	}

	var magPred vec3
	var magPredOK bool
	if !math.IsNaN(trackMagDeg) {
		magPred, magPredOK = PredictRawMag(trackMagDeg*math.Pi/180, n0Ut, inclDeg, yaw, accel, rt.k, rt.l)
	}
	if !magPredOK {
		magPred = vec3{math.NaN(), math.NaN(), math.NaN()}
	}

	now := time.Now()
	row := recordRow{
		TWall:            now,
		TIMU:             smp.Time,
		Accel:            accel,
		Gyro:             gyro,
		MagRaw:           magRaw,
		MagCal:           magCal,
		MagPred:          magPred,
		TiltComp:         tco,
		TempC:            smp.TempC,
		GPS:              gpsFix,
		TrackMagDeg:      trackMagDeg,
		N0Ut:             n0Ut,
		DeclDeg:          declDeg,
		InclDeg:          inclDeg,
		YawOffsetDeg:     yaw * 180 / math.Pi,
		HeadingSensorDeg: math.NaN(),
		HeadingVehDeg:    math.NaN(),
	}
	if headingOK {
		row.HeadingSensorDeg = headingSensor * 180 / math.Pi
		row.HeadingVehDeg = headingVeh * 180 / math.Pi
	}
	if err := rec.Write(row); err != nil {
		log.Printf("compass: csv write: %v", err)
	}

	imu := imuPayload{
		T:      smp.Time,
		Accel:  accel,
		Gyro:   gyro,
		MagRaw: magRaw,
		MagCal: magCal,
		TempC:  smp.TempC,
	}
	gps := gpsPayload{
		T:            gpsFix.T,
		Lat:          nf(gpsFix.Lat),
		Lon:          nf(gpsFix.Lon),
		AltM:         nf(gpsFix.AltMSL),
		TrackTrueDeg: nf(trackTrueDeg),
		TrackMagDeg:  nf(trackMagDeg),
		SpeedMps:     nf(gpsFix.SpeedMps),
		Mode:         gpsFix.Mode,
	}
	gm := geomagPayload{
		N0Ut: n0Ut, DeclDeg: declDeg, InclDeg: inclDeg,
		FUt: fUt, HUt: hUt, XUt: xUt, YUt: yUt, ZUt: zUt,
		Fallback: fallback,
	}
	align := alignPayload{YawOffsetDeg: yaw * 180 / math.Pi, SavedAt: savedAt}

	out := messageOut{
		IMU:    &imu,
		GPS:    &gps,
		Geomag: &gm,
		Align:  &align,
	}
	if headingOK {
		hs := headingSensor * 180 / math.Pi
		hv := headingVeh * 180 / math.Pi
		out.HeadingSensorDeg = &hs
		out.HeadingVehDeg = &hv
	}
	if magPredOK {
		out.Predicted = &predictedPayload{MagPred: magPred}
	}
	rt.bc.publish(out)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

func handleConn(ctx context.Context, rt *runtime, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("compass: upgrade: %v", err)
		return
	}
	defer conn.Close()
	log.Printf("compass: client connected (%s)", r.RemoteAddr)

	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sub := rt.bc.subscribe()
	defer rt.bc.unsubscribe(sub)

	// Initial snapshot: calibration + align + tilt-comp.
	yaw, savedAt := rt.getAlign()
	yawDeg := yaw * 180 / math.Pi
	tco := rt.tiltCompOn.Load()
	if err := conn.WriteJSON(messageOut{
		Cal:      &calPayload{K: rt.k, L: rt.l},
		Align:    &alignPayload{YawOffsetDeg: yawDeg, SavedAt: savedAt},
		TiltComp: &tco,
	}); err != nil {
		return
	}

	inCh := make(chan messageIn, 8)
	go readLoop(conn, inCh, cancel)

	for {
		select {
		case <-connCtx.Done():
			return
		case msg := <-inCh:
			handleClientMessage(rt, msg, conn)
		case frame := <-sub:
			if err := conn.WriteJSON(frame); err != nil {
				return
			}
		}
	}
}

func handleClientMessage(rt *runtime, msg messageIn, conn *websocket.Conn) {
	if msg.TiltComp != nil {
		rt.tiltCompOn.Store(*msg.TiltComp)
		tc := *msg.TiltComp
		_ = conn.WriteJSON(messageOut{TiltComp: &tc})
	}
	if msg.Action == "align" {
		if err := doAlign(rt); err != nil {
			_ = conn.WriteJSON(messageOut{Error: err.Error()})
			return
		}
		yaw, savedAt := rt.getAlign()
		yawDeg := yaw * 180 / math.Pi
		_ = conn.WriteJSON(messageOut{Align: &alignPayload{YawOffsetDeg: yawDeg, SavedAt: savedAt}})
	}
}

func doAlign(rt *runtime) error {
	if rt.imuSample.Load() == nil {
		return fmt.Errorf("no IMU sample yet")
	}
	accelP := rt.smoothAccel.Load()
	magP := rt.smoothMag.Load()
	if accelP == nil || magP == nil {
		return fmt.Errorf("filter not warmed up yet")
	}
	fix := rt.gps.latest()
	if !fix.Valid || math.IsNaN(fix.TrackTrue) {
		return fmt.Errorf("GPS fix or track unavailable")
	}
	_, declDeg, _, _ := rt.geomag.get()
	trackMag := wrapPi((fix.TrackTrue - declDeg) * math.Pi / 180)

	var hs float64
	if rt.tiltCompOn.Load() {
		h, ok := HeadingSensorTiltOn(*accelP, *magP)
		if !ok {
			return fmt.Errorf("degenerate accel/mag at align")
		}
		hs = h
	} else {
		hs = HeadingSensorTiltOff(*magP)
	}
	yaw := wrapPi(trackMag - hs)
	now := time.Now()
	if err := saveAlign(&alignFile{YawOffsetRad: yaw, SavedAt: now}); err != nil {
		return fmt.Errorf("save align.json: %w", err)
	}
	rt.setAlign(yaw, now)
	log.Printf("compass: align: heading_sensor=%.2f° track_mag=%.2f° yaw_offset=%.2f°",
		hs*180/math.Pi, trackMag*180/math.Pi, yaw*180/math.Pi)
	return nil
}

// nf returns nil for NaN, otherwise a pointer to x. JSON-encodes NaN as
// `null`, which encoding/json cannot otherwise represent.
func nf(x float64) *float64 {
	if math.IsNaN(x) {
		return nil
	}
	return &x
}

func wrapDeg(d float64) float64 {
	for d > 180 {
		d -= 360
	}
	for d <= -180 {
		d += 360
	}
	return d
}

func readLoop(conn *websocket.Conn, inCh chan<- messageIn, cancel context.CancelFunc) {
	defer cancel()
	for {
		var msg messageIn
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		inCh <- msg
	}
}
