package main

import (
	"fmt"
	"go.bug.st/serial"
	"gocv.io/x/gocv"
	"io/ioutil"
	"jrkbr/lib"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
)

var roomba *lib.Roomba
var activeTracker *lib.ColorTracker
var trackerMutex sync.Mutex

func main() {
	// Check command line arguments
	if len(os.Args) < 2 {
		fmt.Println("Usage: jrkbr <serial_port>")
		ports, err := serial.GetPortsList()
		if err != nil {
			log.Fatalf("Error getting serial ports: %v", err)
		}
		if len(ports) == 0 {
			fmt.Println("No serial ports found!")
		} else {
			fmt.Println("Available serial ports:")
			for _, port := range ports {
				fmt.Println("  " + port)
			}
		}
		os.Exit(1)
	}

	// Create a new Roomba instance
	roomba = lib.NewRoomba(os.Args[1], 115200)

	// Connect to the Roomba
	if err := roomba.Connect(); err != nil {
		log.Fatalf("Failed to connect to Roomba: %v", err)
	}
	log.Println("Connected to Roomba")
	defer roomba.Close()

	// Start the Roomba
	if err := roomba.Start(); err != nil {
		log.Fatalf("Failed to start Roomba: %v", err)
	}
	log.Println("Roomba started")

	// Set to safe mode
	if err := roomba.SafeMode(); err != nil {
		log.Fatalf("Failed to set Roomba to safe mode: %v", err)
	}
	log.Println("Roomba in safe mode")

	// Create HTTP server
	// Serve static files from the "static" directory
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Main handler for index page
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Read the static HTML file
		htmlContent, err := ioutil.ReadFile("./static/index.html")
		if err != nil {
			log.Printf("Error reading HTML file: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Set the content type
		w.Header().Set("Content-Type", "text/html")

		// Write the HTML content
		_, err = w.Write(htmlContent)
		if err != nil {
			log.Printf("Error writing response: %v", err)
		}
	})

	// Seek color handler
	http.HandleFunc("/seekColor", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse form data
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Failed to parse form data", http.StatusBadRequest)
			return
		}

		// Get color to seek - default to lime if not specified
		colorName := r.FormValue("color")
		if colorName == "" {
			colorName = "lime" // Default to lime
		}

		log.Printf("Seeking color: %s", colorName)

		// Handle the existing tracker
		trackerMutex.Lock()

		// Clean up any existing tracker
		if activeTracker != nil {
			activeTracker.Stop()
			activeTracker = nil
		}

		// Create color tracker config
		config := lib.DefaultColorTrackerConfig()

		// Set color range based on requested color
		switch colorName {
		case "red":
			// Red is special in HSV as it wraps around
			config.DetectorConfig.LowerHSVBound = gocv.NewScalar(0, 100, 100, 0)
			config.DetectorConfig.UpperHSVBound = gocv.NewScalar(10, 255, 255, 0)
		case "blue":
			config.DetectorConfig.LowerHSVBound = gocv.NewScalar(100, 100, 100, 0)
			config.DetectorConfig.UpperHSVBound = gocv.NewScalar(130, 255, 255, 0)
		case "yellow":
			config.DetectorConfig.LowerHSVBound = gocv.NewScalar(20, 100, 100, 0)
			config.DetectorConfig.UpperHSVBound = gocv.NewScalar(30, 255, 255, 0)
		case "black":
			config.DetectorConfig.LowerHSVBound = gocv.NewScalar(0, 0, 0, 0)
			config.DetectorConfig.UpperHSVBound = gocv.NewScalar(180, 255, 50, 0)
		case "lime":
			config.DetectorConfig.LowerHSVBound = gocv.NewScalar(45, 100, 100, 0)
			config.DetectorConfig.UpperHSVBound = gocv.NewScalar(65, 255, 255, 0)
		default: // Green
			config.DetectorConfig.LowerHSVBound = gocv.NewScalar(35, 100, 100, 0)
			config.DetectorConfig.UpperHSVBound = gocv.NewScalar(50, 255, 255, 0)
		}

		// Create the color tracker
		tracker, err := lib.NewColorTracker(config, roomba)
		if err != nil {
			log.Printf("Error creating color tracker: %v", err)
			trackerMutex.Unlock()
			http.Error(w, fmt.Sprintf("Error: %v", err), http.StatusInternalServerError)
			return
		}

		// Store the active tracker
		activeTracker = tracker
		trackerMutex.Unlock()

		// Start the tracker
		tracker.Start()

		response := fmt.Sprintf("Tracking %s color", colorName)
		log.Println(response)
		fmt.Fprint(w, response)
	})

	// Stop handler
	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		trackerMutex.Lock()
		if activeTracker != nil {
			activeTracker.Stop()
			activeTracker = nil
		}
		trackerMutex.Unlock()

		roomba.Stop()
		fmt.Fprint(w, "Stopped")
	})

	// Movement handler for manual control
	http.HandleFunc("/move", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse form data
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Failed to parse form data", http.StatusBadRequest)
			return
		}

		command := r.FormValue("command")
		speedStr := r.FormValue("speed")

		// Convert speed string to int16
		var speed int16 = 150 // Default speed
		if speedStr != "" {
			speedInt, err := strconv.Atoi(speedStr)
			if err == nil {
				if speedInt < 50 {
					speedInt = 50 // Minimum speed
				} else if speedInt > 300 {
					speedInt = 300 // Maximum speed
				}
				speed = int16(speedInt)
			}
		}

		// Stop any active color tracking
		trackerMutex.Lock()
		if activeTracker != nil {
			activeTracker.Stop()
			activeTracker = nil
		}
		trackerMutex.Unlock()

		// Execute the command
		var response string
		switch command {
		case "forward":
			err = roomba.Drive(speed, lib.StraightRadius)
			response = fmt.Sprintf("Moving forward at speed %d", speed)
		case "backward":
			err = roomba.Drive(-speed, lib.StraightRadius)
			response = fmt.Sprintf("Moving backward at speed %d", speed)
		case "left":
			err = roomba.Drive(speed, 1) // Counter-clockwise turn
			response = fmt.Sprintf("Turning left at speed %d", speed)
		case "right":
			err = roomba.Drive(speed, -1) // Clockwise turn
			response = fmt.Sprintf("Turning right at speed %d", speed)
		case "stop":
			err = roomba.Stop()
			response = "Stopped"
		default:
			http.Error(w, "Unknown command", http.StatusBadRequest)
			return
		}

		if err != nil {
			log.Printf("Error controlling Roomba: %v", err)
			http.Error(w, fmt.Sprintf("Error: %v", err), http.StatusInternalServerError)
			return
		}

		log.Println(response)
		fmt.Fprint(w, response)
	})

	// Start the HTTP server
	port := 8080
	log.Printf("Starting server on port %d...", port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
