package main

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"

	"github.com/westphae/magkal/kalman"
)

const (
	n = 2
	deg = math.Pi/180
)

var (
	kf *kalman.KalmanFilter
	x0 []float64
)

func main() {
	var (
		inp           string
		mx, my, theta float64
		err           error
	)

	x0 = make([]float64, 2*n)
	for i:=0; i<n; i++ {
		x0[2*i] = 1+0.25*(2*rand.Float64()-1)
		x0[2*i+1] = kalman.N0*0.25*(2*rand.Float64()-1)
	}

	kf = kalman.NewKalmanFilter(n)

	fmt.Println("Initial state:")
	printState()
	fmt.Println()

	for {
		fmt.Print("> ")
		fmt.Scan(&inp)

		if len(inp)==0 {
			continue
		}

		if inp[0:1]=="q" {
			fmt.Println("Exiting")
			break
		}

		theta, err = strconv.ParseFloat(inp, 64)
		if err != nil {
			continue
		}

		mx = (kalman.N0*math.Cos(theta*deg)-x0[1])/x0[0]
		my = (kalman.N0*math.Sin(theta*deg)-x0[3])/x0[2]
		fmt.Printf("Sending values (%1.3f, %1.3f)\n", mx, my)
		kf.U <- [][]float64{{mx}, {my}}
		kf.Z <- [][]float64{{kalman.N0*kalman.N0}}
		printState()
		fmt.Println()
	}
}

func printState() {
	x := kf.State()
	//fmt.Printf("X: %1.3f L: %1.3f\n", x[0][0], x[1][0])
	fmt.Printf(" X: %v\n", x)
	fmt.Printf("X0: %v\n", x0)

	p := kf.StateCovariance()
	//fmt.Printf("Cov: [%1.6f %1.6f]\n     [%1.6f %1.6f]\n", p[0][0], p[0][1], p[1][0], p[1][1])
	fmt.Printf("P: %v\n", p)
}
