package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
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

type measureCmd struct {
	M0 measurement `json:"m0"` // Raw measurement (for manual), pre-noise
}

type message struct {
	Params  *params     `json:"params"`  // if the message contains new params
	Measure *measureCmd `json:"measure"` // if the message contains a measurement command
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
	log.Println("Listening for messages from a new client")
	var (
		msg message
		p   params
		cmd measureCmd
		myMeasurer measurer
		myMeasurement measurement
	)
	for {
		if err = conn.ReadJSON(&msg); err != nil {
			log.Printf("Error reading from websocket: %s\n", err)
			break
		}

		// Extract any new parameters
		if msg.Params != nil {
			p = *msg.Params
			log.Print(p)
			switch p.Source {
			case manual:
				myMeasurer = makeManualMeasurer(p.N, p.N0, *p.KAct, *p.LAct, p.NSigma*p.N0)
			case random:
				myMeasurer = makeRandomMeasurer(p.N, p.N0, *p.KAct, *p.LAct, p.NSigma*p.N0)
			default:
				myMeasurer = nil
				log.Print("Received bad source")
				break
			}
			//continue
		}

		// Return any requested measurements
		if msg.Measure != nil {
			if myMeasurer == nil {
				log.Print("Received a measure command but haven't set a measurer")
				continue
			}
			cmd = *msg.Measure
			log.Print(cmd)
			myMeasurement = myMeasurer(cmd.M0)
		}
		// For testing: just return the params
		if err = conn.WriteJSON(myMeasurement); err != nil {
			log.Printf("Error writing to websocket: %s\n", err)
		}
	}
	log.Println("Closing client")
}
