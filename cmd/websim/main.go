package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/westphae/magkal/kalman"
)

type source int // Source is where the measurements come from

const (
	manual source = iota // User sends measurements through websocket
	random // Measurements are made randomly
	file // Measurements come from a file
	actual // Measurements come from an actual MPU sensor
)

type params struct {
	Source  source    `json:"source"`   // Source of the magnetometer data
	N       int        `json:"n"`       // Number of dimensions
	N0      float64    `json:"n0"`      // Value of Earth's magnetic field at location
	KAct    *[]float64 `json:"kAct"`    // Actual K for manual, random measurement sources
	LAct    *[]float64 `json:"lAct"`    // Actual L for manual, random measurement sources
	NSigma  float64    `json:"nSigma"`  // Initial noise scale for k
	Epsilon float64    `json:"epsilon"` // Noise scale for measurement and process noise
}

// Some sensible default parameters to start the user off
var defaultParams = params{
	random,2,1.0,
	&[]float64{0.8, 0.7, 0.9}, &[]float64{0.1, 0.15, -0.1},
	0.04, 0.01,
}

type measureCmd struct {
	M0 measurement `json:"m0"` // Raw measurement (for manual), pre-noise
}

type estimateCmd struct {
	NN float64 `json:"nn"` // The actual measurement of N^2
}

type messageIn struct {
	Params   *params      `json:"params"`   // if the messageIn contains new params
	Measure  *measureCmd  `json:"measure"`  // if the messageIn contains a measurement command
	Estimate *estimateCmd `json:"estimate"` // if the messageIn contains an estimate command
}

type state struct {
	K *[]float64 `json:"k"`   // Current estimate of K
	L *[]float64 `json:"l"`   // Current estimate of L
	P *[][]float64 `json:"p"` // Current estimate of P, uncertainty matrix of state
}

type messageOut struct {
	Params      *params      `json:"params"`      // The params the server is using
	Measurement *measurement `json:"measurement"` // A raw measurement from the magnetometer source
	State       *state       `json:"state"`       // Current state of the system
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func main() {
	fs := http.FileServer(http.Dir("www"))
	http.Handle("/", fs)
	http.HandleFunc("/websocket", handleConnections)
	log.Println("Listening for connections on port 8000")
	log.Fatal(http.ListenAndServe(":8000", nil))
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to websocket: %s\n", err)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing websocket: %s\n", err.Error())
		}
	}()
	log.Println("A client opened a connection")

	// Handle ping-pong
	var timeout *time.Timer
	go func() {
		pingTime := time.NewTicker(5 * time.Second)
		for {
			if err = conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(10*time.Second)); err != nil {
				log.Printf("ws error sending ping: %s\n", err)
				break
			}
			timeout = time.NewTimer(4 * time.Second)

			select {
			case <-pingTime.C:
				timeout.Stop()
			case <-timeout.C:
				log.Println("ping timeout")
				pingTime.Stop()
				if err = conn.Close(); err != nil {
					log.Printf("Error closing connection: %s\n", err)
				}
				break
			}
		}
		log.Println("Stopping ping")
	}()
	conn.SetPongHandler(func(appData string) error {
		timeout.Stop()
		return nil
	})

	// Read messages from the client and receive control messages
	/*
	1. Receive websocket message, determine type
	2. If type is params:
	   a. stop/close any current sim (send a nil measurement)
	   b. initialize a new sim
	   c. send reset command to client
	3. If type is measure:
	   a. create a measurement according to source
	   b. send to filter
	   c. send result to client
	 */
	var (
		msgIn         messageIn
		msgOut        messageOut
		p             params
		cmd           measureCmd
		myMeasurer    measurer
		myMeasurement measurement
		myEstimator   *kalman.Filter
	)
	p = defaultParams

	// Send initial params
	msgOut = messageOut{&p, nil, nil, nil}
	if err = conn.WriteJSON(msgOut); err != nil {
		log.Printf("Error writing params to websocket: %s\n", err)
		return
	}
	msgOut.Params = nil

	log.Println("Listening for messages from a new client")
	for {
		if err = conn.ReadJSON(&msgIn); err != nil {
			log.Printf("Error reading from websocket: %s\n", err)
			break
		}
		timeout.Stop() // Stop the timeout if we get a message, not just a pong

		// Extract any new parameters
		if msgIn.Params != nil {
			// Parse and implement new params
			p = *msgIn.Params
			myMeasurer = nil
			log.Print(p)
			msgIn.Params = nil
			msgOut.Params = &p
			log.Printf("Sending params %v\n", p)
		}

		// Return any requested measurements
		if msgIn.Measure != nil {
			if myMeasurer == nil {
				switch p.Source {
				case manual:
					myMeasurer = makeManualMeasurer(p.N, p.N0, *p.KAct, *p.LAct, p.NSigma*p.N0)
					log.Println("Set Manual measurer")
				case random:
					myMeasurer = makeRandomMeasurer(p.N, p.N0, *p.KAct, *p.LAct, p.NSigma*p.N0)
					log.Println("Set Random measurer")
				default:
					myMeasurer = nil
					log.Printf("Received bad source: %d\n", p.Source)
					break
				}
			}
			cmd = *msgIn.Measure
			msgIn.Measure = nil
			log.Printf("Received raw measurement %v\n", cmd)

			myMeasurement = myMeasurer(cmd.M0)
			msgOut.Measurement = &myMeasurement
			log.Printf("Sending measurement %v\n", myMeasurement)
		}

		// Perform any state estimations with the Kalman Filter
		if msgIn.Estimate != nil {
			if myEstimator == nil {
				myEstimator = kalman.NewKalmanFilter(p.N, p.N0, p.NSigma, p.Epsilon)
			}
			nn := msgIn.Estimate.NN
			msgIn.Estimate = nil
			log.Printf("Estimating: %3.1f\n", nn)

			msgOut.State = &state{
				K: &[]float64{0.987, 0.765, 0.543},
				L: &[]float64{0.123, 0.234, -0.345},
				P: &[][]float64{
					{0.9, 0.1, -0.2},
					{0.1, 0.7, 0.4},
					{-0.2, 0.4, 0.8},
				},
			}
			log.Printf("Sending state %v\n", msgOut.State)
		}

		// Return message and clean up
		if err = conn.WriteJSON(msgOut); err != nil {
			log.Printf("Error writing to websocket: %s\n", err)
		}
		msgOut.Params = nil
		msgOut.Measurement = nil
		msgOut.State = nil
	}
	log.Println("Closing client")
}
