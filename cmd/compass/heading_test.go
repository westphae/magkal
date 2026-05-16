package main

import (
	"math"
	"strconv"
	"testing"
)

const tol = 1e-6

func approx(a, b float64) bool { return math.Abs(a-b) < tol }

func degToRad(d float64) float64 { return d * math.Pi / 180 }

// TestBuildAlignRotationLevelCardinals verifies that at the moment of Align,
// applying R to the captured mag reproduces the user-supplied heading. This
// is the construction guarantee; multiple H_target values exercise each
// quadrant for sign-error catches.
func TestBuildAlignRotationLevelCardinals(t *testing.T) {
	const n0 = 50.0
	const inclDeg = 60.0
	const incl = inclDeg * math.Pi / 180
	accelLevel := vec3{0, 0, 9.81} // z-up
	// At heading 0 (sensor-x = magnetic north), z-up: mag.x=cos(I), mag.z=-sin(I)*n0
	magNorth := vec3{n0 * math.Cos(incl), 0, -n0 * math.Sin(incl)}
	for _, hDeg := range []float64{0, 45, 90, 179, -90, -179} {
		t.Run("h="+formatDeg(hDeg), func(t *testing.T) {
			h := degToRad(hDeg)
			R, err := BuildAlignRotation(accelLevel, magNorth, h)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			got := HeadingFromAligned(applyRot(R, magNorth))
			if !approx(got, wrapPi(h)) {
				t.Errorf("got %.6f want %.6f", got, h)
			}
		})
	}
}

// TestBuildAlignRotationPitched verifies the align math handles a tilted
// mount: a 20° pitch-up captured at heading 0 still reads heading 0.
func TestBuildAlignRotationPitched(t *testing.T) {
	pitch := degToRad(20)
	const g = 9.81
	accel := vec3{-g * math.Sin(pitch), 0, g * math.Cos(pitch)}
	const n0 = 50.0
	const incl = 60.0 * math.Pi / 180
	cosI := math.Cos(incl)
	sinI := math.Sin(incl)
	mag := vec3{
		n0*cosI*math.Cos(pitch) + n0*sinI*math.Sin(pitch),
		0,
		-n0*cosI*math.Sin(pitch) + n0*sinI*math.Cos(pitch),
	}
	R, err := BuildAlignRotation(accel, mag, 0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got := HeadingFromAligned(applyRot(R, mag))
	if !approx(got, 0) {
		t.Errorf("pitched-north heading got %.6f want 0", got)
	}
}

// TestAlignmentTracksSensorYaw verifies the substantive claim: after Align,
// physically yawing the sensor by Δh in the world produces a heading reading
// of H_target + Δh. Constructs the new sensor-frame mag from world geometry
// rather than from R^T (which would be a tautology).
func TestAlignmentTracksSensorYaw(t *testing.T) {
	const n0 = 50.0
	const incl = 60.0 * math.Pi / 180
	cosI := math.Cos(incl)
	sinI := math.Sin(incl)
	accelLevel := vec3{0, 0, 9.81}
	// Align at heading 0, level z-up mount. Sensor-x = world-N at align.
	magAtAlign := vec3{n0 * cosI, 0, -n0 * sinI}
	R, err := BuildAlignRotation(accelLevel, magAtAlign, 0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// Yaw sensor +90° (heading east, level). Sensor-x now = world-E,
	// sensor-y = world-N (z-up right-handed). World mag in z-up:
	// (n0 cosI, 0, -n0 sinI). In new sensor coords:
	// (mag·sensor_x, mag·sensor_y, mag·sensor_z) = (0, n0 cosI, -n0 sinI).
	magYawedEast := vec3{0, n0 * cosI, -n0 * sinI}
	got := HeadingFromAligned(applyRot(R, magYawedEast))
	if !approx(got, degToRad(90)) {
		t.Errorf("yawed east got %.6f want %.6f", got, degToRad(90))
	}
}

// TestPredictRawMagRoundTripsThroughCalibration verifies the inverse: predict
// a raw m, apply the forward calibration k*(m-l), and confirm the result is
// the vehicle-frame mag we'd have rotated in. Catches sign errors in the
// rotate-back direction.
func TestPredictRawMagRoundTripsThroughCalibration(t *testing.T) {
	const n0 = 50.0
	const inclDeg = 60.0
	const incl = inclDeg * math.Pi / 180
	cosI := math.Cos(incl)
	sinI := math.Sin(incl)
	accelLevel := vec3{0, 0, 9.81}
	magNorth := vec3{n0 * cosI, 0, -n0 * sinI}
	// Align at heading 30° so R is non-trivial.
	R, err := BuildAlignRotation(accelLevel, magNorth, degToRad(30))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	k := []float64{0.8, 0.75, 0.68}
	l := []float64{-49.0, 3.7, -32.0}
	for _, hDeg := range []float64{0, 45, 90, 180, 270} {
		t.Run("h="+formatDeg(hDeg), func(t *testing.T) {
			h := degToRad(hDeg)
			mV := vec3{n0 * cosI * math.Cos(h), -n0 * cosI * math.Sin(h), n0 * sinI}
			mS := applyRotT(R, mV)
			rawWant := vec3{mS.X/k[0] + l[0], mS.Y/k[1] + l[1], mS.Z/k[2] + l[2]}
			rawGot, ok := PredictRawMag(h, n0, inclDeg, R, k, l)
			if !ok {
				t.Fatalf("predict not ok")
			}
			if !approx(rawGot.X, rawWant.X) || !approx(rawGot.Y, rawWant.Y) || !approx(rawGot.Z, rawWant.Z) {
				t.Errorf("got %+v want %+v", rawGot, rawWant)
			}
		})
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

func TestIsValidRot(t *testing.T) {
	if isValidRot(mat3{}) {
		t.Errorf("zero matrix should not be valid")
	}
	id := mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	if !isValidRot(id) {
		t.Errorf("identity should be valid")
	}
}

func formatDeg(d float64) string {
	return strconv.FormatFloat(d, 'f', 2, 64)
}
