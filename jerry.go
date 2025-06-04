package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"go.bug.st/serial"
)

const (
	StraightRadius int16 = 32767
)

type RoombaCommands struct {
	CmdStart   byte
	CmdControl byte
	CmdSafe    byte
	CmdFull    byte
	CmdPower   byte
	CmdSpot    byte
	CmdClean   byte
	CmdMax     byte
	CmdDrive   byte
	CmdMotors  byte
	CmdLeds    byte
	CmdDock    byte
}

type Roomba struct {
	port     serial.Port
	portName string
	baudRate int
	Cmds     RoombaCommands
}

func NewRoomba(portName string, baudRate int) *Roomba {
	return &Roomba{
		portName: portName,
		baudRate: baudRate,
		Cmds: RoombaCommands{
			CmdStart:   128, // Start command
			CmdControl: 130, // Control mode
			CmdSafe:    131, // Safe mode
			CmdFull:    132, // Full control mode
			CmdPower:   133, // Power down
			CmdSpot:    134, // Spot cleaning
			CmdClean:   135, // Normal cleaning
			CmdMax:     136, // Maximum cleaning
			CmdDrive:   137, // Control wheels
			CmdMotors:  138, // Control motors
			CmdLeds:    139, // Control LEDs
			CmdDock:    143, // Dock the robot
		},
	}
}

func (r *Roomba) Connect() error {
	// Set up the serial port
	mode := &serial.Mode{
		BaudRate: r.baudRate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	// Open the port
	port, err := serial.Open(r.portName, mode)
	if err != nil {
		return fmt.Errorf("failed to open serial port: %v", err)
	}
	r.port = port

	// Reset the Roomba by toggling RTS (if supported)
	// Note: This may not work on all serial adapters
	// If you can't use RTS control, you might need to use a hardware solution
	r.port.SetRTS(false)
	time.Sleep(100 * time.Millisecond)
	r.port.SetRTS(true)
	time.Sleep(2 * time.Second)

	return nil
}

func (r *Roomba) Close() error {
	if r.port != nil {
		return r.port.Close()
	}
	return nil
}

func (r *Roomba) sendCommand(cmd byte) error {
	_, err := r.port.Write([]byte{cmd})
	time.Sleep(100 * time.Millisecond) // Give the Roomba time to process
	return err
}

func (r *Roomba) Start() error {
	return r.sendCommand(r.Cmds.CmdStart)
}

func (r *Roomba) Control() error {
	return r.sendCommand(r.Cmds.CmdControl)
}

func (r *Roomba) SafeMode() error {
	return r.sendCommand(r.Cmds.CmdSafe)
}

func (r *Roomba) FullMode() error {
	return r.sendCommand(r.Cmds.CmdFull)
}

func (r *Roomba) Clean() error {
	return r.sendCommand(r.Cmds.CmdClean)
}

func (r *Roomba) SpotClean() error {
	return r.sendCommand(r.Cmds.CmdSpot)
}

func (r *Roomba) MaxClean() error {
	return r.sendCommand(r.Cmds.CmdMax)
}

func (r *Roomba) Dock() error {
	return r.sendCommand(r.Cmds.CmdDock)
}

func (r *Roomba) PowerOff() error {
	return r.sendCommand(r.Cmds.CmdPower)
}

// Drive controls the Roomba's movement
// velocity: -500 to 500 mm/s
// radius: -2000 to 2000 mm, special cases: 32768=straight, 1=clockwise, -1=counterclockwise
func (r *Roomba) Drive(velocity int16, radius int16) error {
	command := []byte{
		r.Cmds.CmdDrive,
		byte(velocity >> 8),   // Velocity high byte
		byte(velocity & 0xFF), // Velocity low byte
		byte(radius >> 8),     // Radius high byte
		byte(radius & 0xFF),   // Radius low byte
	}
	_, err := r.port.Write(command)
	return err
}

func (r *Roomba) Stop() error {
	return r.Drive(0, 0)
}

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
    
    <script>
        function updateSpeedValue(val) {
            document.getElementById('speedValue').textContent = val;
        }
        
        function sendCommand(command) {
            const speed = document.getElementById('speed').value;
            const statusElement = document.getElementById('status');
            
            fetch('/control', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/x-www-form-urlencoded',
                },
                body: 'command=' + command + '&speed=' + speed
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
        });
    </script>
</body>
</html>
`

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

	roomba := NewRoomba(portName, 115200)

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
			err = roomba.Drive(speedInt16, StraightRadius)
			response = "Moving forward"
		case "backward":
			log.Printf("Executing backward command with speed %d", speedInt16)
			err = roomba.Drive(-speedInt16, StraightRadius)
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

	fmt.Println("Starting web server on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
