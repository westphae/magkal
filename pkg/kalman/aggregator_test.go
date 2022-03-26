package kalman

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

const eps = 1e-6

func testDiff(name string, actual, expected float64, eps float64, t *testing.T) {
	if actual-expected > -eps && actual-expected < eps {
		t.Logf("%s correct: expected %8.4f, got %8.4f", name, expected, actual)
		return
	}
	t.Errorf("%s incorrect: expected %8.4f, got %8.4f", name, expected, actual)
}

func TestPointAggregation(t *testing.T) {
	ms := [][]float64{{-1, 2.9, -11.5}, {0, 3, -12}, {1, 3.1, -11}}
	mavgs := []float64{0, 3, -11.5}

	a := MeasureAgg{}
	mm := 0.0
	ss := 0.0
	for i := 0; i < len(ms); i++ {
		a.Add(ms[i][0], ms[i][1], ms[i][2])
		mm += ms[i][0]*ms[i][0] + ms[i][1]*ms[i][1] + ms[i][2]*ms[i][2]
		ss += ms[i][0]*ms[i][0] + ms[i][1]*ms[i][1] + ms[i][2]*ms[i][2]
	}
	mm = mm / float64(len(ms))
	ss = ss/float64(len(ms)) - (mavgs[0]*mavgs[0] + mavgs[1]*mavgs[1] + mavgs[2]*mavgs[2])
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
		size           int
	)
	ns := []int{6}
	for _, nTheta := range ns {
		g := NewMeasureGrid(nTheta)
		if len(g.Thetas) != nTheta {
			t.Errorf("Theta lengths don't match: %d should be %d", len(g.Thetas), nTheta)
		}
		for i, theta := range g.Thetas {
			if i > 1 {
				testDiff("dTheta", theta-theta0, dTheta, eps, t)
			}
			for j, phi := range g.Phis[i] {
				if j > 1 {
					testDiff("dPhi", phi-phi0, dPhi, eps, t)
				}
				dPhi = phi - phi0
				phi0 = phi
			}
			dTheta = theta - theta0
			theta0 = theta
			size += len(g.Phis[i])
		}
		testDiff("size", float64(size), float64(g.Size), eps, t)
	}
}

func TestGridAreas(t *testing.T) {
	ns := []int{2, 4, 6}
	//ns := []int{2, 3, 4, 5, 6, 12}

	var (
		theta0, theta1 float64
		phi0, phi1     float64
		nPatches       int
	)

	for _, nTheta := range ns {
		g := NewMeasureGrid(nTheta)
		nPatches = 0
		for i := 0; i < len(g.Phis); i++ {
			nPatches += len(g.Phis[i])
		}
		patchSize := 4 * math.Pi / float64(nPatches)

		for i, theta := range g.Thetas {
			if i == 0 {
				theta1 = math.Pi / 2
			} else {
				theta1 = 0.5*theta + 0.5*g.Thetas[i-1]
			}
			if i == nTheta-1 {
				theta0 = -math.Pi / 2
			} else {
				theta0 = 0.5*theta + 0.5*g.Thetas[i+1]
			}

			for j, phi := range g.Phis[i] {
				if j == 0 {
					phi0 = 0.5*phi + 0.5*(g.Phis[i][len(g.Phis[i])-1]-2*math.Pi)
				} else {
					phi0 = 0.5*phi + 0.5*g.Phis[i][j-1]
				}
				if j == len(g.Phis[i])-1 {
					phi1 = 0.5*phi + 0.5*(g.Phis[i][0]+2*math.Pi)
				} else {
					phi1 = 0.5*phi + 0.5*g.Phis[i][j+1]
				}

				testDiff(
					fmt.Sprintf("Area(%d,%d)", i, j),
					(math.Sin(theta1)-math.Sin(theta0))*(phi1-phi0),
					patchSize,
					0.25*patchSize, t)
			}
		}
	}
}

func TestGridNsRandom(t *testing.T) {
	r := rand.New(rand.NewSource(99))
	for j := 0; j < 1; j++ {
		nTheta := r.Intn(100)
		g := NewMeasureGrid(nTheta)
		ns := make([][]int, nTheta)
		for i := 0; i < nTheta; i++ {
			ns[i] = make([]int, len(g.Phis[i]))
		}
		for i := 0; i < r.Intn(1000); i++ {
			ti := r.Intn(len(g.Thetas))
			si := r.Intn(len(g.Phis[ti]))
			m0 := math.Cos(g.Thetas[ti]) * math.Cos(g.Phis[ti][si])
			m1 := math.Cos(g.Thetas[ti]) * math.Sin(g.Phis[ti][si])
			m2 := math.Sin(g.Thetas[ti])
			g.Add(m0, m1, m2)
			ns[ti][si] += 1
		}
		c := g.Ns()
		for i, d := range c {
			for j := range d {
				testDiff(fmt.Sprintf("(%d,%d)", i, j), float64(c[i][j]), float64(ns[i][j]), 0.5, t)
			}
		}
	}
}

func makeTestGrid() (g MeasureGrid, ns [][]int, avgs [][][]float64, stds [][]float64) {
	ms := [][]float64{{11000, 18000, 9500}, {10200, 17100, 8001}}
	dTheta := math.Pi / 3

	g = NewMeasureGrid(len(ns))
	ns = [][]int{{12, 8, 16}, {22, 6, 14}}
	avgs = make([][][]float64, len(ns))
	stds = make([][]float64, len(ns))
	var m0, m1, m2, jj, s float64
	for i := 0; i < 2; i++ {
		ns[i] = make([]int, len(ns[i]))
		avgs[i] = make([][]float64, len(ns[i]))
		stds[i] = make([]float64, len(ns[i]))
		for j := 0; j < 3; j++ {
			jj = float64(2*j + 1)
			m0 = ms[i][j] * math.Cos(jj*dTheta) * math.Cos(dTheta)
			m1 = ms[i][j] * math.Cos(jj*dTheta) * math.Cos(dTheta)
			m2 = ms[i][j] * math.Cos(jj*dTheta) * math.Cos(dTheta)
			s = 1
			for k := 0; k < ns[i][j]; k++ {
				g.Add((1+0.1*s)*m0, (1+0.1*s)*m1, (1+0.1*s)*m2)
				s = -s
			}
			avgs[i][j] = []float64{m0, m1, m2}
			stds[i][j] = 0.1 * ms[i][j]
		}
	}

	return g, ns, avgs, stds
}

func TestGridNs(t *testing.T) {
	g, ns, _, _ := makeTestGrid()
	for i, f := range g.Ns() {
		for j, a := range f {
			testDiff(fmt.Sprintf("(%d,%d) N", i, j), float64(a), float64(ns[i][j]), eps, t)
		}
	}
}

func TestGridAverages(t *testing.T) {
	g, _, avgs, _ := makeTestGrid()
	for i, f := range g.Averages() {
		for j, a := range f {
			testDiff(fmt.Sprintf("(%d,%d) Average m0", i, j), a[0], avgs[i][j][0], eps, t)
			testDiff(fmt.Sprintf("(%d,%d) Average m1", i, j), a[1], avgs[i][j][1], eps, t)
			testDiff(fmt.Sprintf("(%d,%d) Average m2", i, j), a[2], avgs[i][j][2], eps, t)
		}
	}
}

func TestGridStDevs(t *testing.T) {
	g, _, _, stds := makeTestGrid()
	for i, f := range g.StDevs() {
		for j, a := range f {
			testDiff(fmt.Sprintf("(%d,%d) Std", i, j), a, stds[i][j], eps, t)
		}
	}
}

func TestGridCalculatedFieldStrength2s(t *testing.T) {
	k := []float64{0.8, 0.6, 0.7}
	l := []float64{4180, -2660, 250}
	nn := 40000.0
	g := NewMeasureGrid(2)
	var n0, n1, n2 float64
	for i, theta := range g.Thetas {
		for _, phi := range g.Phis[i] {
			n0 = nn * math.Cos(theta) * math.Cos(phi)
			n1 = nn * math.Cos(theta) * math.Sin(phi)
			n2 = nn * math.Sin(theta)
			g.Add(n0/k[0]+l[0], n1/k[1]+l[1], n2/k[2]+l[2])
		}
	}
	for i, e := range g.CalculatedFieldStrength2(k, l) {
		for j, f := range e {
			testDiff(fmt.Sprintf("(%d,%d) FieldStrength**2", i, j), f, nn*nn, eps, t)
		}
	}
}

func TestGridCalculatedFieldStrengths(t *testing.T) {
	k := []float64{0.8, 0.6, 0.7}
	l := []float64{4180, -2660, 250}
	nn := 40000.0
	g := NewMeasureGrid(2)
	n := make([][][]float64, len(g.Phis))
	for i, theta := range g.Thetas {
		n[i] = make([][]float64, len(g.Phis[i]))
		for j, phi := range g.Phis[i] {
			n[i][j] = []float64{
				nn * math.Cos(theta) * math.Cos(phi),
				nn * math.Cos(theta) * math.Sin(phi),
				nn * math.Sin(theta),
			}
			g.Add(n[i][j][0]/k[0]+l[0], n[i][j][1]/k[1]+l[1], n[i][j][2]/k[2]+l[2])
		}
	}
	for i, e := range g.CalculatedField(k, l) {
		for j, f := range e {
			for l := 0; l < 3; l++ {
				testDiff(fmt.Sprintf("(%d,%d) Field%d", i, j, l), f[l], n[i][j][l], eps, t)
			}
		}
	}
}
