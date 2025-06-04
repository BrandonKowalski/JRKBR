package lib

import (
	"log"
	"time"
)

// Direction represents the possible movement directions
type Direction string

const (
	Forward Direction = "FORWARD"
	Left    Direction = "LEFT"
	Right   Direction = "RIGHT"
	Stop    Direction = "STOP"
)

// LineFollowerConfig holds configuration for the line following behavior
type LineFollowerConfig struct {
	ForwardSpeed   int16
	TurningSpeed   int16
	UpdateInterval time.Duration
	DetectorConfig ColorDetectionConfig
}

// DefaultLineFollowerConfig returns a default configuration for line following
func DefaultLineFollowerConfig() LineFollowerConfig {
	return LineFollowerConfig{
		ForwardSpeed:   150,
		TurningSpeed:   100,
		UpdateInterval: 50 * time.Millisecond,
		DetectorConfig: DefaultColorDetectionConfig(),
	}
}

// LineFollower manages line following behavior using the color detector
type LineFollower struct {
	config        LineFollowerConfig
	colorDetector *ColorDetector
	roomba        *Roomba
	running       bool
	stopChan      chan struct{}
	direction     Direction
}

// NewLineFollower creates a new line follower with the given configuration
func NewLineFollower(config LineFollowerConfig, roomba *Roomba) (*LineFollower, error) {
	detector, err := NewColorDetector(config.DetectorConfig)
	if err != nil {
		return nil, err
	}

	return &LineFollower{
		config:        config,
		colorDetector: detector,
		roomba:        roomba,
		running:       false,
		stopChan:      make(chan struct{}),
		direction:     Stop,
	}, nil
}

// Start begins the line following behavior
func (lf *LineFollower) Start() {
	if lf.running {
		return
	}
	lf.running = true

	// Start the color detector
	lf.colorDetector.Start()

	// Start the control loop
	go lf.controlLoop()
}

// Stop halts the line following behavior
func (lf *LineFollower) Stop() {
	if !lf.running {
		return
	}
	lf.running = false
	close(lf.stopChan)

	// Stop the color detector
	lf.colorDetector.Stop()

	// Stop the Roomba
	if lf.roomba != nil {
		lf.roomba.Stop()
	}
}

// Close releases all resources
func (lf *LineFollower) Close() {
	lf.Stop()
	lf.colorDetector.Close()
}

// GetColorDetector returns the underlying color detector
func (lf *LineFollower) GetColorDetector() *ColorDetector {
	return lf.colorDetector
}

// GetCurrentDirection returns the current direction of movement
func (lf *LineFollower) GetCurrentDirection() Direction {
	return lf.direction
}

// controlLoop is the main control loop for line following
func (lf *LineFollower) controlLoop() {
	ticker := time.NewTicker(lf.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lf.stopChan:
			return
		case <-ticker.C:
			position := lf.colorDetector.GetPosition()
			lf.updateMovement(position)
		}
	}
}

// updateMovement controls the Roomba based on the detected line position
func (lf *LineFollower) updateMovement(position LinePosition) {
	var err error
	var newDirection Direction

	switch position {
	case LineCentered:
		// Move forward when the line is centered
		err = lf.roomba.Drive(lf.config.ForwardSpeed, StraightRadius)
		newDirection = Forward
		log.Println("Line CENTERED - Moving forward")
	case LineLeft:
		// Turn left when the line is to the left
		err = lf.roomba.Drive(lf.config.TurningSpeed, 1) // Turn counter-clockwise
		newDirection = Left
		log.Println("Line LEFT - Turning left")
	case LineRight:
		// Turn right when the line is to the right
		err = lf.roomba.Drive(lf.config.TurningSpeed, -1) // Turn clockwise
		newDirection = Right
		log.Println("Line RIGHT - Turning right")
	case LineNotFound:
		// Stop when the line is not found
		err = lf.roomba.Stop()
		newDirection = Stop
		log.Println("Line NOT FOUND - Stopping")
	}

	if err != nil {
		log.Printf("Error controlling Roomba: %v", err)
	}

	lf.direction = newDirection
}
