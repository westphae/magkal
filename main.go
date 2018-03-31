package main

import (
	"fmt"

	"github.com/westphae/magkal/kalman"
)

const (
	k = 0.9
	l = -0.25*kalman.N0
)

var (
	kf *kalman.KalmanFilter
)

func main() {
	var (
		inp string
		m   float64
	)

	kf = kalman.NewKalmanFilter()

	fmt.Println("Initial state:")
	printState()
	fmt.Println()

	Loop:
		for {
			fmt.Print("> ")
			fmt.Scan(&inp)
			fmt.Printf("Input was %s\n", inp)

			if len(inp)==0 {
				continue Loop
			}

			inp = inp[0:1]
			switch inp {
			case "q":
				fmt.Println("Exiting")
				break Loop
			case "j":
				m = (-kalman.N0-l)/k
				fmt.Printf("Sending value %1.3f\n", m)
			case "k":
				m = (+kalman.N0-l)/k
				fmt.Printf("Sending value %1.3f\n", m)
			}
			kf.U <- m
			kf.Z <- kalman.N0*kalman.N0
			printState()
			fmt.Println()
		}
}

func printState() {
	x := kf.State()
	fmt.Printf("K: %1.3f L: %1.3f\n", x[0], x[1])

	p := kf.StateCovariance()
	fmt.Printf("Cov: [%1.6f %1.6f]\n     [%1.6f %1.6f]\n", p[0][0], p[0][1], p[1][0], p[1][1])
}
