// Define a data structure that aggregates measurements
// Averages/counts measurements that are "close"
// Can quickly apply the current K/L and return the associated N^2
package kalman

import (
	"math"
)

const (
	deg = math.Pi/180
)

type MeasureAgg struct {
	M0, M1, M2 float64 // Averaged measurements near this point
	MM float64 // M0*M0+M1*M1+M2*M2
	SS float64 // variance of all measurements relative to mean
	N float64 // number of measurements included at this point
}

func (a *MeasureAgg) Add(m0, m1, m2 float64) {
	mOld0 := a.M0
	mOld1 := a.M1
	mOld2 := a.M2
	a.M0 = (a.N*mOld0+m0)/(a.N+1)
	a.M1 = (a.N*mOld1+m1)/(a.N+1)
	a.M2 = (a.N*mOld2+m2)/(a.N+1)
	a.MM = (a.N*a.MM+m0*m0+m1*m1+m2*m2)/(a.N+1)
	a.SS = (a.N*(a.SS +
		(mOld0-a.M0)*(mOld0-a.M0) + (mOld1-a.M1)*(mOld1-a.M1) + (mOld2-a.M2)*(mOld2-a.M2)) +
		(m0-a.M0)*(m0-a.M0) + (m1-a.M1)*(m1-a.M1) + (m2-a.M2)*(m2-a.M2))/(a.N+1)
	a.N += 1
}

type MeasureGrid struct {
	measurements [][]MeasureAgg
	Thetas       []float64
	Phis         [][]float64
	N            int
}

func NewMeasureGrid(nTheta int) (g MeasureGrid) {
	g = MeasureGrid{}
	fnTheta := float64(nTheta)
	g.Thetas = make([]float64, nTheta)
	g.Phis = make([][]float64, nTheta)
	g.measurements = make([][]MeasureAgg, nTheta)
	patchSize := (math.Pi/fnTheta)*(math.Pi/fnTheta)
	for i:=0; i<nTheta; i++ {
		g.Thetas[i] = math.Pi*(0.5-(float64(i)+0.5)/fnTheta)
		nPhi := int(2*math.Pi*
			(math.Sin(g.Thetas[i]+0.5*math.Pi/fnTheta)-math.Sin(g.Thetas[i]-0.5*math.Pi/fnTheta)) /
		patchSize+0.5)
		g.Phis[i] = make([]float64, nPhi)
		g.measurements[i] = make([]MeasureAgg, nPhi)
		for j:=0; j<nPhi; j++ {
			g.Phis[i][j] = 2*math.Pi*(float64(j)+0.5)/float64(nPhi)
		}
	}
	return g
}

func (g *MeasureGrid) Add(m0, m1, m2 float64) {
	mm := math.Sqrt(m0*m0 + m1*m1 + m2*m2)
	theta := math.Asin(m2/mm)
	phi := math.Atan2(m0, m1)
	iTheta := int((0.5-theta/math.Pi)*float64(len(g.Thetas))+0.5)
	iPhi := int((phi+math.Pi)/(2*math.Pi)*float64(len(g.Phis[iTheta]))+0.5)

	g.measurements[iTheta][iPhi].Add(m0, m1, m2)
	g.N += 1
}

func (g MeasureGrid) Ns() (c []float64) {
	c = make([]float64, g.N)
	n := 0
	for i:=0; i<len(g.Thetas); i++ {
		for j:=0; j<len(g.Phis[i]); j++ {
			c[n] = g.measurements[i][j].N
			n += 1
		}
	}
	return c
}

func (g MeasureGrid) Averages() (m [][]float64) {
	m = make([][]float64, g.N)
	n := 0
	for i:=0; i<len(g.Thetas); i++ {
		for j:=0; j<len(g.Phis[i]); j++ {
			m[n] = []float64{g.measurements[i][j].M0, g.measurements[i][j].M1, g.measurements[i][j].M2}
			n += 1
		}
	}
	return m
}

func (g MeasureGrid) StDevs() (s []float64) {
	s = make([]float64, g.N)
	n := 0
	for i:=0; i<len(g.Thetas); i++ {
		for j:=0; j<len(g.Phis[i]); j++ {
			s[n] = g.measurements[i][j].SS
			n += 1
		}
	}
	return s
}

func (g MeasureGrid) CalculatedField(k, l []float64) (f [][]float64) {
	f = make([][]float64, g.N)
	m := g.Averages()
	for i:=0; i<len(m); i++ {
		f[i] = []float64{k[0]*(m[i][0]-l[0]), k[1]*(m[i][1]-l[1]), k[2]*(m[i][2]-l[2])}
	}
	return f
}

func (g MeasureGrid) CalculatedFieldStrength2(k, l []float64) (s []float64) {
	s = make([]float64, g.N)
	m := g.Averages()
	for i:=0; i<len(m); i++ {
		s[i] = (k[0]*(m[i][0]-l[0]))*(k[0]*(m[i][0]-l[0])) +
			(k[1]*(m[i][1]-l[1]))*(k[1]*(m[i][1]-l[1])) +
			(k[2]*(m[i][2]-l[2]))*(k[2]*(m[i][2]-l[2]))
	}
	return s
}
