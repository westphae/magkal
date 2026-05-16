package main

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"github.com/westphae/geomag/pkg/egm96"
	"github.com/westphae/geomag/pkg/wmm"
)

// geomagState is the latest n0/declination/inclination, computed from the
// most recent GPS fix via the WMM model. n0 is in µT (geomag returns nT,
// converted at the boundary to match the rest of magkal). Fallback=true
// means "no GPS fix yet; using config.json n0 with declination=inclination=0".
type geomagState struct {
	mu       sync.RWMutex
	n0Ut     float64
	declDeg  float64
	inclDeg  float64
	fUt      float64
	hUt      float64
	xUt      float64
	yUt      float64
	zUt      float64
	fallback bool
}

func newGeomagState(fallbackN0 float64) *geomagState {
	return &geomagState{n0Ut: fallbackN0, fUt: fallbackN0, fallback: true}
}

// seedFromLocation runs WMM at a user-supplied (lat, lon, altMEllipsoid) so
// the UI shows a full geomag model before GPS has a fix. The fallback flag
// stays true — a real GPS fix supersedes the config-supplied seed.
func (g *geomagState) seedFromLocation(lat, lon, altM float64) error {
	loc := egm96.NewLocationGeodetic(lat, lon, altM)
	mf, err := wmm.CalculateWMMMagneticField(loc, time.Now())
	if err != nil {
		return err
	}
	xNT, yNT, zNT, _, _, _ := mf.Ellipsoidal()
	const ntToUt = 1.0 / 1000.0
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n0Ut = mf.F() * ntToUt
	g.declDeg = mf.D()
	g.inclDeg = mf.I()
	g.fUt = mf.F() * ntToUt
	g.hUt = mf.H() * ntToUt
	g.xUt = xNT * ntToUt
	g.yUt = yNT * ntToUt
	g.zUt = zNT * ntToUt
	// fallback stays true; a real GPS fix is still expected to replace this.
	return nil
}

func (g *geomagState) get() (n0Ut, declDeg, inclDeg float64, fallback bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.n0Ut, g.declDeg, g.inclDeg, g.fallback
}

func (g *geomagState) getAll() (n0Ut, declDeg, inclDeg, fUt, hUt, xUt, yUt, zUt float64, fallback bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.n0Ut, g.declDeg, g.inclDeg, g.fUt, g.hUt, g.xUt, g.yUt, g.zUt, g.fallback
}

func (g *geomagState) set(n0Ut, declDeg, inclDeg, fUt, hUt, xUt, yUt, zUt float64) {
	g.mu.Lock()
	g.n0Ut, g.declDeg, g.inclDeg = n0Ut, declDeg, inclDeg
	g.fUt, g.hUt, g.xUt, g.yUt, g.zUt = fUt, hUt, xUt, yUt, zUt
	g.fallback = false
	g.mu.Unlock()
}

// runGeomag refreshes (n0, declination, inclination) from WMM every 60s,
// using the latest GPS fix. First fix triggers an immediate refresh. Holds
// the previous values silently when GPS drops out (no "fallback" flip-back
// after we've had a fix).
func runGeomag(ctx context.Context, src *gpsSource, state *geomagState) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	var haveFix bool
	for {
		if !haveFix {
			// Poll every 500ms until we get a fix, then switch to the 60s cadence.
			if f := src.latest(); f.Valid {
				refresh(f, state)
				haveFix = true
			} else {
				select {
				case <-time.After(500 * time.Millisecond):
					continue
				case <-ctx.Done():
					return
				}
			}
		}
		select {
		case <-ticker.C:
			if f := src.latest(); f.Valid {
				refresh(f, state)
			}
		case <-ctx.Done():
			return
		}
	}
}

func refresh(f gpsFix, state *geomagState) {
	alt := f.AltEllip
	if math.IsNaN(alt) {
		alt = f.AltMSL // close enough for n0 purposes (WMM varies <1 nT over the 50m geoid range)
	}
	if math.IsNaN(alt) {
		alt = 0
	}
	loc := egm96.NewLocationGeodetic(f.Lat, f.Lon, alt)
	mf, err := wmm.CalculateWMMMagneticField(loc, time.Now())
	if err != nil {
		log.Printf("compass: wmm: %v", err)
		return
	}
	xNT, yNT, zNT, _, _, _ := mf.Ellipsoidal()
	const ntToUt = 1.0 / 1000.0
	state.set(
		mf.F()*ntToUt, mf.D(), mf.I(),
		mf.F()*ntToUt, mf.H()*ntToUt,
		xNT*ntToUt, yNT*ntToUt, zNT*ntToUt,
	)
}
