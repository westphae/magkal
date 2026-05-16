package main

import "time"

// Wire protocol for the compass websocket. Each messageIn/messageOut may
// contain any subset of the optional fields below; recipients ignore the
// fields they don't recognize.

type messageIn struct {
	Action           string   `json:"action,omitempty"`           // "align"
	MagEMA           *bool    `json:"magEma,omitempty"`           // toggle display-path EMA
	ManualHeadingDeg *float64 `json:"manualHeadingDeg,omitempty"` // align target (deg *magnetic*); nil = use GPS track (which is true-deg, server subtracts declination)
}

type vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type imuPayload struct {
	T      time.Time `json:"t"`
	Accel  vec3      `json:"accel"`  // m/s²
	Gyro   vec3      `json:"gyro"`   // rad/s
	MagRaw vec3      `json:"magRaw"` // µT
	MagCal vec3      `json:"magCal"` // µT, k*(m - l)
	TempC  float64   `json:"tempC"`
}

type gpsPayload struct {
	T            time.Time `json:"t"`
	Lat          *float64  `json:"lat"`
	Lon          *float64  `json:"lon"`
	AltM         *float64  `json:"altM"`
	TrackTrueDeg *float64  `json:"trackTrueDeg"`
	TrackMagDeg  *float64  `json:"trackMagDeg"`
	SpeedMps     *float64  `json:"speedMps"`
	Mode         int       `json:"mode"` // 0 unk, 1 no-fix, 2 2D, 3 3D
}

type geomagPayload struct {
	N0Ut     float64 `json:"n0Ut"`     // µT, total field (same as F)
	DeclDeg  float64 `json:"declDeg"`  // degrees east-positive
	InclDeg  float64 `json:"inclDeg"`  // degrees down-positive
	FUt      float64 `json:"fUt"`      // µT, total field strength
	HUt      float64 `json:"hUt"`      // µT, horizontal component
	XUt      float64 `json:"xUt"`      // µT, ellipsoidal-north
	YUt      float64 `json:"yUt"`      // µT, ellipsoidal-east
	ZUt      float64 `json:"zUt"`      // µT, ellipsoidal-down
	Fallback bool    `json:"fallback"` // true if from config.json (no GPS yet)
}

type predictedPayload struct {
	MagPred vec3 `json:"magPred"` // µT raw expected at GPS track heading
}

// geomagMeasuredPayload reports the geomag quantities computable directly
// from the calibrated mag and (where needed) accel/GPS. F is always set;
// H/ZDown/InclDeg require accel; DeclDeg/X/Y require GPS track + alignment
// so the magnetic-vs-true heading delta is observable. Nil pointers mean
// "not computable at this tick".
type geomagMeasuredPayload struct {
	F       float64  `json:"f"`              // µT, |magCal|
	H       *float64 `json:"h,omitempty"`    // µT, horizontal magnitude (accel-required)
	ZDown   *float64 `json:"zDown,omitempty"` // µT, vertical, positive down
	InclDeg *float64 `json:"inclDeg,omitempty"`
	DeclDeg *float64 `json:"declDeg,omitempty"`
	X       *float64 `json:"x,omitempty"` // µT, geographic-north
	Y       *float64 `json:"y,omitempty"` // µT, geographic-east
}

// alignPayload reports the current alignment state. Active=false before any
// alignment has been captured (no heading is shown in the UI in that state).
// AlignHeadingDeg is the user-supplied (or GPS-track-derived) magnetic
// heading at the moment Align was clicked.
type alignPayload struct {
	Active          bool      `json:"active"`
	AlignHeadingDeg float64   `json:"alignHeadingDeg"`
	SavedAt         time.Time `json:"savedAt"`
}

type calPayload struct {
	K []float64 `json:"k"`
	L []float64 `json:"l"`
}

type messageOut struct {
	IMU              *imuPayload       `json:"imu,omitempty"`
	GPS              *gpsPayload       `json:"gps,omitempty"`
	HeadingSensorDeg *float64          `json:"headingSensorDeg,omitempty"`
	HeadingVehDeg    *float64          `json:"headingVehDeg,omitempty"`
	Predicted        *predictedPayload `json:"predicted,omitempty"`
	Geomag           *geomagPayload    `json:"geomag,omitempty"`
	GeomagMeasured   *geomagMeasuredPayload `json:"geomagMeasured,omitempty"`
	Align            *alignPayload     `json:"align,omitempty"`
	Cal              *calPayload       `json:"cal,omitempty"`
	MagEMA           *bool             `json:"magEma,omitempty"`
	Error            string            `json:"error,omitempty"`
}
