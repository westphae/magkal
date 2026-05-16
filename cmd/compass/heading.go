package main

import (
	"fmt"
	"math"
)

// Alignment captures the sensor's mounting attitude and yaw offset *once*,
// when the user clicks Align with the vehicle held in a known orientation
// (level + known heading). The resulting 3x3 matrix R maps any subsequent
// sensor-frame vector v_s to a fixed "vehicle" frame v_v = R · v_s where
//   row 0 (forward) = vehicle's forward direction at align time,
//   row 1 (right)   = vehicle's right direction,
//   row 2 (down)    = local gravity direction captured at align.
// Heading is then a single atan2 on the horizontal components of R · m_cal.
// No per-sample accelerometer is involved, so accel noise no longer feeds
// into the displayed heading.

// mat3 is a 3x3 rotation matrix stored row-major; mat3[i][j] is row i col j.
type mat3 [3][3]float64

func wrapPi(x float64) float64 {
	for x > math.Pi {
		x -= 2 * math.Pi
	}
	for x <= -math.Pi {
		x += 2 * math.Pi
	}
	return x
}

func dot(a, b vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func cross(a, b vec3) vec3 {
	return vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

func normalize(v vec3) (vec3, bool) {
	n := math.Sqrt(dot(v, v))
	if n == 0 {
		return vec3{}, false
	}
	return vec3{v.X / n, v.Y / n, v.Z / n}, true
}

// projectHoriz returns v projected onto the plane perpendicular to dHat,
// normalized. Returns ok=false if v is parallel to dHat.
func projectHoriz(v, dHat vec3) (vec3, bool) {
	c := dot(v, dHat)
	return normalize(vec3{v.X - c*dHat.X, v.Y - c*dHat.Y, v.Z - c*dHat.Z})
}

// applyRot returns R · v.
func applyRot(R mat3, v vec3) vec3 {
	return vec3{
		R[0][0]*v.X + R[0][1]*v.Y + R[0][2]*v.Z,
		R[1][0]*v.X + R[1][1]*v.Y + R[1][2]*v.Z,
		R[2][0]*v.X + R[2][1]*v.Y + R[2][2]*v.Z,
	}
}

// applyRotT returns R^T · v. R is orthonormal so R^T = R^-1.
func applyRotT(R mat3, v vec3) vec3 {
	return vec3{
		R[0][0]*v.X + R[1][0]*v.Y + R[2][0]*v.Z,
		R[0][1]*v.X + R[1][1]*v.Y + R[2][1]*v.Z,
		R[0][2]*v.X + R[1][2]*v.Y + R[2][2]*v.Z,
	}
}

// isValidRot returns true if R looks like a non-degenerate rotation
// (orthonormal rows). A zero matrix loaded from disk fails this check.
func isValidRot(R mat3) bool {
	for i := 0; i < 3; i++ {
		row := vec3{R[i][0], R[i][1], R[i][2]}
		if math.Abs(dot(row, row)-1) > 1e-3 {
			return false
		}
	}
	return true
}

// BuildAlignRotation constructs the alignment matrix R from one captured
// (accel, magCal) sample plus the vehicle's known magnetic heading. The
// accel vector defines the down axis (gravity); the magCal vector defines
// magnetic north in the horizontal plane; headingMagRad is the angle (CW
// from magnetic north, viewed from above) of the vehicle's forward axis.
//
// At any subsequent time, vehicle-frame mag = R · m_cal_sensor; heading
// follows from HeadingFromAligned. The matrix is fixed for the duration
// of the mounted session and is rebuilt only when the user re-aligns.
func BuildAlignRotation(accel, magCal vec3, headingMagRad float64) (mat3, error) {
	dHat, ok := normalize(vec3{-accel.X, -accel.Y, -accel.Z})
	if !ok {
		return mat3{}, fmt.Errorf("zero accel")
	}
	nHat, ok := projectHoriz(magCal, dHat)
	if !ok {
		return mat3{}, fmt.Errorf("magCal is parallel to gravity")
	}
	eHat := cross(dHat, nHat)
	cosH := math.Cos(headingMagRad)
	sinH := math.Sin(headingMagRad)
	fwd := vec3{
		cosH*nHat.X + sinH*eHat.X,
		cosH*nHat.Y + sinH*eHat.Y,
		cosH*nHat.Z + sinH*eHat.Z,
	}
	right := cross(dHat, fwd)
	return mat3{
		{fwd.X, fwd.Y, fwd.Z},
		{right.X, right.Y, right.Z},
		{dHat.X, dHat.Y, dHat.Z},
	}, nil
}

// HeadingFromAligned returns the compass heading (rad, magnetic) given a
// calibrated mag vector already expressed in the vehicle frame. The
// horizontal components have north = +x and east = -y (since right = D×N is
// west in NED-style vehicle frame), so heading = atan2(-m.y, m.x).
func HeadingFromAligned(magVehicle vec3) float64 {
	return wrapPi(math.Atan2(-magVehicle.Y, magVehicle.X))
}

// PredictRawMag returns the raw magnetometer reading the model expects at
// the given compass heading, given the alignment R, local field parameters
// (n0 in µT, inclination in deg), and the per-axis calibration (k, l).
// Used to overlay "expected" vs "measured" raw mag on the UI/CSV.
func PredictRawMag(headingMagRad, n0Ut, inclDeg float64, R mat3, k, l []float64) (vec3, bool) {
	if len(k) < 3 || len(l) < 3 || k[0] == 0 || k[1] == 0 || k[2] == 0 {
		return vec3{}, false
	}
	incl := inclDeg * math.Pi / 180
	cosI := math.Cos(incl)
	sinI := math.Sin(incl)
	mV := vec3{
		n0Ut * cosI * math.Cos(headingMagRad),
		-n0Ut * cosI * math.Sin(headingMagRad),
		n0Ut * sinI,
	}
	mS := applyRotT(R, mV)
	return vec3{
		l[0] + mS.X/k[0],
		l[1] + mS.Y/k[1],
		l[2] + mS.Z/k[2],
	}, true
}

// ApplyCal applies n = k*(m - l) per axis.
func ApplyCal(raw vec3, k, l []float64) vec3 {
	if len(k) < 3 || len(l) < 3 {
		return vec3{}
	}
	return vec3{
		k[0] * (raw.X - l[0]),
		k[1] * (raw.Y - l[1]),
		k[2] * (raw.Z - l[2]),
	}
}
