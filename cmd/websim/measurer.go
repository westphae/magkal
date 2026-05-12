package main

import (
	"math"
	"math/rand"

	"github.com/kidoman/embd"
	_ "github.com/kidoman/embd/host/all"
	_ "github.com/kidoman/embd/host/rpi"
	"github.com/westphae/goflying/sensors/icm20948"
)

const deg = math.Pi / 180

type measurement []float64 // A magnetometer measurement like [m1, m2, m3]
type direction []float64   // Angles pointing in a direction like [theta, phi], in degrees
type measurer func(a direction) (m measurement)

// makeRandomMeasurer creates a function that returns a new measurement of m, the magnetometer measurement.
// Inputs:
//   n: number of dimensions (1, 2, or 3)
//   n0: Earth's magnetic field (1.0 is fine for testing)
//   k: n-vector of the scaling factors
//   l: n-vector of the additive factors
//   r: noise level
//   equation is n = k*(m-l)
// The returned function takes a rough measurement just to satisfy the interface, but doesn't use it.
func makeRandomMeasurer(n int, n0 float64, k, l []float64, r float64) (m measurer, err error) {
	var theta, phi float64
	if n == 1 {
		return func(a direction) (m measurement) {
			theta = 360 * deg * (rand.Float64() - 0.5)
			if theta < 0 {
				return measurement{-n0/k[0] + l[0] + r*rand.NormFloat64()}
			}
			return measurement{n0/k[0] + l[0] + r*rand.NormFloat64()}
		}, nil
	}
	if n == 2 {
		const w = 0.01
		var (
			theta = 0.0
			rot   = 5 * deg
		)

		return func(a direction) (m measurement) {
			rot = (1-w)*rot + w*5*deg*2*(rand.Float64()-0.5)
			if rand.Float64() < w {
				if rand.Float64() < 0.5 {
					rot += 5 * deg
				} else {
					rot -= 5 * deg
				}
			}
			theta += rot
			nx := n0 * math.Cos(theta)
			ny := n0 * math.Sin(theta)
			return measurement{
				nx/k[0] + l[0] + r*rand.NormFloat64(),
				ny/k[1] + l[1] + r*rand.NormFloat64(),
			}
		}, nil
	}
	return func(a direction) (m measurement) {
		phi = 2 * math.Pi * (rand.Float64() - 0.5)
		theta = math.Asin(2*rand.Float64() - 1)
		nx := n0 * math.Cos(phi) * math.Cos(theta)
		ny := n0 * math.Sin(phi) * math.Cos(theta)
		nz := n0 * math.Sin(theta)
		return measurement{
			nx/k[0] + l[0] + r*rand.NormFloat64(),
			ny/k[1] + l[1] + r*rand.NormFloat64(),
			nz/k[2] + l[2] + r*rand.NormFloat64(),
		}
	}, nil
}

// makeManualMeasurer creates a function that returns a new measurement of m, the magnetometer measurement.
// Inputs:
//   n: number of dimensions (1, 2, or 3)
//   n0: Earth's magnetic field (1.0 is fine for testing)
//   k: n-vector of the scaling factors
//   l: n-vector of the additive factors
//   r: noise level
//   equation is n = k*(m-l)
// The returned function takes a rough measurement and computes the corresponding angles, then computes
//   a corrected measurement including noise.
func makeManualMeasurer(n int, n0 float64, k, l []float64, r float64) (m measurer, err error) {
	if n == 1 {
		return func(a direction) (m measurement) {
			var theta float64
			if a != nil && len(a) >= 1 {
				theta = a[0] * deg
			} else {
				theta = 2 * math.Pi * rand.Float64()
			}
			if theta > math.Pi/2 && theta < 3*math.Pi/2 {
				return []float64{-n0/k[0] + l[0] + r*rand.NormFloat64()}
			}
			return []float64{n0/k[0] + l[0] + r*rand.NormFloat64()}
		}, nil
	}
	if n == 2 {
		return func(a direction) (m measurement) {
			var theta float64
			if a != nil && len(a) >= 1 {
				theta = a[0] * math.Pi / 180
			} else {
				theta = 2 * math.Pi * rand.Float64()
			}
			nx := n0 * math.Cos(theta)
			ny := n0 * math.Sin(theta)
			return []float64{
				nx/k[0] + l[0] + r*rand.NormFloat64(),
				ny/k[1] + l[1] + r*rand.NormFloat64(),
			}
		}, nil
	}
	return func(a direction) (m measurement) {
		var theta, phi float64
		if a != nil && len(a) >= 2 {
			theta = a[0] * math.Pi / 180
			phi = a[1] * math.Pi / 180
		} else {
			theta = 2 * math.Pi * rand.Float64()
			phi = math.Acos(2*rand.Float64() - 1)
		}
		nx := n0 * math.Cos(theta) * math.Cos(phi)
		ny := n0 * math.Sin(theta) * math.Cos(phi)
		nz := n0 * math.Sin(phi)
		return []float64{
			nx/k[0] + l[0] + r*rand.NormFloat64(),
			ny/k[1] + l[1] + r*rand.NormFloat64(),
			nz/k[2] + l[2] + r*rand.NormFloat64(),
		}
	}, nil
}

// actualMPU is a process-wide singleton ICM-20948 handle. The Pi has one
// physical sensor; re-initializing on every params change would leak I²C
// handles and the driver's polling goroutine. First "Actual" selection
// initializes; later selections reuse the handle. Not closed; process
// exit terminates the driver goroutine.
var actualMPU *icm20948.ICM20948

// makeActualMeasurer returns a measurer that reads the physical ICM-20948
// over I²C bus 1 at MPU_ADDRESS1. M1/M2/M3 are AK09916 readings in µT;
// the driver's user-cal matrix is reset to identity so the EKF sees
// pre-calibration data regardless of /etc/imu_cal.json contents.
func makeActualMeasurer() (measurer, error) {
	if actualMPU == nil {
		// embd's DetectHost() runs `uname -r` and chokes on modern Pi
		// kernel strings (e.g. "6.12.62+rpt-rpi-v8") because parseVersion
		// tries to Atoi("62+rpt"). SetHost bypasses detection; the RPi
		// describer's I2CDriver doesn't read the revision number, so any
		// value works.
		embd.SetHost(embd.HostRPi, 0)
		bus := embd.NewI2CBus(1)
		mpu, err := icm20948.NewICM20948(&bus, icm20948.MPU_ADDRESS1,
			250, 4, 50, true, false)
		if err != nil {
			return nil, err
		}
		mpu.IMUCalData.Reset()
		actualMPU = mpu
	}
	mpu := actualMPU
	return func(_ direction) measurement {
		data := <-mpu.C
		return measurement{data.M1, data.M2, data.M3}
	}, nil
}
