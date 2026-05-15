package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// recordRow is everything we know at one 10 Hz tick. NaN fields are written
// as the literal "NaN" by csv (Go's strconv.FormatFloat on NaN); downstream
// tools (pandas et al.) handle that correctly.
type recordRow struct {
	TWall          time.Time
	TIMU           time.Time
	Accel          vec3
	Gyro           vec3
	MagRaw         vec3
	MagCal         vec3
	MagPred        vec3
	TiltComp       bool
	TempC          float64
	GPS            gpsFix
	TrackMagDeg    float64
	N0Ut           float64
	DeclDeg        float64
	InclDeg        float64
	YawOffsetDeg   float64
	HeadingSensorDeg float64
	HeadingVehDeg  float64
}

// recorder owns the CSV file for one compass session. Tickers flush every
// second; explicit Close performs a final flush.
type recorder struct {
	mu    sync.Mutex
	f     *os.File
	w     *csv.Writer
	path  string
	closed bool
}

func newRecorder(now time.Time) (*recorder, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".local", "share", "magkal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := now.Format("20060102-150405") + ".csv"
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := csv.NewWriter(f)
	if err := w.Write(csvHeader); err != nil {
		f.Close()
		return nil, err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return nil, err
	}
	return &recorder{f: f, w: w, path: path}, nil
}

func (r *recorder) Path() string { return r.path }

func (r *recorder) Write(row recordRow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	return r.w.Write(rowToFields(row))
}

func (r *recorder) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.w.Flush()
	if err := r.w.Error(); err != nil {
		return err
	}
	return r.f.Sync()
}

func (r *recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	r.w.Flush()
	if err := r.w.Error(); err != nil {
		r.f.Close()
		return err
	}
	if err := r.f.Sync(); err != nil {
		r.f.Close()
		return err
	}
	return r.f.Close()
}

// runFlusher fires r.Flush() every 1 s until ctx is done.
func runFlusher(ctx context.Context, r *recorder) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if err := r.Flush(); err != nil {
				fmt.Fprintf(os.Stderr, "compass: recorder flush: %v\n", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

var csvHeader = []string{
	"t_wall", "t_imu",
	"ax", "ay", "az",
	"gx", "gy", "gz",
	"mx_raw", "my_raw", "mz_raw",
	"mx_cal", "my_cal", "mz_cal",
	"mx_pred", "my_pred", "mz_pred",
	"tilt_comp",
	"temp_c",
	"gps_t",
	"gps_lat", "gps_lon", "gps_alt_msl_m", "gps_alt_ellip_m",
	"gps_track_true_deg", "gps_track_mag_deg",
	"gps_speed_mps", "gps_mode",
	"n0_ut", "decl_deg", "incl_deg",
	"yaw_offset_deg",
	"heading_sensor_deg", "heading_veh_deg",
}

func ff(v float64) string {
	if math.IsNaN(v) {
		return "NaN"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func bb(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func rowToFields(r recordRow) []string {
	return []string{
		r.TWall.Format(time.RFC3339Nano),
		r.TIMU.Format(time.RFC3339Nano),
		ff(r.Accel.X), ff(r.Accel.Y), ff(r.Accel.Z),
		ff(r.Gyro.X), ff(r.Gyro.Y), ff(r.Gyro.Z),
		ff(r.MagRaw.X), ff(r.MagRaw.Y), ff(r.MagRaw.Z),
		ff(r.MagCal.X), ff(r.MagCal.Y), ff(r.MagCal.Z),
		ff(r.MagPred.X), ff(r.MagPred.Y), ff(r.MagPred.Z),
		bb(r.TiltComp),
		ff(r.TempC),
		gpsTimeStr(r.GPS.T),
		ff(r.GPS.Lat), ff(r.GPS.Lon), ff(r.GPS.AltMSL), ff(r.GPS.AltEllip),
		ff(r.GPS.TrackTrue), ff(r.TrackMagDeg),
		ff(r.GPS.SpeedMps), strconv.Itoa(r.GPS.Mode),
		ff(r.N0Ut), ff(r.DeclDeg), ff(r.InclDeg),
		ff(r.YawOffsetDeg),
		ff(r.HeadingSensorDeg), ff(r.HeadingVehDeg),
	}
}

func gpsTimeStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}
