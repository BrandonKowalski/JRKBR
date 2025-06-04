package main

import (
	"fmt"
	"html/template"
	"jrkbr/lib"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.bug.st/serial"
	"gocv.io/x/gocv"
)

const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>Roomba Control</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 800px;
            margin: 0 auto;
            padding: 20px;
        }
        h1 {
            color: #333;
            text-align: center;
        }
        .control-panel {
            display: flex;
            flex-direction: column;
            align-items: center;
            margin-top: 20px;
        }
        .button-row {
            display: flex;
            justify-content: center;
            margin: 10px 0;
        }
        button {
            padding: 15px 25px;
            margin: 5px;
            font-size: 16px;
            cursor: pointer;
            background-color: #4CAF50;
            color: white;
            border: none;
            border-radius: 5px;
        }
        button:hover {
            background-color: #45a049;
        }
        .stop-button {
            background-color: #f44336;
        }
        .stop-button:hover {
            background-color: #d32f2f;
        }
        .function-buttons {
            margin-top: 30px;
            display: flex;
            flex-wrap: wrap;
            justify-content: center;
        }
        .function-button {
            background-color: #2196F3;
        }
        .function-button:hover {
            background-color: #0b7dda;
        }
        .status {
            margin-top: 20px;
            padding: 10px;
            border: 1px solid #ddd;
            border-radius: 5px;
            text-align: center;
        }
        .speed-control {
            margin-top: 20px;
            width: 100%;
            max-width: 300px;
            display: flex;
            flex-direction: column;
            align-items: center;
        }
        input[type="range"] {
            width: 100%;
            margin: 10px 0;
        }
        .color-select {
            margin-top: 10px;
            display: flex;
            flex-direction: column;
            align-items: center;
        }
        .color-select select {
            margin: 10px 0;
            padding: 8px;
            font-size: 16px;
        }
    </style>
</head>
<body>
    <h1>Roomba Control Panel</h1>
    
    <div class="status" id="status">Status: Connected</div>
    
    <div class="speed-control">
        <label for="speed">Speed: <span id="speedValue">200</span> mm/s</label>
        <input type="range" id="speed" name="speed" min="50" max="500" value="200" oninput="updateSpeedValue(this.value)">
    </div>
    
    <div class="control-panel">
        <div class="button-row">
            <button id="forwardBtn">Forward</button>
        </div>
        <div class="button-row">
            <button id="leftBtn">Left</button>
            <button class="stop-button" id="stopBtn">STOP</button>
            <button id="rightBtn">Right</button>
        </div>
        <div class="button-row">
            <button id="backwardBtn">Backward</button>
        </div>
    </div>
    
    <div class="function-buttons">
        <button class="function-button" id="cleanBtn">Clean</button>
        <button class="function-button" id="spotBtn">Spot Clean</button>
        <button class="function-button" id="maxBtn">Max Clean</button>
        <button class="function-button" id="dockBtn">Dock</button>
        <button class="function-button" id="powerBtn">Power Off</button>
    </div>
    
    <div class="color-select">
        <label for="colorSelect">Select Color to Track:</label>
        <select id="colorSelect">
            <option value="green">Green</option>
            <option value="red">Red</option>
            <option value="blue">Blue</option>
            <option value="yellow">Yellow</option>
        </select>
        <button class="function-button" id="seekColorBtn">Seek & Follow Color</button>
    </div>
    
    <script>
        function updateSpeedValue(val) {
            document.getElementById('speedValue').textContent = val;
        }
        
        function sendCommand(command) {
            const speed = document.getElementById('speed').value;
            const statusElement = document.getElementById('status');
            
            let url = '/control';
            let body = 'command=' + command + '&speed=' + speed;
            
            // Add color parameter for seekColor command
            if (command === 'seekColor') {
                const color = document.getElementById('colorSelect').value;
                body += '&color=' + color;
                url = '/seekColor';
            }
            
            fetch(url, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/x-www-form-urlencoded',
                },
                body: body
            })
            .then(response => response.text())
            .then(data => {
                statusElement.textContent = 'Status: ' + data;
            })
            .catch(error => {
                statusElement.textContent = 'Status: Error - ' + error;
            });
        }
        
        // Set up event listeners for all buttons
        document.addEventListener('DOMContentLoaded', function() {
            document.getElementById('forwardBtn').addEventListener('click', function() {
                sendCommand('forward');
            });
            
            document.getElementById('backwardBtn').addEventListener('click', function() {
                sendCommand('backward');
            });
            
            document.getElementById('leftBtn').addEventListener('click', function() {
                sendCommand('left');
            });
            
            document.getElementById('rightBtn').addEventListener('click', function() {
                sendCommand('right');
            });
            
            document.getElementById('stopBtn').addEventListener('click', function() {
                sendCommand('stop');
            });
            
            document.getElementById('cleanBtn').addEventListener('click', function() {
                sendCommand('clean');
            });
            
            document.getElementById('spotBtn').addEventListener('click', function() {
                sendCommand('spot');
            });
            
            document.getElementById('maxBtn').addEventListener('click', function() {
                sendCommand('max');
            });
            
            document.getElementById('dockBtn').addEventListener('click', function() {
                sendCommand('dock');
            });
            
            document.getElementById('powerBtn').addEventListener('click', function() {
                sendCommand('power');
            });
            
            document.getElementById('seekColorBtn').addEventListener('click', function() {
                sendCommand('seekColor');
            });
        });
    </script>
</body>
</html>
`

// ColorSeekState represents the current state of the color seeking process
type ColorSeekState string

const (
	Spinning  ColorSeekState = "SPINNING"  // Looking for color by spinning
	Centering ColorSeekState = "CENTERING" // Centering the detected color
	Following ColorSeekState = "FOLLOWING" // Following the color
	Stopped   ColorSeekState = "STOPPED"   // Stopped (color lost or finished)
)

// ColorSeekConfig holds configuration for color seeking behavior
type ColorSeekConfig struct {
	// Speed settings
	SpinSpeed       int16
	ForwardSpeed    int16
	AdjustmentSpeed int16

	// Timing settings
	UpdateInterval   time.Duration
	LostColorTimeout time.Duration

	// Color detection settings
	DetectorConfig lib.ColorDetectionConfig
}

// DefaultColorSeekConfig returns a default configuration for color seeking
func DefaultColorSeekConfig() ColorSeekConfig {
	return ColorSeekConfig{
		SpinSpeed:        100, // Speed for spinning to find color
		ForwardSpeed:     150, // Speed for moving forward
		AdjustmentSpeed:  80,  // Speed for adjusting position
		UpdateInterval:   50 * time.Millisecond,
		LostColorTimeout: 2 * time.Second,
		DetectorConfig:   lib.DefaultColorDetectionConfig(),
	}
}

// ColorSeeker manages the behavior of seeking and following a specific color
type ColorSeeker struct {
	config        ColorSeekConfig
	colorDetector *lib.ColorDetector
	roomba        *lib.Roomba
	running       bool
	stopChan      chan struct{}
	state         ColorSeekState
	lastColorSeen time.Time
	mu            sync.RWMutex
}

// NewColorSeeker creates a new color seeker with the given configuration
func NewColorSeeker(config ColorSeekConfig, roomba *lib.Roomba) (*ColorSeeker, error) {
	detector, err := lib.NewColorDetector(config.DetectorConfig)
	if err != nil {
		return nil, err
	}

	return &ColorSeeker{
		config:        config,
		colorDetector: detector,
		roomba:        roomba,
		running:       false,
		stopChan:      make(chan struct{}),
		state:         Stopped,
		lastColorSeen: time.Time{},
	}, nil
}

// Start begins the color seeking behavior
func (cs *ColorSeeker) Start() {
	cs.mu.Lock()
	if cs.running {
		cs.mu.Unlock()
		return
	}
	cs.running = true
	cs.mu.Unlock()

	// Start the color detector
	cs.colorDetector.Start()

	// Start in spinning state
	cs.state = Spinning
	cs.roomba.Drive(cs.config.SpinSpeed, -1) // Start spinning clockwise

	// Start the control loop
	go cs.controlLoop()
}

// Stop halts the color seeking behavior
func (cs *ColorSeeker) Stop() {
	cs.mu.Lock()
	if !cs.running {
		cs.mu.Unlock()
		return
	}
	cs.running = false
	cs.mu.Unlock()

	close(cs.stopChan)

	// Stop the color detector
	cs.colorDetector.Stop()

	// Stop the Roomba
	if cs.roomba != nil {
		cs.roomba.Stop()
	}

	cs.state = Stopped
}

// Close releases all resources
func (cs *ColorSeeker) Close() {
	cs.Stop()
	cs.colorDetector.Close()
}

// GetColorDetector returns the underlying color detector
func (cs *ColorSeeker) GetColorDetector() *lib.ColorDetector {
	return cs.colorDetector
}

// GetCurrentState returns the current state of the color seeker
func (cs *ColorSeeker) GetCurrentState() ColorSeekState {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.state
}

// SetColorRange allows changing the color being detected
func (cs *ColorSeeker) SetColorRange(lowerHSV, upperHSV gocv.Scalar) {
	cs.colorDetector.Config.LowerHSVBound = lowerHSV
	cs.colorDetector.Config.UpperHSVBound = upperHSV
}

// controlLoop is the main control loop for color seeking
func (cs *ColorSeeker) controlLoop() {
	ticker := time.NewTicker(cs.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cs.stopChan:
			return
		case <-ticker.C:
			position := cs.colorDetector.GetPosition()
			cs.updateMovement(position)
		}
	}
}

// updateMovement controls the Roomba based on the detected color position
func (cs *ColorSeeker) updateMovement(position lib.LinePosition) {
	var err error
	var newState ColorSeekState

	cs.mu.RLock()
	currentState := cs.state
	cs.mu.RUnlock()

	switch position {
	case lib.LineNotFound:
		// Color not found
		if currentState == Following || currentState == Centering {
			// We were following but lost the color
			if time.Since(cs.lastColorSeen) > cs.config.LostColorTimeout {
				// If we've lost the color for too long, stop
				log.Println("Color lost for too long - Stopping")
				err = cs.roomba.Stop()
				newState = Stopped
			} else {
				// Keep the current state for a bit hoping to find the color again
				newState = currentState
			}
		} else if currentState == Spinning {
			// Keep spinning to find the color
			err = cs.roomba.Drive(cs.config.SpinSpeed, -1) // Continue spinning clockwise
			log.Println("Spinning to find color")
			newState = Spinning
		} else {
			newState = currentState
		}
	case lib.LineCentered:
		// Color is centered
		cs.lastColorSeen = time.Now()

		if currentState == Spinning || currentState == Centering {
			// Transition to following state
			newState = Following
			log.Println("Color CENTERED - Moving forward")
		} else {
			newState = currentState
		}

		// Move forward with the color centered
		err = cs.roomba.Drive(cs.config.ForwardSpeed, lib.StraightRadius)

	case lib.LineLeft:
		// Color is to the left
		cs.lastColorSeen = time.Now()

		if currentState == Spinning {
			// Found the color, transition to centering
			newState = Centering
		} else {
			newState = currentState
		}

		// Turn left to center the color
		err = cs.roomba.Drive(cs.config.AdjustmentSpeed, 1) // Turn counter-clockwise
		log.Println("Color LEFT - Turning left to center")

	case lib.LineRight:
		// Color is to the right
		cs.lastColorSeen = time.Now()

		if currentState == Spinning {
			// Found the color, transition to centering
			newState = Centering
		} else {
			newState = currentState
		}

		// Turn right to center the color
		err = cs.roomba.Drive(cs.config.AdjustmentSpeed, -1) // Turn clockwise
		log.Println("Color RIGHT - Turning right to center")
	}

	if err != nil {
		log.Printf("Error controlling Roomba: %v", err)
	}

	// Update the state if it changed
	if newState != "" && newState != currentState {
		cs.mu.Lock()
		cs.state = newState
		cs.mu.Unlock()
	}
}

// Global variable to store the active color seeker
var activeSeeker *ColorSeeker
var seekerMutex sync.Mutex

func main() {
	ports, err := serial.GetPortsList()
	if err != nil {
		log.Fatalf("Error getting serial ports: %v", err)
	}

	fmt.Println("Available serial ports:")
	for _, port := range ports {
		fmt.Println("  ", port)
	}

	portName := "/dev/ttyUSB0"

	roomba := lib.NewRoomba(portName, 115200)

	err = roomba.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to Roomba: %v", err)
	}

	fmt.Println("Connected to Roomba")

	defer roomba.Close()

	err = roomba.Start()
	if err != nil {
		log.Fatalf("Failed to start Roomba: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	err = roomba.Control()
	if err != nil {
		log.Fatalf("Failed to put Roomba in control mode: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	err = roomba.FullMode()
	if err != nil {
		log.Fatalf("Failed to put Roomba in full mode: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	tmpl, err := template.New("roombaControl").Parse(htmlTemplate)
	if err != nil {
		log.Fatalf("Failed to parse template: %v", err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		err := tmpl.Execute(w, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/control", func(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("Received command: %s", command)

		speedStr := r.FormValue("speed")
		speed, err := strconv.ParseInt(speedStr, 10, 16)
		if err != nil {
			log.Printf("Error parsing speed: %v, using default", err)
			speed = 200 // Default speed
		}

		speedInt16 := int16(speed)
		var response string

		switch command {
		case "forward":
			log.Printf("Executing forward command with speed %d", speedInt16)
			err = roomba.Drive(speedInt16, lib.StraightRadius)
			response = "Moving forward"
		case "backward":
			log.Printf("Executing backward command with speed %d", speedInt16)
			err = roomba.Drive(-speedInt16, lib.StraightRadius)
			response = "Moving backward"
		case "left":
			log.Printf("Executing left command with speed %d", speedInt16)
			err = roomba.Drive(speedInt16, 1) // Rotate counter-clockwise
			response = "Turning left"
		case "right":
			log.Printf("Executing right command with speed %d", speedInt16)
			err = roomba.Drive(speedInt16, -1) // Rotate clockwise
			response = "Turning right"
		case "stop":
			log.Printf("Executing stop command")
			err = roomba.Stop()
			response = "Stopped"

			// Also stop any active color seeker
			seekerMutex.Lock()
			if activeSeeker != nil {
				activeSeeker.Stop()
				activeSeeker = nil
			}
			seekerMutex.Unlock()
		case "clean":
			log.Printf("Executing clean command")
			err = roomba.Clean()
			response = "Started cleaning"
		case "spot":
			log.Printf("Executing spot clean command")
			err = roomba.SpotClean()
			response = "Started spot cleaning"
		case "max":
			log.Printf("Executing max clean command")
			err = roomba.MaxClean()
			response = "Started max cleaning"
		case "dock":
			log.Printf("Executing dock command")
			err = roomba.Dock()
			response = "Returning to dock"
		case "power":
			log.Printf("Executing power off command")
			err = roomba.PowerOff()
			response = "Powering off"
		default:
			errorMsg := fmt.Sprintf("Unknown command: %s", command)
			log.Println(errorMsg)
			http.Error(w, errorMsg, http.StatusBadRequest)
			return
		}

		if err != nil {
			log.Printf("Error executing command %s: %v", command, err)
			http.Error(w, fmt.Sprintf("Error: %v", err), http.StatusInternalServerError)
			return
		}

		log.Printf("Command executed successfully: %s", response)
		fmt.Fprint(w, response)
	})

	// Add handler for color seeking
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

		// Get color to seek - default to green if not specified
		colorName := r.FormValue("color")
		if colorName == "" {
			colorName = "green"
		}

		log.Printf("Seeking color: %s", colorName)

		// Stop any existing color seeker
		seekerMutex.Lock()
		if activeSeeker != nil {
			activeSeeker.Stop()
			activeSeeker = nil
		}

		// Create color seeker config
		config := DefaultColorSeekConfig()

		// Set color range based on requested color
		switch colorName {
		case "red":
			// Red is special in HSV as it wraps around
			// Lower range (0-10)
			config.DetectorConfig.LowerHSVBound = gocv.NewScalar(0, 100, 100, 0)
			config.DetectorConfig.UpperHSVBound = gocv.NewScalar(10, 255, 255, 0)
		case "blue":
			config.DetectorConfig.LowerHSVBound = gocv.NewScalar(100, 100, 100, 0)
			config.DetectorConfig.UpperHSVBound = gocv.NewScalar(130, 255, 255, 0)
		case "yellow":
			config.DetectorConfig.LowerHSVBound = gocv.NewScalar(20, 100, 100, 0)
			config.DetectorConfig.UpperHSVBound = gocv.NewScalar(30, 255, 255, 0)
		default: // Green
			config.DetectorConfig.LowerHSVBound = gocv.NewScalar(35, 100, 100, 0)
			config.DetectorConfig.UpperHSVBound = gocv.NewScalar(50, 255, 255, 0)
		}

		// Create and start the color seeker
		seeker, err := NewColorSeeker(config, roomba)
		if err != nil {
			log.Printf("Error creating color seeker: %v", err)
			seekerMutex.Unlock()
			http.Error(w, fmt.Sprintf("Error: %v", err), http.StatusInternalServerError)
			return
		}

		// Store the active seeker
		activeSeeker = seeker
		seekerMutex.Unlock()

		// Start the seeker
		seeker.Start()

		response := fmt.Sprintf("Seeking %s color", colorName)
		log.Println(response)
		fmt.Fprint(w, response)
	})

	localIP := lib.GetLocalIP()
	fmt.Printf("Starting web server on http://%s:8080\n", localIP)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
