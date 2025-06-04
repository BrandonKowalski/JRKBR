package lib

import (
	"gocv.io/x/gocv"
	"log"
	"time"
)

// ColorTrackerConfig holds configuration for the color tracking behavior
type ColorTrackerConfig struct {
	// Speed settings
	MaxRotationSpeed int16 // Maximum speed for rotating to find and follow color
	MinRotationSpeed int16 // Minimum speed for fine adjustments
	ForwardSpeed     int16 // Speed for moving forward when color is centered

	// Timing and behavior settings
	UpdateInterval time.Duration // How often to check color position
	StopDelay      time.Duration // How long to wait before stopping after color lost

	// Color detection settings
	DetectorConfig ColorDetectionConfig
}

// DefaultColorTrackerConfig returns reasonable default settings
func DefaultColorTrackerConfig() ColorTrackerConfig {
	return ColorTrackerConfig{
		MaxRotationSpeed: 80,  // Maximum rotation speed for large adjustments
		MinRotationSpeed: 35,  // Minimum rotation speed for fine adjustments
		ForwardSpeed:     130, // Moderate forward speed
		UpdateInterval:   50 * time.Millisecond,
		StopDelay:        300 * time.Millisecond,
		DetectorConfig:   DefaultColorDetectionConfig(),
	}
}

// ColorTracker implements a simple state machine for tracking colors
type ColorTracker struct {
	config        ColorTrackerConfig
	colorDetector *ColorDetector
	roomba        *Roomba
	running       bool
	stopChan      chan struct{}
	colorLastSeen time.Time
	lastPosition  LinePosition // Track previous position to reduce oscillation
}

// NewColorTracker creates a new color tracker
func NewColorTracker(config ColorTrackerConfig, roomba *Roomba) (*ColorTracker, error) {
	detector, err := NewColorDetector(config.DetectorConfig)
	if err != nil {
		return nil, err
	}

	return &ColorTracker{
		config:        config,
		colorDetector: detector,
		roomba:        roomba,
		running:       false,
		stopChan:      make(chan struct{}),
		colorLastSeen: time.Time{},
		lastPosition:  LineNotFound,
	}, nil
}

// Start begins the color tracking behavior
func (ct *ColorTracker) Start() {
	if ct.running {
		return
	}

	ct.running = true
	ct.colorDetector.Start()

	// Begin searching by rotating
	ct.roomba.Drive(ct.config.MinRotationSpeed, -1) // Start rotating clockwise at minimal speed
	log.Println("Starting color search - Rotating clockwise")

	// Start the control loop
	go ct.controlLoop()
}

// Stop halts the color tracking behavior
func (ct *ColorTracker) Stop() {
	if !ct.running {
		return
	}

	ct.running = false
	close(ct.stopChan)

	ct.colorDetector.Stop()
	ct.roomba.Stop()

	log.Println("Color tracker stopped")
}

// Close releases all resources
func (ct *ColorTracker) Close() {
	ct.Stop()
	ct.colorDetector.Close()
}

// SetColorRange allows changing the color being detected
func (ct *ColorTracker) SetColorRange(lowerHSV, upperHSV gocv.Scalar) {
	ct.colorDetector.Config.LowerHSVBound = lowerHSV
	ct.colorDetector.Config.UpperHSVBound = upperHSV
}

// GetColorDetector returns the underlying color detector
func (ct *ColorTracker) GetColorDetector() *ColorDetector {
	return ct.colorDetector
}

// controlLoop is the main control loop for color tracking
func (ct *ColorTracker) controlLoop() {
	ticker := time.NewTicker(ct.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ct.stopChan:
			return
		case <-ticker.C:
			position := ct.colorDetector.GetPosition()
			ct.handleColorPosition(position)
		}
	}
}

// handleColorPosition reacts to the detected color position
func (ct *ColorTracker) handleColorPosition(position LinePosition) {
	var err error

	switch position {
	case LineNotFound:
		// Check if we recently saw the color
		if !ct.colorLastSeen.IsZero() && time.Since(ct.colorLastSeen) > ct.config.StopDelay {
			// Color has been missing for too long - stop the robot
			log.Println("Color lost - Stopping")
			err = ct.roomba.Stop()
			ct.colorLastSeen = time.Time{} // Reset the last seen time
		} else if ct.colorLastSeen.IsZero() {
			// If we've never seen the color, use a slow search speed
			err = ct.roomba.Drive(ct.config.MinRotationSpeed, -1) // Slow clockwise rotation
		}
		// If we've just started searching or recently lost the color,
		// continue with the current rotation

	case LineCentered:
		// Color is centered - move forward
		ct.colorLastSeen = time.Now()
		err = ct.roomba.Drive(ct.config.ForwardSpeed, StraightRadius)
		log.Println("Color CENTERED - Moving forward")
		ct.lastPosition = LineCentered

	case LineLeft:
		// Color is to the left - rotate counter-clockwise
		ct.colorLastSeen = time.Now()

		// Use different speeds based on whether we're switching directions
		rotationSpeed := ct.config.MinRotationSpeed
		if ct.lastPosition == LineRight {
			// If we're switching from right to left, use an even lower speed to prevent oscillation
			rotationSpeed = ct.config.MinRotationSpeed / 2
		} else if ct.lastPosition != LineLeft {
			// If this is the first detection or after being centered, use normal speed
			rotationSpeed = ct.config.MinRotationSpeed
		}

		err = ct.roomba.Drive(rotationSpeed, 1) // Counter-clockwise
		log.Println("Color LEFT - Rotating left at speed", rotationSpeed)
		ct.lastPosition = LineLeft

	case LineRight:
		// Color is to the right - rotate clockwise
		ct.colorLastSeen = time.Now()

		// Use different speeds based on whether we're switching directions
		rotationSpeed := ct.config.MinRotationSpeed
		if ct.lastPosition == LineLeft {
			// If we're switching from left to right, use an even lower speed to prevent oscillation
			rotationSpeed = ct.config.MinRotationSpeed / 2
		} else if ct.lastPosition != LineRight {
			// If this is the first detection or after being centered, use normal speed
			rotationSpeed = ct.config.MinRotationSpeed
		}

		err = ct.roomba.Drive(rotationSpeed, -1) // Clockwise
		log.Println("Color RIGHT - Rotating right at speed", rotationSpeed)
		ct.lastPosition = LineRight
	}

	if err != nil {
		log.Printf("Error controlling Roomba: %v", err)
	}
}
