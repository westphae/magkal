package main

import "math"

// Sensor body frame is whatever the ICM20948 driver returns. Accel is in
// m/s² (positive when device acceleration opposes -gravity); mag_cal is the
// calibrated k*(m - l) vector in µT. The math below makes no assumption
// about which sensor axis is up/down/forward — it builds a local NED basis
// from accel and (where needed) the mounting yaw offset captured at Align.
//
// Compass heading conventions: 0 = magnetic north, +π/2 = east, measured CW
// from above about the local "down" direction D̂. wrap() folds to (-π, π].

func wrapPi(x float64) float64 {
	for x > math.Pi {
		x -= 2 * math.Pi
	}
	for x <= -math.Pi {
		x += 2 * math.Pi
	}
	return x
}

func vecLen(v vec3) float64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
}

func vecScale(v vec3, s float64) vec3 { return vec3{v.X * s, v.Y * s, v.Z * s} }
func vecAdd(a, b vec3) vec3           { return vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func vecSub(a, b vec3) vec3           { return vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func dot(a, b vec3) float64           { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func cross(a, b vec3) vec3 {
	return vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

func normalize(v vec3) vec3 {
	n := vecLen(v)
	if n == 0 {
		return v
	}
	return vecScale(v, 1/n)
}

// rotateAxis rotates v about the unit-length axis by angle (Rodrigues).
func rotateAxis(v, axis vec3, angle float64) vec3 {
	c := math.Cos(angle)
	s := math.Sin(angle)
	kxv := cross(axis, v)
	kdv := dot(axis, v)
	return vec3{
		v.X*c + kxv.X*s + axis.X*kdv*(1-c),
		v.Y*c + kxv.Y*s + axis.Y*kdv*(1-c),
		v.Z*c + kxv.Z*s + axis.Z*kdv*(1-c),
	}
}

// dHatFromAccel returns the local down direction expressed in sensor coords.
// accel measures specific force (= -gravity at rest), so down = -accel/|accel|.
// Returns (false, _) if accel magnitude is zero.
func dHatFromAccel(accel vec3) (vec3, bool) {
	n := vecLen(accel)
	if n == 0 {
		return vec3{}, false
	}
	return vec3{-accel.X / n, -accel.Y / n, -accel.Z / n}, true
}

// projectHoriz returns v projected onto the plane perpendicular to d, then
// normalized. Returns (false, _) if v is parallel to d.
func projectHoriz(v, d vec3) (vec3, bool) {
	c := dot(v, d)
	h := vec3{v.X - c*d.X, v.Y - c*d.Y, v.Z - c*d.Z}
	n := vecLen(h)
	if n == 0 {
		return vec3{}, false
	}
	return vecScale(h, 1/n), true
}

// HeadingSensorTiltOn returns the compass heading (rad) of sensor-x, using
// the accel vector to define "down". Tilt-compensated: works at any roll/pitch
// as long as accel is dominated by gravity. Returns (0, false) on degenerate
// inputs (zero accel or magCal parallel to D̂ or sensor-x parallel to D̂).
func HeadingSensorTiltOn(accel, magCal vec3) (float64, bool) {
	dHat, ok := dHatFromAccel(accel)
	if !ok {
		return 0, false
	}
	nHat, ok := projectHoriz(magCal, dHat)
	if !ok {
		return 0, false
	}
	eHat := cross(dHat, nHat)
	xHoriz, ok := projectHoriz(vec3{1, 0, 0}, dHat)
	if !ok {
		return 0, false
	}
	return wrapPi(math.Atan2(dot(xHoriz, eHat), dot(xHoriz, nHat))), true
}

// HeadingSensorTiltOff returns the compass heading of sensor-x assuming the
// sensor is level (D̂ = -sensor-z, i.e. z-up mounting). For other mountings
// the value differs from HeadingSensorTiltOn by a constant that gets folded
// into yaw_offset at Align time, so the displayed vehicle heading remains
// usable for relative comparisons.
func HeadingSensorTiltOff(magCal vec3) float64 {
	return wrapPi(math.Atan2(magCal.Y, magCal.X))
}

// VehicleHeading composes the sensor heading with the saved mounting yaw
// offset to produce the compass heading of the vehicle's forward axis.
func VehicleHeading(headingSensor, yawOffset float64) float64 {
	return wrapPi(headingSensor + yawOffset)
}

// PredictRawMag returns the raw magnetometer reading that the model would
// produce at the given truth heading. Inverts the per-axis calibration after
// computing the predicted calibrated-n vector in sensor frame.
//
// trackMag is the compass heading of the vehicle's forward axis (rad), n0Ut
// is the local field magnitude (µT), incl is the magnetic inclination
// (positive down, deg), and yawOffset is the saved mounting offset (rad).
// Returns (zero, false) on degenerate accel.
func PredictRawMag(trackMag, n0Ut, inclDeg, yawOffset float64, accel vec3, k, l []float64) (vec3, bool) {
	dHat, ok := dHatFromAccel(accel)
	if !ok {
		return vec3{}, false
	}
	xHoriz, ok := projectHoriz(vec3{1, 0, 0}, dHat)
	if !ok {
		return vec3{}, false
	}
	// Sensor-x has compass heading β = trackMag - yawOffset; magnetic-north
	// in sensor coords is xHoriz rotated by -β about D̂.
	beta := trackMag - yawOffset
	nHat := rotateAxis(xHoriz, dHat, -beta)
	incl := inclDeg * math.Pi / 180
	cosI := math.Cos(incl)
	sinI := math.Sin(incl)
	nPred := vec3{
		n0Ut*cosI*nHat.X + n0Ut*sinI*dHat.X,
		n0Ut*cosI*nHat.Y + n0Ut*sinI*dHat.Y,
		n0Ut*cosI*nHat.Z + n0Ut*sinI*dHat.Z,
	}
	if len(k) < 3 || len(l) < 3 || k[0] == 0 || k[1] == 0 || k[2] == 0 {
		return vec3{}, false
	}
	return vec3{
		l[0] + nPred.X/k[0],
		l[1] + nPred.Y/k[1],
		l[2] + nPred.Z/k[2],
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
