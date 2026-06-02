package main

import "github.com/westphae/magkal/pkg/field"

type mat3 = field.Mat3

func toFieldVec(v vec3) field.Vec3 {
	return field.Vec3{X: v.X, Y: v.Y, Z: v.Z}
}

func fromFieldVec(v field.Vec3) vec3 {
	return vec3{X: v.X, Y: v.Y, Z: v.Z}
}

func ApplyCal(raw vec3, k, l []float64) vec3 {
	return fromFieldVec(field.ApplyCal(toFieldVec(raw), k, l))
}

func applyRot(R mat3, v vec3) vec3 {
	return fromFieldVec(field.ApplyRot(R, toFieldVec(v)))
}

func BuildAlignRotation(accel, magCal vec3, headingMagRad float64) (mat3, error) {
	return field.BuildAlignRotation(toFieldVec(accel), toFieldVec(magCal), headingMagRad)
}

func isValidRot(R mat3) bool {
	return field.IsValidRot(R)
}

func HeadingFromAligned(magVehicle vec3) float64 {
	return field.HeadingFromAligned(toFieldVec(magVehicle))
}

func PredictRawMag(headingMagRad, n0Ut, inclDeg float64, R mat3, k, l []float64) (vec3, bool) {
	v, ok := field.PredictRawMag(headingMagRad, n0Ut, inclDeg, R, k, l)
	return fromFieldVec(v), ok
}

func measureGeomag(magCal, accel vec3, haveVehHeading bool, vehHeadingMagDeg, trackTrueDeg float64) geomagMeasuredPayload {
	m := field.MeasureField(toFieldVec(magCal), toFieldVec(accel), haveVehHeading, vehHeadingMagDeg, trackTrueDeg)
	out := geomagMeasuredPayload{F: m.F}
	out.H = m.H
	out.ZDown = m.ZDown
	out.InclDeg = m.InclDeg
	out.DeclDeg = m.DeclDeg
	out.X = m.X
	out.Y = m.Y
	return out
}
