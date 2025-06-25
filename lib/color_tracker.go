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

	// Bang-bang control settings
	PulseDuration   time.Duration // How long to move in each pulse
	PauseDuration   time.Duration // How long to pause between pulses
	CenterPauseTime time.Duration // How long to pause when centered
	SearchSpeed     int16         // Speed for initial search

	// Timing and behavior settings
	UpdateInterval time.Duration // How often to check color position
	StopDelay      time.Duration // How long to wait before stopping after color lost
	MaxSearchTime  time.Duration // Maximum time to search for color before giving up

	// Orientation detection settings
	MaxUnknownOrientationAttempts int           // Max attempts before giving up on orientation
	UnknownOrientationTimeout     time.Duration // How long to wait before assuming vertical

	// Color detection settings
	DetectorConfig ColorDetectionConfig
}

// DefaultColorTrackerConfig returns reasonable default settings for vertical line following
func DefaultColorTrackerConfig() ColorTrackerConfig {
	return ColorTrackerConfig{
		MaxRotationSpeed:              80,                     // Maximum rotation speed for large adjustments
		MinRotationSpeed:              50,                     // Minimum rotation speed for fine adjustments
		ForwardSpeed:                  130,                    // Moderate forward speed
		PulseDuration:                 80 * time.Millisecond,  // Short pulses to prevent overshooting
		PauseDuration:                 120 * time.Millisecond, // Pause to let robot settle
		CenterPauseTime:               200 * time.Millisecond, // Longer pause when centered
		SearchSpeed:                   60,                     // Speed for searching
		UpdateInterval:                200 * time.Millisecond, // Slower updates for bang-bang
		StopDelay:                     500 * time.Millisecond,
		MaxSearchTime:                 30 * time.Second,
		MaxUnknownOrientationAttempts: 5,               // Give up after 5 unknown attempts
		UnknownOrientationTimeout:     2 * time.Second, // Assume vertical after 2 seconds
		DetectorConfig:                DefaultColorDetectionConfig(),
	}
}

type ColorTracker struct {
	config              ColorTrackerConfig
	Detector            *ColorDetector
	roomba              *Roomba
	running             bool
	stopChan            chan struct{}
	ownedDetector       bool // Whether this tracker owns the detector
	colorLastSeen       time.Time
	lastPosition        LinePosition
	searchStarted       time.Time
	colorEverFound      bool
	currentState        RobotState
	stateStartTime      time.Time
	consecutiveCentered int

	// Orientation tracking
	unknownOrientationCount int
	orientationStartTime    time.Time
}

// RobotState represents the current state of the robot
type RobotState int

const (
	StateSearching RobotState = iota
	StateTurningLeft
	StateTurningRight
	StateMovingForward
	StatePaused
	StateStopped
)

func NewColorTracker(config ColorTrackerConfig, roomba *Roomba) (*ColorTracker, error) {
	// Make the center zone wider for bang-bang control
	config.DetectorConfig.CenterWidth = 4 // Much wider dead zone (1/4 of screen width)

	detector, err := NewColorDetector(config.DetectorConfig)
	if err != nil {
		return nil, err
	}

	return &ColorTracker{
		config:              config,
		Detector:            detector,
		roomba:              roomba,
		running:             false,
		stopChan:            make(chan struct{}),
		ownedDetector:       true, // We own this detector
		colorLastSeen:       time.Time{},
		lastPosition:        LineNotFound,
		searchStarted:       time.Time{},
		colorEverFound:      false,
		currentState:        StateStopped,
		stateStartTime:      time.Now(),
		consecutiveCentered: 0,
	}, nil
}

// NewColorTrackerWithDetector creates a tracker with an existing detector
func NewColorTrackerWithDetector(detector *ColorDetector, roomba *Roomba) (*ColorTracker, error) {
	// Stop the detector if it's currently running
	if detector.IsRunning() {
		detector.Stop()
	}

	config := DefaultColorTrackerConfig()

	return &ColorTracker{
		config:              config,
		Detector:            detector,
		roomba:              roomba,
		running:             false,
		stopChan:            make(chan struct{}),
		ownedDetector:       false, // Don't close this detector when tracker is closed
		colorLastSeen:       time.Time{},
		lastPosition:        LineNotFound,
		searchStarted:       time.Time{},
		colorEverFound:      false,
		currentState:        StateStopped,
		stateStartTime:      time.Now(),
		consecutiveCentered: 0,
	}, nil
}

// Start begins the color tracking behavior
func (ct *ColorTracker) Start() {
	if ct.running {
		return
	}

	ct.running = true
	ct.searchStarted = time.Now()
	ct.colorEverFound = false
	ct.stopChan = make(chan struct{}) // Create new stop channel
	ct.Detector.Start()

	// Begin searching
	ct.enterState(StateSearching)
	log.Println("Starting vertical line tracking")

	// Start the control loop
	go ct.controlLoop()
}

// Stop halts the color tracking behavior
func (ct *ColorTracker) Stop() {
	if !ct.running {
		return
	}

	ct.running = false

	// Signal the control loop to stop
	close(ct.stopChan)

	// Make sure to stop the detector
	if ct.Detector != nil {
		ct.Detector.Stop()
	}

	// Stop the Roomba
	if ct.roomba != nil {
		ct.roomba.Stop()
	}

	ct.enterState(StateStopped)
	log.Println("Vertical line tracker stopped")
}

func (ct *ColorTracker) Close() {
	ct.Stop()

	// Only close the detector if we own it
	if ct.ownedDetector && ct.Detector != nil {
		ct.Detector.Close()
	}
}

// SetColorRange allows changing the color being detected
func (ct *ColorTracker) SetColorRange(lowerHSV, upperHSV gocv.Scalar) {
	if ct.Detector != nil {
		ct.Detector.UpdateColorRange(lowerHSV, upperHSV)
	}
}

// GetColorDetector returns the underlying color detector
func (ct *ColorTracker) GetColorDetector() *ColorDetector {
	return ct.Detector
}

// enterState changes the robot state and performs the associated action
func (ct *ColorTracker) enterState(newState RobotState) {
	if ct.currentState == newState {
		return
	}

	prevState := ct.currentState
	ct.currentState = newState
	ct.stateStartTime = time.Now()

	var err error

	switch newState {
	case StateSearching:
		err = ct.roomba.Drive(ct.config.SearchSpeed, -1) // Clockwise search
		log.Printf("STATE: Searching (from %s)", ct.stateString(prevState))

	case StateTurningLeft:
		err = ct.roomba.Drive(ct.config.MinRotationSpeed, 1) // Counter-clockwise
		log.Printf("STATE: Turning Left (from %s)", ct.stateString(prevState))

	case StateTurningRight:
		err = ct.roomba.Drive(ct.config.MinRotationSpeed, -1) // Clockwise
		log.Printf("STATE: Turning Right (from %s)", ct.stateString(prevState))

	case StateMovingForward:
		err = ct.roomba.Drive(ct.config.ForwardSpeed, StraightRadius)
		log.Printf("STATE: Moving Forward (from %s)", ct.stateString(prevState))

	case StatePaused:
		err = ct.roomba.Stop()
		log.Printf("STATE: Paused (from %s)", ct.stateString(prevState))

	case StateStopped:
		err = ct.roomba.Stop()
		log.Printf("STATE: Stopped (from %s)", ct.stateString(prevState))
	}

	if err != nil {
		log.Printf("Error entering state %s: %v", ct.stateString(newState), err)
	}
}

// stateString returns a string representation of the robot state
func (ct *ColorTracker) stateString(state RobotState) string {
	switch state {
	case StateSearching:
		return "Searching"
	case StateTurningLeft:
		return "TurningLeft"
	case StateTurningRight:
		return "TurningRight"
	case StateMovingForward:
		return "MovingForward"
	case StatePaused:
		return "Paused"
	case StateStopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}

// controlLoop is the main control loop for vertical line tracking
func (ct *ColorTracker) controlLoop() {
	ticker := time.NewTicker(ct.config.UpdateInterval)
	defer ticker.Stop()

	// Create a separate timer for the max search time
	searchTimer := time.NewTimer(ct.config.MaxSearchTime)
	defer searchTimer.Stop()

	for {
		select {
		case <-ct.stopChan:
			return
		case <-searchTimer.C:
			// Maximum search time reached without finding the color
			if !ct.colorEverFound {
				log.Println("Maximum search time reached without finding color - Stopping search")
				ct.enterState(StateStopped)
				ct.Stop()
				return
			}
		case <-ticker.C:
			if !ct.running {
				return
			}

			// Check current position
			position := LineNotFound
			if ct.Detector != nil {
				position = ct.Detector.GetPosition()
			}

			// If we found color for the first time, mark it
			if position != LineNotFound && !ct.colorEverFound {
				ct.colorEverFound = true
				// Reset the search timer since we found the color
				if !searchTimer.Stop() {
					<-searchTimer.C // Drain the channel if the timer has already fired
				}
			}

			// Handle the position with bang-bang control
			ct.handleVerticalLineControl()
		}
	}
}

// handleVerticalLineControl implements simplified line following logic
func (ct *ColorTracker) handleVerticalLineControl() {
	now := time.Now()
	stateAge := now.Sub(ct.stateStartTime)

	// Get detection result
	result := ct.Detector.GetDetectionResult()

	// Update color tracking
	if result.Position != LineNotFound {
		ct.colorLastSeen = now
		ct.lastPosition = result.Position
		ct.colorEverFound = true
	}

	// Check if color has been lost for too long
	if !ct.colorLastSeen.IsZero() && now.Sub(ct.colorLastSeen) > ct.config.StopDelay {
		if ct.colorEverFound && now.Sub(ct.searchStarted) > 5*time.Second {
			log.Println("Line tracking session complete - Stopping tracker")
			ct.Stop()
			return
		} else {
			ct.enterState(StateSearching)
			ct.colorLastSeen = time.Time{}
			return
		}
	}

	// Pure position-based control - ignore orientation completely
	var nextAction RobotState = StateSearching

	if result.Position != LineNotFound {
		switch result.Position {
		case LineCentered:
			nextAction = StateMovingForward
		case LineLeft:
			nextAction = StateTurningLeft
		case LineRight:
			nextAction = StateTurningRight
		}
	}

	// Simple state transitions
	switch ct.currentState {
	case StateSearching:
		if nextAction != StateSearching {
			log.Printf("Line detected - %s", ct.stateString(nextAction))
			ct.enterState(nextAction)
		}

	case StateTurningLeft, StateTurningRight:
		if stateAge >= ct.config.PulseDuration {
			ct.enterState(StatePaused)
		}

	case StatePaused:
		if stateAge >= ct.config.PauseDuration {
			ct.enterState(nextAction)
		}

	case StateMovingForward:
		if nextAction != StateMovingForward {
			log.Printf("Need to adjust - %s", ct.stateString(nextAction))
			ct.enterState(nextAction)
		}
	}
}

// isOrientationGoodForFollowing determines if the detected line orientation is suitable for forward movement
func (ct *ColorTracker) isOrientationGoodForFollowing(result LineDetectionResult) bool {
	switch result.Orientation {
	case OrientationVertical:
		return true
	case OrientationUnknown:
		// Handle unknown orientation with timeout
		if ct.unknownOrientationCount == 0 {
			ct.orientationStartTime = time.Now()
		}
		ct.unknownOrientationCount++

		// If we've been getting unknown orientation for too long, assume it's vertical
		if time.Since(ct.orientationStartTime) > ct.config.UnknownOrientationTimeout {
			log.Printf("Unknown orientation timeout - assuming vertical")
			ct.resetOrientationTracking()
			return true
		}

		// If we've exceeded max attempts, assume vertical
		if ct.unknownOrientationCount >= ct.config.MaxUnknownOrientationAttempts {
			log.Printf("Max unknown orientation attempts reached - assuming vertical")
			ct.resetOrientationTracking()
			return true
		}

		return false
	default:
		ct.resetOrientationTracking()
		return false
	}
}

// getTurnDirectionForAlignment determines which way to turn to align with a non-vertical line
func (ct *ColorTracker) getTurnDirectionForAlignment(result LineDetectionResult) RobotState {
	// For horizontal or diagonal lines, we want to turn to make them more vertical
	// This is a simplified approach - could be improved with more sophisticated angle analysis
	angle := result.Angle

	if angle > 0 {
		return StateTurningLeft // Turn counter-clockwise to reduce positive angle
	} else {
		return StateTurningRight // Turn clockwise to reduce negative angle
	}
}

// resetOrientationTracking resets the orientation tracking counters
func (ct *ColorTracker) resetOrientationTracking() {
	ct.unknownOrientationCount = 0
	ct.orientationStartTime = time.Time{}
}
