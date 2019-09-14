package kalman

import (
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
