package field

import (
	"fmt"
	"math"
)

// Measured holds geomagnetic quantities derived from a calibrated mag vector.
// F is always set (µT). Other fields are nil when not computable this tick.
type Measured struct {
	F       float64
	H       *float64
	ZDown   *float64
	InclDeg *float64
	DeclDeg *float64
	X       *float64
	Y       *float64
}

// IsValidRot returns true if R looks like a non-degenerate rotation (orthonormal rows).
func IsValidRot(R Mat3) bool {
	for i := 0; i < 3; i++ {
		row := Vec3{R[i][0], R[i][1], R[i][2]}
		if math.Abs(dot(row, row)-1) > 1e-3 {
			return false
		}
	}
	return true
}

// BuildAlignRotation constructs the sensor→vehicle alignment matrix R from one
// (accel, magCal) snapshot and the vehicle magnetic heading (rad, CW from north).
func BuildAlignRotation(accel, magCal Vec3, headingMagRad float64) (Mat3, error) {
	dHat, ok := normalize(Vec3{-accel.X, -accel.Y, -accel.Z})
	if !ok {
		return Mat3{}, fmt.Errorf("zero accel")
	}
	nHat, ok := projectHoriz(magCal, dHat)
	if !ok {
		return Mat3{}, fmt.Errorf("magCal is parallel to gravity")
	}
	eHat := cross(dHat, nHat)
	cosH := math.Cos(headingMagRad)
	sinH := math.Sin(headingMagRad)
	fwd := Vec3{
		cosH*nHat.X + sinH*eHat.X,
		cosH*nHat.Y + sinH*eHat.Y,
		cosH*nHat.Z + sinH*eHat.Z,
	}
	right := cross(dHat, fwd)
	return Mat3{
		{fwd.X, fwd.Y, fwd.Z},
		{right.X, right.Y, right.Z},
		{dHat.X, dHat.Y, dHat.Z},
	}, nil
}

// BuildAlignRotationToField constructs sensor→vehicle R so R·magCal is parallel to
// bVehicle (µT, vehicle frame), then twists about vehicle down so magnetic heading
// matches headingMagRad. Use for WMM/pod mag when gravity is not in the mag frame.
func BuildAlignRotationToField(magCal, bVehicle Vec3, headingMagRad float64) (Mat3, error) {
	if _, ok := normalize(bVehicle); !ok {
		return Mat3{}, fmt.Errorf("zero field vector")
	}
	R0, err := rotationAlignVectors(magCal, bVehicle)
	if err != nil {
		return Mat3{}, err
	}
	magV := ApplyRot(R0, magCal)
	downV := Vec3{0, 0, 1}
	nCur, ok := projectHoriz(magV, downV)
	if !ok {
		return Mat3{}, fmt.Errorf("magCal field is vertical in vehicle frame")
	}
	nTgt, ok := projectHoriz(bVehicle, downV)
	if !ok {
		return Mat3{}, fmt.Errorf("target field is vertical in vehicle frame")
	}
	delta := math.Atan2(nCur.X*nTgt.Y-nCur.Y*nTgt.X, dot(nCur, nTgt))
	dHat := Vec3{R0[2][0], R0[2][1], R0[2][2]}
	Rd := rotAboutAxis(dHat, delta)
	return mulMat3(Rd, R0), nil
}

// BuildAlignRotationFromGeo constructs the sensor→vehicle alignment matrix R using
// fuselage down (unit vector in sensor coordinates), calibrated mag, and the
// expected geomagnetic field in the vehicle frame (µT, X fwd / Y right / Z down).
// Prefer BuildAlignRotationToField when down and mag are not in the same frame.
func BuildAlignRotationFromGeo(downSensor, magCal, bVehicle Vec3, headingMagRad float64) (Mat3, error) {
	dHat, ok := normalize(downSensor)
	if !ok {
		return Mat3{}, fmt.Errorf("zero down vector")
	}
	if _, ok := normalize(bVehicle); !ok {
		return Mat3{}, fmt.Errorf("zero field vector")
	}
	R0, err := BuildAlignRotation(Vec3{-downSensor.X, -downSensor.Y, -downSensor.Z}, magCal, headingMagRad)
	if err != nil {
		return Mat3{}, err
	}
	magV := ApplyRot(R0, magCal)
	downV := Vec3{0, 0, 1}
	nCur, ok := projectHoriz(magV, downV)
	if !ok {
		return Mat3{}, fmt.Errorf("magCal field is vertical in vehicle frame")
	}
	nTgt, ok := projectHoriz(bVehicle, downV)
	if !ok {
		return Mat3{}, fmt.Errorf("target field is vertical in vehicle frame")
	}
	delta := math.Atan2(nCur.X*nTgt.Y-nCur.Y*nTgt.X, dot(nCur, nTgt))
	Rd := rotAboutAxis(dHat, delta)
	return mulMat3(Rd, R0), nil
}

// FieldNED returns nT components and derived scalars from a NED field vector (µT).
func FieldNED(bNedUt Vec3) (xNt, yNt, zNt, hNt, fNt, inclDeg float64) {
	xNt = bNedUt.X * 1000
	yNt = bNedUt.Y * 1000
	zNt = bNedUt.Z * 1000
	hUt := math.Hypot(bNedUt.X, bNedUt.Y)
	hNt = hUt * 1000
	fNt = math.Sqrt(dot(bNedUt, bNedUt)) * 1000
	if hUt > 0 {
		inclDeg = math.Atan2(bNedUt.Z, hUt) * 180 / math.Pi
	}
	return xNt, yNt, zNt, hNt, fNt, inclDeg
}

func rotAboutAxis(axis Vec3, theta float64) Mat3 {
	u, ok := normalize(axis)
	if !ok {
		return Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	}
	c := math.Cos(theta)
	s := math.Sin(theta)
	t := 1 - c
	return Mat3{
		{t*u.X*u.X + c, t*u.X*u.Y - s*u.Z, t*u.X*u.Z + s*u.Y},
		{t*u.X*u.Y + s*u.Z, t*u.Y*u.Y + c, t*u.Y*u.Z - s*u.X},
		{t*u.X*u.Z - s*u.Y, t*u.Y*u.Z + s*u.X, t*u.Z*u.Z + c},
	}
}

func mulMat3(a, b Mat3) Mat3 {
	var out Mat3
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i][j] = a[i][0]*b[0][j] + a[i][1]*b[1][j] + a[i][2]*b[2][j]
		}
	}
	return out
}

// rotationAlignVectors returns R such that R·from is parallel to to (|from|, |to| > 0).
func rotationAlignVectors(from, to Vec3) (Mat3, error) {
	fHat, ok := normalize(from)
	if !ok {
		return Mat3{}, fmt.Errorf("zero from vector")
	}
	tHat, ok := normalize(to)
	if !ok {
		return Mat3{}, fmt.Errorf("zero to vector")
	}
	c := dot(fHat, tHat)
	if c > 1-1e-12 {
		return Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, nil
	}
	if c < -1+1e-12 {
		axis, ok := normalize(cross(fHat, Vec3{1, 0, 0}))
		if !ok {
			axis, ok = normalize(cross(fHat, Vec3{0, 1, 0}))
			if !ok {
				return Mat3{}, fmt.Errorf("degenerate 180° alignment")
			}
		}
		return rotAboutAxis(axis, math.Pi), nil
	}
	axis := cross(fHat, tHat)
	return rotAboutAxis(axis, math.Acos(c)), nil
}

// ScaleDirectionToMagnitude returns dir scaled to |magRef| (µT).
func ScaleDirectionToMagnitude(magRef, dir Vec3) Vec3 {
	m := math.Sqrt(dot(magRef, magRef))
	d := math.Sqrt(dot(dir, dir))
	if d == 0 {
		return dir
	}
	s := m / d
	return Vec3{dir.X * s, dir.Y * s, dir.Z * s}
}

// HeadingFromAligned returns magnetic heading (rad) from a vehicle-frame mag vector.
func HeadingFromAligned(magVehicle Vec3) float64 {
	return WrapPi(math.Atan2(-magVehicle.Y, magVehicle.X))
}

// HeadingSensorDeg returns sensor-frame heading (deg) from calibrated mag (µT).
func HeadingSensorDeg(magCal Vec3) float64 {
	return HeadingDeg360(WrapPi(math.Atan2(-magCal.Y, magCal.X)) * 180 / math.Pi)
}

// HeadingDeg360 maps any angle to [0, 360).
func HeadingDeg360(deg float64) float64 {
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	return d
}

// PredictRawMag returns the raw magnetometer reading expected at headingMagRad.
func PredictRawMag(headingMagRad, n0Ut, inclDeg float64, R Mat3, k, l []float64) (Vec3, bool) {
	if len(k) < 3 || len(l) < 3 || k[0] == 0 || k[1] == 0 || k[2] == 0 {
		return Vec3{}, false
	}
	incl := inclDeg * math.Pi / 180
	cosI := math.Cos(incl)
	sinI := math.Sin(incl)
	mV := Vec3{
		n0Ut * cosI * math.Cos(headingMagRad),
		-n0Ut * cosI * math.Sin(headingMagRad),
		n0Ut * sinI,
	}
	mS := ApplyRotT(R, mV)
	return Vec3{
		l[0] + mS.X/k[0],
		l[1] + mS.Y/k[1],
		l[2] + mS.Z/k[2],
	}, true
}

// ApplyCal applies n = k*(m - l) per axis (µT).
func ApplyCal(raw Vec3, k, l []float64) Vec3 {
	if len(k) < 3 || len(l) < 3 {
		return Vec3{}
	}
	return Vec3{
		k[0] * (raw.X - l[0]),
		k[1] * (raw.Y - l[1]),
		k[2] * (raw.Z - l[2]),
	}
}

// MeasureField derives geomagnetic quantities from calibrated mag (µT) and accel (m/s²).
// When haveVehHeading and trackTrueDeg are set, DeclDeg/X/Y use measured decl =
// track_true − vehHeadingMagDeg.
func MeasureField(magCal, accel Vec3, haveVehHeading bool, vehHeadingMagDeg, trackTrueDeg float64) Measured {
	out := Measured{F: math.Sqrt(dot(magCal, magCal))}
	dHat, ok := normalize(Vec3{-accel.X, -accel.Y, -accel.Z})
	if !ok {
		return out
	}
	zDown := dot(magCal, dHat)
	horiz := Vec3{magCal.X - zDown*dHat.X, magCal.Y - zDown*dHat.Y, magCal.Z - zDown*dHat.Z}
	h := math.Sqrt(dot(horiz, horiz))
	incl := math.Atan2(zDown, h) * 180 / math.Pi
	out.H = &h
	out.ZDown = &zDown
	out.InclDeg = &incl
	if haveVehHeading && !math.IsNaN(trackTrueDeg) {
		d := WrapDeg(trackTrueDeg - vehHeadingMagDeg)
		dRad := d * math.Pi / 180
		x := h * math.Cos(dRad)
		y := h * math.Sin(dRad)
		out.DeclDeg = &d
		out.X = &x
		out.Y = &y
	}
	return out
}
