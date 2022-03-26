package kalman

import (
	"math"
)

const (
	deg = math.Pi / 180
)

type MeasureAgg struct {
	M0, M1, M2 float64 // Averaged measurements near this point
	MM         float64 // M0*M0+M1*M1+M2*M2
	SS         float64 // variance of all measurements relative to mean
	N          int     // number of measurements included at this point
}

func (a *MeasureAgg) Add(m0, m1, m2 float64) {
	mOld0 := a.M0
	mOld1 := a.M1
	mOld2 := a.M2
	fN := float64(a.N)
	a.M0 = (fN*mOld0 + m0) / (fN + 1)
	a.M1 = (fN*mOld1 + m1) / (fN + 1)
	a.M2 = (fN*mOld2 + m2) / (fN + 1)
	a.MM = (fN*a.MM + m0*m0 + m1*m1 + m2*m2) / (fN + 1)
	a.SS = (fN*(a.SS+
		(mOld0-a.M0)*(mOld0-a.M0)+(mOld1-a.M1)*(mOld1-a.M1)+(mOld2-a.M2)*(mOld2-a.M2)) +
		(m0-a.M0)*(m0-a.M0) + (m1-a.M1)*(m1-a.M1) + (m2-a.M2)*(m2-a.M2)) / (fN + 1)
	a.N += 1
}

type MeasureGrid struct {
	measurements [][]MeasureAgg
	Thetas       []float64
	Phis         [][]float64
	N            int
	Size         int
}

func NewMeasureGrid(nTheta int) (g MeasureGrid) {
	fnTheta := float64(nTheta)
	g.Thetas = make([]float64, nTheta)
	g.Phis = make([][]float64, nTheta)
	g.measurements = make([][]MeasureAgg, nTheta)
	patchSize := (math.Pi / fnTheta) * (math.Pi / fnTheta)
	for i := 0; i < nTheta; i++ {
		g.Thetas[i] = math.Pi * (0.5 - (float64(i)+0.5)/fnTheta)
		nPhi := int(2*math.Pi*
			(math.Sin(g.Thetas[i]+0.5*math.Pi/fnTheta)-math.Sin(g.Thetas[i]-0.5*math.Pi/fnTheta))/
			patchSize + 0.5)
		g.Phis[i] = make([]float64, nPhi)
		g.measurements[i] = make([]MeasureAgg, nPhi)
		for j := 0; j < nPhi; j++ {
			g.Phis[i][j] = 2 * math.Pi * (float64(j) + 0.5) / float64(nPhi)
		}
		g.Size += nPhi
	}
	return g
}

func (g *MeasureGrid) Add(m0, m1, m2 float64) {
	mm := math.Sqrt(m0*m0 + m1*m1 + m2*m2)
	theta := math.Asin(m2 / mm)
	phi := math.Atan2(m1, m0)
	iTheta := int((0.5 - theta/math.Pi) * float64(len(g.Thetas)))
	iPhi := int((phi+2*math.Pi)/(2*math.Pi)*float64(len(g.Phis[iTheta]))) % len(g.Phis[iTheta])

	g.measurements[iTheta][iPhi].Add(m0, m1, m2)
	g.N += 1
}

func (g MeasureGrid) Ns() (c [][]int) {
	c = make([][]int, len(g.Thetas))
	for i := 0; i < len(g.Thetas); i++ {
		c[i] = make([]int, len(g.Phis[i]))
		for j := 0; j < len(g.Phis[i]); j++ {
			c[i][j] = g.measurements[i][j].N
		}
	}
	return c
}

func (g MeasureGrid) Averages() (m [][][]float64) {
	m = make([][][]float64, len(g.Thetas))
	for i := 0; i < len(g.Thetas); i++ {
		m[i] = make([][]float64, len(g.Phis[i]))
		for j := 0; j < len(g.Phis[i]); j++ {
			m[i][j] = []float64{g.measurements[i][j].M0, g.measurements[i][j].M1, g.measurements[i][j].M2}
			if g.measurements[i][j].N == 0 {
				m[i][j] = []float64{math.NaN(), math.NaN(), math.NaN()}
			}
		}
	}
	return m
}

func (g MeasureGrid) StDevs() (s [][]float64) {
	s = make([][]float64, len(g.Thetas))
	for i := 0; i < len(g.Thetas); i++ {
		s[i] = make([]float64, len(g.Phis[i]))
		for j := 0; j < len(g.Phis[i]); j++ {
			s[i][j] = g.measurements[i][j].SS
			if g.measurements[i][j].N == 0 {
				s[i][j] = math.NaN()
			}
		}
	}
	return s
}

func (g MeasureGrid) CalculatedField(k, l []float64) (f [][][]float64) {
	m := g.Averages()
	f = make([][][]float64, len(m))
	for i := 0; i < len(m); i++ {
		f[i] = make([][]float64, len(m[i]))
		for j := 0; j < len(m[i]); j++ {
			f[i][j] = []float64{k[0] * (m[i][j][0] - l[0]), k[1] * (m[i][j][1] - l[1]), k[2] * (m[i][j][2] - l[2])}
			if g.measurements[i][j].N == 0 {
				f[i][j] = []float64{math.NaN(), math.NaN(), math.NaN()}
			}
		}
	}
	return f
}

func (g MeasureGrid) CalculatedFieldStrength2(k, l []float64) (s [][]float64) {
	m := g.Averages()
	s = make([][]float64, len(m))
	for i := 0; i < len(m); i++ {
		s[i] = make([]float64, len(m[i]))
		for j := 0; j < len(m[i]); j++ {
			s[i][j] = (k[0]*(m[i][j][0]-l[0]))*(k[0]*(m[i][j][0]-l[0])) +
				(k[1]*(m[i][j][1]-l[1]))*(k[1]*(m[i][j][1]-l[1])) +
				(k[2]*(m[i][j][2]-l[2]))*(k[2]*(m[i][j][2]-l[2]))
		}
	}
	return s
}
