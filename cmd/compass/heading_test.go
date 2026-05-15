package main

import (
	"math"
	"testing"
)

const tol = 1e-6

func approx(a, b float64) bool { return math.Abs(a-b) < tol }

func degToRad(d float64) float64 { return d * math.Pi / 180 }

// z-up mounting helper: given a compass heading β (rad) for sensor-x and an
// inclination (rad), return the calibrated mag vector in sensor coords. Uses
// the world coordinate construction: sensor-x at compass β, sensor-z = up,
// so sensor-y at compass β-90°. The world field has N component cos(incl)*n0,
// D component sin(incl)*n0; world Up = -world D.
func magCalZUp(beta, incl, n0 float64) vec3 {
	// sensor-x = cos(β)N + sin(β)E (horizontal)
	// sensor-y = cos(β-90°)N + sin(β-90°)E = sin(β)N - cos(β)E
	// World mag = n0*cos(incl) N + n0*sin(incl) D = n0*cos(incl) N + 0 E + n0*sin(incl) D
	// In sensor coords: mag.x = N*cos(β) + E*sin(β) = n0*cos(incl)*cos(β)
	//                   mag.y = N*sin(β) - E*cos(β) = n0*cos(incl)*sin(β)
	//                   mag.z = -D = -n0*sin(incl)   (sensor-z = up = -world-D)
	return vec3{
		n0 * math.Cos(incl) * math.Cos(beta),
		n0 * math.Cos(incl) * math.Sin(beta),
		-n0 * math.Sin(incl),
	}
}

func TestHeadingSensorTiltOnLevelCardinals(t *testing.T) {
	const n0 = 50.0
	const inclDeg = 60.0
	const incl = inclDeg * math.Pi / 180
	accelLevel := vec3{0, 0, 9.81} // z-up

	cases := []struct {
		name  string
		beta  float64
		wantH float64
	}{
		{"north", 0, 0},
		{"east", degToRad(90), degToRad(90)},
		{"south_pos", degToRad(179), degToRad(179)},
		{"south_neg", -degToRad(179), -degToRad(179)},
		{"west", -degToRad(90), -degToRad(90)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mag := magCalZUp(c.beta, incl, n0)
			got, ok := HeadingSensorTiltOn(accelLevel, mag)
			if !ok {
				t.Fatalf("not ok")
			}
			if !approx(got, c.wantH) {
				t.Errorf("got %.6f want %.6f", got, c.wantH)
			}
		})
	}
}

func TestHeadingSensorTiltOnPitched(t *testing.T) {
	// 20° pitch up about sensor-y: sensor-x rotates toward sensor +z (up).
	// Gravity in sensor coords goes from (0,0,-g) to (g*sin(20°), 0, -g*cos(20°)).
	// accel = -gravity = (-g*sin(20°), 0, g*cos(20°))
	pitch := degToRad(20)
	const g = 9.81
	accel := vec3{-g * math.Sin(pitch), 0, g * math.Cos(pitch)}

	// World mag at heading north (β=0), incl=60°. World vec = n0*(cos(60), 0, sin(60))
	// in NED. World-up = -D. After pitching sensor by 20° about sensor-y (which is
	// world east), sensor-x = world-N rotated up by pitch; world-N in sensor coords
	// after this rotation = (cos(pitch), 0, -sin(pitch)). World-D in sensor coords =
	// (sin(pitch), 0, cos(pitch)).
	const n0 = 50.0
	const inclDeg = 60.0
	const incl = inclDeg * math.Pi / 180
	cosI := math.Cos(incl)
	sinI := math.Sin(incl)
	mag := vec3{
		n0*cosI*math.Cos(pitch) + n0*sinI*math.Sin(pitch),
		0,
		-n0*cosI*math.Sin(pitch) + n0*sinI*math.Cos(pitch),
	}
	got, ok := HeadingSensorTiltOn(accel, mag)
	if !ok {
		t.Fatalf("not ok")
	}
	if !approx(got, 0) {
		t.Errorf("pitched-north heading got %.6f want 0", got)
	}
}

func TestHeadingSensorTiltOffLevel(t *testing.T) {
	const n0 = 50.0
	const incl = 60.0 * math.Pi / 180
	cases := []struct {
		name  string
		beta  float64
		wantH float64
	}{
		{"north", 0, 0},
		{"east", degToRad(90), degToRad(90)},
		{"west", -degToRad(90), -degToRad(90)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mag := magCalZUp(c.beta, incl, n0)
			got := HeadingSensorTiltOff(mag)
			if !approx(got, c.wantH) {
				t.Errorf("got %.6f want %.6f", got, c.wantH)
			}
		})
	}
}

func TestPredictRawMagRoundTripsThroughCalibration(t *testing.T) {
	// If raw m is calibrated to n, then predicting raw at the same heading as
	// the calibrated-mag observation should reproduce m. Tests the inverse
	// consistency: predictor(forward(m)) == m.
	const n0 = 50.0
	const inclDeg = 60.0
	const incl = inclDeg * math.Pi / 180
	beta := degToRad(45) // sensor-x at NE
	accel := vec3{0, 0, 9.81}
	yawOffset := degToRad(10)
	trackMag := beta + yawOffset // so heading_vehicle = trackMag, heading_sensor = β
	k := []float64{0.8, 0.75, 0.68}
	l := []float64{-49.0, 3.7, -32.0}

	magCal := magCalZUp(beta, incl, n0)
	// raw m such that k*(m-l) = magCal → m = magCal/k + l
	raw := vec3{magCal.X/k[0] + l[0], magCal.Y/k[1] + l[1], magCal.Z/k[2] + l[2]}

	pred, ok := PredictRawMag(trackMag, n0, inclDeg, yawOffset, accel, k, l)
	if !ok {
		t.Fatalf("not ok")
	}
	if !approx(pred.X, raw.X) || !approx(pred.Y, raw.Y) || !approx(pred.Z, raw.Z) {
		t.Errorf("predictRaw got %+v want %+v", pred, raw)
	}
}

func TestApplyCal(t *testing.T) {
	k := []float64{0.8, 0.75, 0.68}
	l := []float64{-49.0, 3.7, -32.0}
	raw := vec3{1.0, 2.0, 3.0}
	got := ApplyCal(raw, k, l)
	want := vec3{
		0.8 * (1.0 - -49.0),
		0.75 * (2.0 - 3.7),
		0.68 * (3.0 - -32.0),
	}
	if !approx(got.X, want.X) || !approx(got.Y, want.Y) || !approx(got.Z, want.Z) {
		t.Errorf("ApplyCal got %+v want %+v", got, want)
	}
}
