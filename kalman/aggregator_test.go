package kalman

import (
	"fmt"
	"math"
	"testing"
)

const eps = 1e-6

func testDiff(name string, actual, expected float64, eps float64, t *testing.T) {
	if actual - expected > -eps && actual - expected < eps {
		t.Logf("%s correct: expected %8.4f, got %8.4f", name, expected, actual)
		return
	}
	t.Errorf("%s incorrect: expected %8.4f, got %8.4f", name, expected, actual)
}

func TestPointAggregation(t *testing.T) {
	ms := [][]float64{[]float64{-1,2.9,-11.5}, []float64{0,3,-12}, []float64{1,3.1,-11}}
	mavgs := []float64{0, 3, -11.5}

	a := MeasureAgg{}
	mm := 0.0
	ss := 0.0
	for i:=0; i<len(ms); i++ {
		a.Add(ms[i][0], ms[i][1], ms[i][2])
		mm += ms[i][0]*ms[i][0]+ms[i][1]*ms[i][1]+ms[i][2]*ms[i][2]
		ss += ms[i][0]*ms[i][0]+ms[i][1]*ms[i][1]+ms[i][2]*ms[i][2]
	}
	mm = mm/float64(len(ms))
	ss = ss/float64(len(ms))-(mavgs[0]*mavgs[0]+mavgs[1]*mavgs[1]+mavgs[2]*mavgs[2])
	testDiff("N", float64(a.N), float64(len(ms)), eps, t)
	testDiff("M0", a.M0, mavgs[0], eps, t)
	testDiff("M1", a.M1, mavgs[1], eps, t)
	testDiff("M2", a.M2, mavgs[2], eps, t)
	testDiff("MM", a.MM, mm, eps, t)
	testDiff("SS", a.SS, ss, eps, t)
}

func TestGridShape(t *testing.T) {
	var (
		theta0, dTheta float64
		phi0, dPhi     float64
	)
	ns := []int{6}
	for _, nTheta := range ns {
		g := NewMeasureGrid(nTheta)
		if len(g.Thetas)!=nTheta {
			t.Errorf("Theta lengths don't match: %d should be %d", len(g.Thetas), nTheta)
		}
		for i, theta := range g.Thetas {
			if i>1 {
				testDiff("dTheta", theta-theta0, dTheta, eps, t)
			}
			for j, phi := range g.Phis[i] {
				if j>1 {
					testDiff("dPhi", phi-phi0, dPhi, eps, t)
				}
				dPhi = phi-phi0
				phi0 = phi
			}
			dTheta = theta-theta0
			theta0 = theta
		}
	}
}

func TestGridAreas(t *testing.T) {
	ns := []int{6}

	var (
		theta0, theta1 float64
		phi0, phi1     float64
		nPatches       int
	)

	for _, nTheta := range ns {
		g := NewMeasureGrid(nTheta)
		for i:=0; i<nTheta; i++ {
			nPatches += len(g.Phis[i])
		}
		patchSize := 8*math.Pi*math.Pi/float64(nPatches)

		for i, theta := range g.Thetas {
			if i==0 {
				theta1 = math.Pi/2
			} else {
				theta1 = 0.5*theta+0.5*g.Thetas[i-1]
			}
			if i==nTheta-1 {
				theta0 = -math.Pi/2
			} else {
				theta0 = 0.5*theta+0.5*g.Thetas[i+1]
			}

			for j, phi := range g.Phis[i] {
				if j==0 {
					phi0 = 0.5*phi+0.5*(g.Phis[i][len(g.Phis[i])-1]-2*math.Pi)
				} else {
					phi0 = 0.5*phi+0.5*g.Phis[i][j-1]
				}
				if j==len(g.Phis[i])-1 {
					phi1 = 0.5*phi+0.5*(g.Phis[i][0]+2*math.Pi)
				} else {
					phi1 = 0.5*phi+0.5*g.Phis[i][j+1]
				}

				testDiff(
					fmt.Sprintf("Area(%d,%d)", i, j),
					2*math.Pi*(math.Sin(theta1)-math.Sin(theta0))*(phi1-phi0),
					patchSize,
					0.1*patchSize, t)
			}
		}
	}
}

/*
type MeasureGrid struct {
	measurements [][]MeasureAgg
	Thetas       []float64
	Phis         [][]float64
	N            int
}
*/
