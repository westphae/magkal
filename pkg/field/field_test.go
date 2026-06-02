package field

import (
	"math"
	"strconv"
	"testing"
)

const tol = 1e-6

func approx(a, b float64) bool { return math.Abs(a-b) < tol }

func degToRad(d float64) float64 { return d * math.Pi / 180 }

func TestBuildAlignRotationLevelCardinals(t *testing.T) {
	const n0 = 50.0
	const incl = 60.0 * math.Pi / 180
	accelLevel := Vec3{0, 0, 9.81}
	magNorth := Vec3{n0 * math.Cos(incl), 0, -n0 * math.Sin(incl)}
	for _, hDeg := range []float64{0, 45, 90, 179, -90, -179} {
		t.Run("h="+formatDeg(hDeg), func(t *testing.T) {
			h := degToRad(hDeg)
			R, err := BuildAlignRotation(accelLevel, magNorth, h)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			got := HeadingFromAligned(ApplyRot(R, magNorth))
			if !approx(got, WrapPi(h)) {
				t.Errorf("got %.6f want %.6f", got, h)
			}
		})
	}
}

func TestBuildAlignRotationPitched(t *testing.T) {
	pitch := degToRad(20)
	const g = 9.81
	accel := Vec3{-g * math.Sin(pitch), 0, g * math.Cos(pitch)}
	const n0 = 50.0
	const incl = 60.0 * math.Pi / 180
	cosI := math.Cos(incl)
	sinI := math.Sin(incl)
	mag := Vec3{
		n0*cosI*math.Cos(pitch) + n0*sinI*math.Sin(pitch),
		0,
		-n0*cosI*math.Sin(pitch) + n0*sinI*math.Cos(pitch),
	}
	R, err := BuildAlignRotation(accel, mag, 0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got := HeadingFromAligned(ApplyRot(R, mag))
	if !approx(got, 0) {
		t.Errorf("pitched-north heading got %.6f want 0", got)
	}
}

func TestAlignmentTracksSensorYaw(t *testing.T) {
	const n0 = 50.0
	const incl = 60.0 * math.Pi / 180
	cosI := math.Cos(incl)
	sinI := math.Sin(incl)
	accelLevel := Vec3{0, 0, 9.81}
	magAtAlign := Vec3{n0 * cosI, 0, -n0 * sinI}
	R, err := BuildAlignRotation(accelLevel, magAtAlign, 0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	magYawedEast := Vec3{0, n0 * cosI, -n0 * sinI}
	got := HeadingFromAligned(ApplyRot(R, magYawedEast))
	if !approx(got, degToRad(90)) {
		t.Errorf("yawed east got %.6f want %.6f", got, degToRad(90))
	}
}

func TestPredictRawMagRoundTripsThroughCalibration(t *testing.T) {
	const n0 = 50.0
	const inclDeg = 60.0
	const incl = inclDeg * math.Pi / 180
	cosI := math.Cos(incl)
	sinI := math.Sin(incl)
	accelLevel := Vec3{0, 0, 9.81}
	magNorth := Vec3{n0 * cosI, 0, -n0 * sinI}
	R, err := BuildAlignRotation(accelLevel, magNorth, degToRad(30))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	k := []float64{0.8, 0.75, 0.68}
	l := []float64{-49.0, 3.7, -32.0}
	for _, hDeg := range []float64{0, 45, 90, 180, 270} {
		t.Run("h="+formatDeg(hDeg), func(t *testing.T) {
			h := degToRad(hDeg)
			mV := Vec3{n0 * cosI * math.Cos(h), -n0 * cosI * math.Sin(h), n0 * sinI}
			mS := ApplyRotT(R, mV)
			rawWant := Vec3{mS.X/k[0] + l[0], mS.Y/k[1] + l[1], mS.Z/k[2] + l[2]}
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
	raw := Vec3{1.0, 2.0, 3.0}
	got := ApplyCal(raw, k, l)
	want := Vec3{
		0.8 * (1.0 - -49.0),
		0.75 * (2.0 - 3.7),
		0.68 * (3.0 - -32.0),
	}
	if !approx(got.X, want.X) || !approx(got.Y, want.Y) || !approx(got.Z, want.Z) {
		t.Errorf("ApplyCal got %+v want %+v", got, want)
	}
}

func TestHeadingDeg360(t *testing.T) {
	if got := HeadingDeg360(-135); got != 225 {
		t.Errorf("got %v want 225", got)
	}
	if got := HeadingDeg360(360); got != 0 {
		t.Errorf("got %v want 0", got)
	}
}

func TestBuildAlignRotationToField(t *testing.T) {
	const n0 = 50.0
	const incl = 60.0 * math.Pi / 180
	accel := Vec3{0, 0, -9.81}
	magNorth := Vec3{n0 * math.Cos(incl), 0, -n0 * math.Sin(incl)}
	Raccel, err := BuildAlignRotation(accel, magNorth, 0)
	if err != nil {
		t.Fatalf("accel build: %v", err)
	}
	bVehicle := ApplyRot(Raccel, magNorth)
	R, err := BuildAlignRotationToField(magNorth, bVehicle, 0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got := ApplyRot(R, magNorth)
	if !approx(got.X, bVehicle.X) || !approx(got.Y, bVehicle.Y) || !approx(got.Z, bVehicle.Z) {
		t.Errorf("aligned mag %+v want %+v", got, bVehicle)
	}
}

func TestBuildAlignRotationFromGeo(t *testing.T) {
	const n0 = 50.0
	const incl = 60.0 * math.Pi / 180
	downSensor := Vec3{0, 0, 1}
	accel := Vec3{0, 0, -9.81}
	magNorth := Vec3{n0 * math.Cos(incl), 0, -n0 * math.Sin(incl)}
	Raccel, err := BuildAlignRotation(accel, magNorth, 0)
	if err != nil {
		t.Fatalf("accel build: %v", err)
	}
	bVehicle := ApplyRot(Raccel, magNorth)
	R, err := BuildAlignRotationFromGeo(downSensor, magNorth, bVehicle, 0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got := ApplyRot(R, magNorth)
	if !approx(got.X, bVehicle.X) || !approx(got.Y, bVehicle.Y) || !approx(got.Z, bVehicle.Z) {
		t.Errorf("aligned mag %+v want %+v", got, bVehicle)
	}
	if gotH := HeadingFromAligned(got); !approx(gotH, 0) {
		t.Errorf("heading got %.6f want 0", gotH)
	}
}

func TestIsValidRot(t *testing.T) {
	if IsValidRot(Mat3{}) {
		t.Errorf("zero matrix should not be valid")
	}
	id := Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	if !IsValidRot(id) {
		t.Errorf("identity should be valid")
	}
}

func formatDeg(d float64) string {
	return strconv.FormatFloat(d, 'f', 2, 64)
}
