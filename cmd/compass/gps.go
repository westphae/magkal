package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"math"
	"net"
	"sync"
	"time"
)

// gpsdAddr is the standard gpsd JSON socket. Hard-coded — gpsd uses this
// port everywhere by default, and exposing a flag would be premature.
const gpsdAddr = "127.0.0.1:2947"

// gpsFix is the subset of a gpsd TPV ("time-position-velocity") record we
// care about. NaN fields signal "not provided in this report".
type gpsFix struct {
	T          time.Time
	Lat        float64
	Lon        float64
	AltMSL     float64 // meters above mean sea level (gpsd "altMSL"/"alt")
	AltEllip   float64 // meters above WGS-84 ellipsoid (gpsd "altHAE")
	TrackTrue  float64 // deg (true north)
	SpeedMps   float64
	Mode       int // 0 unknown, 1 no fix, 2 2D, 3 3D
	Valid      bool
}

// gpsSource holds the latest fix received from gpsd. Reads are lock-free
// via an atomic pointer; updates take a lock to coalesce concurrent writes.
type gpsSource struct {
	mu  sync.RWMutex
	cur gpsFix
}

func (g *gpsSource) latest() gpsFix {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.cur
}

func (g *gpsSource) set(f gpsFix) {
	g.mu.Lock()
	g.cur = f
	g.mu.Unlock()
}

// runGPS connects to gpsd, sends WATCH, and pushes every TPV into src.
// Reconnects on failure with a 2 s backoff. Returns when ctx is done.
func runGPS(ctx context.Context, src *gpsSource) {
	for {
		if err := readOnce(ctx, src); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("compass: gpsd: %v; retrying in 2s", err)
		}
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return
		}
	}
}

func readOnce(ctx context.Context, src *gpsSource) error {
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", gpsdAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(`?WATCH={"enable":true,"json":true};` + "\n")); err != nil {
		return err
	}

	// Goroutine to close the conn on context cancellation so that the
	// scanner's Read unblocks.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		// Cheap class filter before json.Unmarshal — most reports are TPV.
		if !bytesContains(line, []byte(`"class":"TPV"`)) {
			continue
		}
		var r struct {
			Time    time.Time `json:"time"`
			Lat     *float64  `json:"lat"`
			Lon     *float64  `json:"lon"`
			AltHAE  *float64  `json:"altHAE"`
			AltMSL  *float64  `json:"altMSL"`
			Alt     *float64  `json:"alt"` // legacy gpsd <3.20
			Track   *float64  `json:"track"`
			Speed   *float64  `json:"speed"`
			Mode    int       `json:"mode"`
		}
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		f := gpsFix{
			T:         r.Time,
			Mode:      r.Mode,
			Valid:     r.Mode >= 2 && r.Lat != nil && r.Lon != nil,
			Lat:       optF(r.Lat),
			Lon:       optF(r.Lon),
			AltEllip:  optF(r.AltHAE),
			AltMSL:    firstNonNil(r.AltMSL, r.Alt),
			TrackTrue: optF(r.Track),
			SpeedMps:  optF(r.Speed),
		}
		src.set(f)
	}
	return sc.Err()
}

func optF(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}

func firstNonNil(a, b *float64) float64 {
	if a != nil {
		return *a
	}
	if b != nil {
		return *b
	}
	return math.NaN()
}

func bytesContains(haystack, needle []byte) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		eq := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				eq = false
				break
			}
		}
		if eq {
			return true
		}
	}
	return false
}
