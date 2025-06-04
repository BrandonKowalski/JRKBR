package lib

import (
	"fmt"
	"go.bug.st/serial"
	"time"
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
