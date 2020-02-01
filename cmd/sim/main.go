package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"

	"github.com/westphae/magkal/kalman"
)

const (
	deg     = math.Pi / 180
	N0      = 1.0  // Strength of Earth's magnetic field at current location
	NSigma  = 0.1  // Initial uncertainty scale
	Epsilon = 1e-2 // Some tiny noise scale
)

var (
	kf *kalman.Filter
	x0 []float64
)

func main() {
	var (
		inp         string
		i, n        int
		mx, my, psi float64
		psis        []float64
		err         error
	)

	nStr := os.Args
	if len(nStr) == 1 {
		n = 2
	} else if n, err = strconv.Atoi(nStr[1]); err != nil {
		n = 2
	}
	fmt.Printf("n = %d\n", n)

	x0 = make([]float64, 2*n)
	for i := 0; i < n; i++ {
		x0[2*i] = 1 + 0.25*(2*rand.Float64()-1)
		x0[2*i+1] = N0 * 0.25 * (2*rand.Float64() - 1)
	}

	kf = kalman.NewKalmanFilter(n, N0, NSigma, Epsilon)

	fmt.Println("Initial state:")
	printState()
	fmt.Println()

Loop:
	for {
		fmt.Print("> ")
		_, _ = fmt.Scan(&inp)

		switch {
		case len(inp) == 0:
			continue Loop
		case inp[0:1] == "q":
			fmt.Println("Exiting")
			break Loop
		case inp[0:1] == "a" && n == 2:
			psis = make([]float64, 0, 360)
			for i = 0; i < 360; i += 30 {
				psis = append(psis, float64(i))
			}
		case inp[0:1] == "a" && n == 1:
			psis = []float64{0, 180}
		default:
			psi, err = strconv.ParseFloat(inp, 64)
			if err != nil {
				continue Loop
			}
			if psi > 0 {
				psi = 180
			}
			psis = []float64{psi}
		}

		for i, psi = range psis {
			fmt.Printf("Psi: %1.1f\n", psi)
			mx = (N0*math.Cos(psi*deg) - x0[1]) / x0[0]
			mx += Epsilon * math.Sqrt(2) * rand.NormFloat64()
			if n == 1 {
				fmt.Printf("Sending values (%1.3f)\n", mx)
				kf.U <- [][]float64{{mx}}
			} else {
				my = (N0*math.Sin(psi*deg) - x0[3]) / x0[2]
				my += Epsilon * math.Sqrt(2) * rand.NormFloat64()
				fmt.Printf("Sending values (%1.3f, %1.3f)\n", mx, my)
				kf.U <- [][]float64{{mx}, {my}}
			}
			kf.Z <- [][]float64{{N0 * N0}}
			printState()
			fmt.Println()
		}
	}
}

func printState() {
	x := kf.State()
	fmt.Printf(" X: %v\n", x)
	fmt.Printf("X0: %v\n", x0)

	p := kf.StateCovariance()
	fmt.Printf("P: %v\n", p)
}
