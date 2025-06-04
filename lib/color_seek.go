package lib

import (
	"gocv.io/x/gocv"
	"log"
	"time"
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
	DetectorConfig ColorDetectionConfig
}

// DefaultColorSeekConfig returns a default configuration for color seeking
func DefaultColorSeekConfig() ColorSeekConfig {
	return ColorSeekConfig{
		SpinSpeed:        100, // Speed for spinning to find color
		ForwardSpeed:     150, // Speed for moving forward
		AdjustmentSpeed:  80,  // Speed for adjusting position
		UpdateInterval:   50 * time.Millisecond,
		LostColorTimeout: 2 * time.Second,
		DetectorConfig:   DefaultColorDetectionConfig(),
	}
}

// ColorSeekState represents the current state of the color seeking process
type ColorSeekState string

const (
	Spinning  ColorSeekState = "SPINNING"  // Looking for color by spinning
	Centering ColorSeekState = "CENTERING" // Centering the detected color
	Following ColorSeekState = "FOLLOWING" // Following the color
	Stopped   ColorSeekState = "STOPPED"   // Stopped (color lost or finished)
)

// ColorSeeker manages the behavior of seeking and following a specific color
type ColorSeeker struct {
	config        ColorSeekConfig
	colorDetector *ColorDetector
	roomba        *Roomba
	running       bool
	stopChan      chan struct{}
	state         ColorSeekState
	lastColorSeen time.Time
}

// NewColorSeeker creates a new color seeker with the given configuration
func NewColorSeeker(config ColorSeekConfig, roomba *Roomba) (*ColorSeeker, error) {
	detector, err := NewColorDetector(config.DetectorConfig)
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
	if cs.running {
		return
	}
	cs.running = true

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
	if !cs.running {
		return
	}
	cs.running = false
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
func (cs *ColorSeeker) GetColorDetector() *ColorDetector {
	return cs.colorDetector
}

// GetCurrentState returns the current state of the color seeker
func (cs *ColorSeeker) GetCurrentState() ColorSeekState {
	return cs.state
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
func (cs *ColorSeeker) updateMovement(position LinePosition) {
	var err error

	switch position {
	case LineNotFound:
		// Color not found
		if cs.state == Following || cs.state == Centering {
			// We were following but lost the color
			if time.Since(cs.lastColorSeen) > cs.config.LostColorTimeout {
				// If we've lost the color for too long, stop
				log.Println("Color lost for too long - Stopping")
				err = cs.roomba.Stop()
				cs.state = Stopped
			}
			// Otherwise keep current movement hoping to find the color again
		} else if cs.state == Spinning {
			// Keep spinning to find the color
			err = cs.roomba.Drive(cs.config.SpinSpeed, -1) // Continue spinning clockwise
			log.Println("Spinning to find color")
		}
	case LineCentered:
		// Color is centered
		cs.lastColorSeen = time.Now()

		if cs.state == Spinning || cs.state == Centering {
			// Transition to following state
			cs.state = Following
			log.Println("Color CENTERED - Moving forward")
		}

		// Move forward with the color centered
		err = cs.roomba.Drive(cs.config.ForwardSpeed, StraightRadius)

	case LineLeft:
		// Color is to the left
		cs.lastColorSeen = time.Now()

		if cs.state == Spinning {
			// Found the color, transition to centering
			cs.state = Centering
		}

		// Turn left to center the color
		err = cs.roomba.Drive(cs.config.AdjustmentSpeed, 1) // Turn counter-clockwise
		log.Println("Color LEFT - Turning left to center")

	case LineRight:
		// Color is to the right
		cs.lastColorSeen = time.Now()

		if cs.state == Spinning {
			// Found the color, transition to centering
			cs.state = Centering
		}

		// Turn right to center the color
		err = cs.roomba.Drive(cs.config.AdjustmentSpeed, -1) // Turn clockwise
		log.Println("Color RIGHT - Turning right to center")
	}

	if err != nil {
		log.Printf("Error controlling Roomba: %v", err)
	}
}

// SetColorRange allows changing the color being detected
func (cs *ColorSeeker) SetColorRange(lowerHSV, upperHSV gocv.Scalar) {
	cs.colorDetector.Config.LowerHSVBound = lowerHSV
	cs.colorDetector.Config.UpperHSVBound = upperHSV
}
