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
	addr         = flag.String("addr", "0.0.0.0:8001", "listen address for HTTP/websocket")
	rateHz       = flag.Int("hz", 10, "UI/CSV emit rate (Hz)")
	imuHz        = flag.Int("imu-hz", 100, "IIO trigger rate (Hz); 100 captures every fresh AK09916 mag sample for boxcar averaging")
	wwwDir       = flag.String("www", "", "path to www/ (defaults to ./www relative to binary)")
	magTau       = flag.Float64("mag-tau", 0.5, "EMA time constant (s) on the (already-averaged) mag vector when the UI EMA toggle is on")
	alignTau     = flag.Float64("align-tau", 1.0, "long EMA time constant (s) on accel+mag used for the Align snapshot; always on")
	accelDLPF    = flag.Float64("accel-dlpf-hz", 11.5, "on-chip accel DLPF cutoff (Hz); snapped to nearest kernel-available value")
	gyroDLPF     = flag.Float64("gyro-dlpf-hz", 11.6, "on-chip gyro DLPF cutoff (Hz); snapped to nearest kernel-available value")
	accelScaleG  = flag.Int("accel-scale-g", 2, "accel full-scale range (G): 2/4/8/16")
	gyroScaleDps = flag.Int("gyro-scale-dps", 250, "gyro full-scale range (dps): 250/500/1000/2000")
)

// runtime holds the shared, mutable server-side state.
type runtime struct {
	k         []float64
	l         []float64
	n         int
	gps       *gpsSource
	geomag    *geomagState
	imuSample atomic.Pointer[icm20948.Sample]

	// align is the current sensor→vehicle rotation captured at Align time,
	// plus the heading the user supplied as truth. nil means "no alignment
	// yet"; heading is suppressed in the UI/CSV until Align is pressed.
	align atomic.Pointer[alignState]

	bc *broadcaster

	// magEMAOn gates the optional display-path EMA. Default off — the
	// boxcar mean over imuHz/rateHz samples already gives ~√N noise
	// reduction and the user is evaluating whether the EMA buys anything
	// more for steady-state heading display.
	magEMAOn      atomic.Bool
	displayMagEMA *vec3EMA
	magAcc        magAccumulator

	// alignAccelEMA/alignMagEMA feed only the Align snapshot — long tau,
	// always on, so the user gets a clean (accel, mag) capture independent
	// of the UI display filter.
	alignAccelEMA *vec3EMA
	alignMagEMA   *vec3EMA
	smoothAccel   atomic.Pointer[vec3]
	smoothMag     atomic.Pointer[vec3]
}

type alignState struct {
	R               mat3
	AlignHeadingDeg float64
	SavedAt         time.Time
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
		bc:            newBroadcaster(),
		displayMagEMA: newVec3EMA(dt, *magTau),
		alignAccelEMA: newVec3EMA(dt, *alignTau),
		alignMagEMA:   newVec3EMA(dt, *alignTau),
	}
	log.Printf("compass: imu=%d Hz, emit=%d Hz, accel/gyro DLPF=%.1f/%.1f Hz, mag tau=%.2fs, align tau=%.2fs",
		*imuHz, *rateHz, *accelDLPF, *gyroDLPF, *magTau, *alignTau)

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
	var cfg *configFile
	if c, err := loadConfig(); err == nil && c != nil {
		cfg = c
		if c.N0 > 0 {
			fallbackN0 = c.N0
		}
	}
	rt.geomag = newGeomagState(fallbackN0)
	if cfg != nil && cfg.Lat != nil && cfg.Lon != nil {
		alt := 0.0
		if cfg.AltM != nil {
			alt = *cfg.AltM
		}
		if err := rt.geomag.seedFromLocation(*cfg.Lat, *cfg.Lon, alt); err != nil {
			log.Printf("compass: config WMM seed failed: %v", err)
		} else {
			log.Printf("compass: WMM seeded from config (%.4f, %.4f, %.1fm) until GPS fix", *cfg.Lat, *cfg.Lon, alt)
		}
	}
	rt.gps = &gpsSource{}

	if al, err := loadAlign(); err == nil && al != nil {
		if isValidRot(al.R) {
			st := &alignState{R: al.R, AlignHeadingDeg: al.AlignHeadingDeg, SavedAt: al.SavedAt}
			rt.align.Store(st)
			log.Printf("compass: loaded align.json: heading=%.2f° saved=%s",
				al.AlignHeadingDeg, al.SavedAt.Format(time.RFC3339))
		} else {
			log.Printf("compass: align.json present but R is not a valid rotation; starting unaligned")
		}
	} else {
		log.Printf("compass: no align.json (compass unaligned; click Align to capture)")
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

	// Boxcar-mean every per-sample calibrated mag arrived since the last
	// emit. Accel comes through the on-chip DLPF so we just take the
	// latest sample. UI/CSV still show the raw single-sample magCal.
	magBoxcar, _, ok := rt.magAcc.drain()
	if !ok {
		magBoxcar = magCal
	}

	// Always update the align-purpose long-tau EMAs so Align has a stable
	// capture regardless of the display EMA toggle.
	sAccel := rt.alignAccelEMA.update(accel)
	sMag := rt.alignMagEMA.update(magBoxcar)
	rt.smoothAccel.Store(&sAccel)
	rt.smoothMag.Store(&sMag)

	// Display path: boxcar by default; optional short-tau EMA on top.
	emaOn := rt.magEMAOn.Load()
	displayMag := magBoxcar
	if emaOn {
		displayMag = rt.displayMagEMA.update(magBoxcar)
	}

	// Sensor-frame heading is a debug-only readout: what the compass
	// would say if sensor-x were vehicle-forward and the sensor were
	// level. Always defined.
	headingSensor := wrapPi(math.Atan2(-displayMag.Y, displayMag.X))
	headingSensorDeg := headingSensor * 180 / math.Pi

	// Vehicle heading requires an alignment. Without it, the UI/CSV
	// show "—" / NaN.
	align := rt.align.Load()
	var headingVehDeg float64
	headingVehOK := false
	if align != nil {
		magVeh := applyRot(align.R, displayMag)
		headingVehDeg = HeadingFromAligned(magVeh) * 180 / math.Pi
		headingVehOK = true
	}

	gpsFix := rt.gps.latest()
	n0Ut, declDeg, inclDeg, fUt, hUt, xUt, yUt, zUt, fallback := rt.geomag.getAll()
	trackTrueDeg := gpsFix.TrackTrue
	trackMagDeg := math.NaN()
	if !math.IsNaN(trackTrueDeg) {
		trackMagDeg = wrapDeg(trackTrueDeg - declDeg)
	}

	var magPred vec3
	magPredOK := false
	if align != nil && !math.IsNaN(trackMagDeg) {
		magPred, magPredOK = PredictRawMag(trackMagDeg*math.Pi/180, n0Ut, inclDeg, align.R, rt.k, rt.l)
	}
	if !magPredOK {
		magPred = vec3{math.NaN(), math.NaN(), math.NaN()}
	}

	alignHeading := math.NaN()
	if align != nil {
		alignHeading = align.AlignHeadingDeg
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
		MagEMA:           emaOn,
		TempC:            smp.TempC,
		GPS:              gpsFix,
		TrackMagDeg:      trackMagDeg,
		N0Ut:             n0Ut,
		DeclDeg:          declDeg,
		InclDeg:          inclDeg,
		AlignHeadingDeg:  alignHeading,
		HeadingSensorDeg: headingSensorDeg,
		HeadingVehDeg:    math.NaN(),
	}
	if headingVehOK {
		row.HeadingVehDeg = headingVehDeg
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
	alignPL := alignPayloadFrom(align)

	measured := measureGeomag(magCal, accel, headingVehOK, headingVehDeg, trackTrueDeg)

	out := messageOut{
		IMU:              &imu,
		GPS:              &gps,
		Geomag:           &gm,
		GeomagMeasured:   &measured,
		Align:            &alignPL,
		HeadingSensorDeg: &headingSensorDeg,
	}
	if headingVehOK {
		out.HeadingVehDeg = &headingVehDeg
	}
	if magPredOK {
		out.Predicted = &predictedPayload{MagPred: magPred}
	}
	rt.bc.publish(out)
}

// measureGeomag derives the geomag quantities computable from the current
// calibrated mag (and where possible accel + a true-heading reference) for
// side-by-side comparison with the WMM model.
//   F is just |magCal| and is always defined.
//   H, ZDown, InclDeg need gravity to split horizontal/vertical — they're
//     omitted on a zero-accel reading.
//   DeclDeg, X, Y need a true-heading reference (GPS trackTrue + a vehicle
//     heading from the alignment); omitted otherwise. Note that when the
//     alignment was captured from GPS track, DeclDeg ≈ model decl by
//     construction (R was built using model decl); the comparison is only
//     informative if the alignment was captured from a manual heading.
func measureGeomag(magCal, accel vec3, haveVehHeading bool, vehHeadingMagDeg, trackTrueDeg float64) geomagMeasuredPayload {
	out := geomagMeasuredPayload{F: math.Sqrt(dot(magCal, magCal))}
	dHat, ok := normalize(vec3{-accel.X, -accel.Y, -accel.Z})
	if !ok {
		return out
	}
	zDown := dot(magCal, dHat)
	horiz := vec3{magCal.X - zDown*dHat.X, magCal.Y - zDown*dHat.Y, magCal.Z - zDown*dHat.Z}
	h := math.Sqrt(dot(horiz, horiz))
	incl := math.Atan2(zDown, h) * 180 / math.Pi
	out.H = &h
	out.ZDown = &zDown
	out.InclDeg = &incl
	if haveVehHeading && !math.IsNaN(trackTrueDeg) {
		d := wrapDeg(trackTrueDeg - vehHeadingMagDeg)
		dRad := d * math.Pi / 180
		x := h * math.Cos(dRad)
		y := h * math.Sin(dRad)
		out.DeclDeg = &d
		out.X = &x
		out.Y = &y
	}
	return out
}

func alignPayloadFrom(a *alignState) alignPayload {
	if a == nil {
		return alignPayload{Active: false}
	}
	return alignPayload{Active: true, AlignHeadingDeg: a.AlignHeadingDeg, SavedAt: a.SavedAt}
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

	// Initial snapshot: calibration + align state + EMA toggle.
	align := rt.align.Load()
	alignPL := alignPayloadFrom(align)
	ema := rt.magEMAOn.Load()
	if err := conn.WriteJSON(messageOut{
		Cal:    &calPayload{K: rt.k, L: rt.l},
		Align:  &alignPL,
		MagEMA: &ema,
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
	if msg.MagEMA != nil {
		rt.magEMAOn.Store(*msg.MagEMA)
		v := *msg.MagEMA
		_ = conn.WriteJSON(messageOut{MagEMA: &v})
	}
	if msg.Action == "align" {
		if err := doAlign(rt, msg.ManualHeadingDeg); err != nil {
			_ = conn.WriteJSON(messageOut{Error: err.Error()})
			return
		}
		alignPL := alignPayloadFrom(rt.align.Load())
		_ = conn.WriteJSON(messageOut{Align: &alignPL})
	}
}

// doAlign builds the sensor→vehicle rotation from the current smoothed
// (accel, mag) snapshot and the heading the user (or the GPS track)
// supplies as truth. manualHeadingMagDeg, if non-nil, is the vehicle's
// magnetic heading (matches the convention used everywhere else on the
// dial — the GPS-track path converts trackTrue → trackMag with the model
// declination, and the manual-input path skips that conversion since the
// user types magnetic directly).
func doAlign(rt *runtime, manualHeadingMagDeg *float64) error {
	if rt.imuSample.Load() == nil {
		return fmt.Errorf("no IMU sample yet")
	}
	accelP := rt.smoothAccel.Load()
	magP := rt.smoothMag.Load()
	if accelP == nil || magP == nil {
		return fmt.Errorf("filter not warmed up yet — wait ~3s after start")
	}

	var headingMagDeg float64
	if manualHeadingMagDeg != nil {
		headingMagDeg = wrapDeg(*manualHeadingMagDeg)
	} else {
		_, declDeg, _, _ := rt.geomag.get()
		fix := rt.gps.latest()
		if !fix.Valid || math.IsNaN(fix.TrackTrue) {
			return fmt.Errorf("no GPS track; supply a manual heading or wait for a 2D+ fix")
		}
		headingMagDeg = wrapDeg(fix.TrackTrue - declDeg)
	}

	R, err := BuildAlignRotation(*accelP, *magP, headingMagDeg*math.Pi/180)
	if err != nil {
		return fmt.Errorf("build alignment: %w", err)
	}
	now := time.Now()
	st := &alignState{R: R, AlignHeadingDeg: headingMagDeg, SavedAt: now}
	if err := saveAlign(&alignFile{R: R, AlignHeadingDeg: headingMagDeg, SavedAt: now}); err != nil {
		return fmt.Errorf("save align.json: %w", err)
	}
	rt.align.Store(st)
	log.Printf("compass: align: heading_mag=%.2f° (R captured)", headingMagDeg)
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
