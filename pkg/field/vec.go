package field

import "math"

// Vec3 is a 3-vector in sensor or field coordinates (µT for mag, m/s² for accel).
type Vec3 struct {
	X, Y, Z float64
}

// Mat3 is a 3×3 rotation matrix stored row-major; Mat3[i][j] is row i, col j.
type Mat3 [3][3]float64

func dot(a, b Vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

func cross(a, b Vec3) Vec3 {
	return Vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

func normalize(v Vec3) (Vec3, bool) {
	n := math.Sqrt(dot(v, v))
	if n == 0 {
		return Vec3{}, false
	}
	return Vec3{v.X / n, v.Y / n, v.Z / n}, true
}

// projectHoriz returns v projected onto the plane perpendicular to dHat, normalized.
func projectHoriz(v, dHat Vec3) (Vec3, bool) {
	c := dot(v, dHat)
	return normalize(Vec3{v.X - c*dHat.X, v.Y - c*dHat.Y, v.Z - c*dHat.Z})
}

// ApplyRot returns R · v.
func ApplyRot(R Mat3, v Vec3) Vec3 {
	return Vec3{
		R[0][0]*v.X + R[0][1]*v.Y + R[0][2]*v.Z,
		R[1][0]*v.X + R[1][1]*v.Y + R[1][2]*v.Z,
		R[2][0]*v.X + R[2][1]*v.Y + R[2][2]*v.Z,
	}
}

// ApplyRotT returns R^T · v. R is orthonormal so R^T = R^-1.
func ApplyRotT(R Mat3, v Vec3) Vec3 {
	return Vec3{
		R[0][0]*v.X + R[1][0]*v.Y + R[2][0]*v.Z,
		R[0][1]*v.X + R[1][1]*v.Y + R[2][1]*v.Z,
		R[0][2]*v.X + R[1][2]*v.Y + R[2][2]*v.Z,
	}
}

// WrapPi normalizes an angle to (-π, π].
func WrapPi(x float64) float64 {
	for x > math.Pi {
		x -= 2 * math.Pi
	}
	for x <= -math.Pi {
		x += 2 * math.Pi
	}
	return x
}

// WrapDeg normalizes an angle to (-180, 180].
func WrapDeg(d float64) float64 {
	for d > 180 {
		d -= 360
	}
	for d <= -180 {
		d += 360
	}
	return d
}
