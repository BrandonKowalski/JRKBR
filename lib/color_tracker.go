package lib

import (
	"gocv.io/x/gocv"
	"log"
	"math"
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

// ColorTracker implements a bang-bang control state machine for tracking vertical lines
type ColorTracker struct {
	config         ColorTrackerConfig
	colorDetector  *ColorDetector
	roomba         *Roomba
	running        bool
	stopChan       chan struct{}
	colorLastSeen  time.Time
	lastPosition   LinePosition
	searchStarted  time.Time
	colorEverFound bool

	// Bang-bang control state
	currentState        RobotState
	stateStartTime      time.Time
	consecutiveCentered int // Count consecutive centered detections

	// Orientation tracking
	unknownOrientationCount int
	firstUnknownTime        time.Time
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

// NewColorTracker creates a new color tracker for vertical line following
func NewColorTracker(config ColorTrackerConfig, roomba *Roomba) (*ColorTracker, error) {
	// Make the center zone wider for bang-bang control
	config.DetectorConfig.CenterWidth = 4 // Much wider dead zone (1/4 of screen width)

	detector, err := NewColorDetector(config.DetectorConfig)
	if err != nil {
		return nil, err
	}

	return &ColorTracker{
		config:              config,
		colorDetector:       detector,
		roomba:              roomba,
		running:             false,
		stopChan:            make(chan struct{}),
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
	ct.colorDetector.Start()

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
	if ct.colorDetector != nil {
		ct.colorDetector.Stop()
	}

	// Stop the Roomba
	if ct.roomba != nil {
		ct.roomba.Stop()
	}

	ct.enterState(StateStopped)
	log.Println("Vertical line tracker stopped")
}

// Close releases all resources
func (ct *ColorTracker) Close() {
	// First stop tracking
	ct.Stop()

	// Then close detector resources
	if ct.colorDetector != nil {
		ct.colorDetector.Close()
		ct.colorDetector = nil
	}
}

// SetColorRange allows changing the color being detected
func (ct *ColorTracker) SetColorRange(lowerHSV, upperHSV gocv.Scalar) {
	if ct.colorDetector != nil {
		ct.colorDetector.Config.LowerHSVBound = lowerHSV
		ct.colorDetector.Config.UpperHSVBound = upperHSV
	}
}

// GetColorDetector returns the underlying color detector
func (ct *ColorTracker) GetColorDetector() *ColorDetector {
	return ct.colorDetector
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
			if ct.colorDetector != nil {
				position = ct.colorDetector.GetPosition()
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
			ct.handleVerticalLineControl(position)
		}
	}
}

// handleVerticalLineControl implements line following logic for vertical lines
func (ct *ColorTracker) handleVerticalLineControl(position LinePosition) {
	now := time.Now()
	stateAge := now.Sub(ct.stateStartTime)

	// Get full detection result including orientation
	result := ct.colorDetector.GetDetectionResult()

	// Update color tracking
	if result.Position != LineNotFound {
		ct.colorLastSeen = now
		ct.lastPosition = result.Position
	}

	// Check if color has been lost for too long
	if !ct.colorLastSeen.IsZero() && now.Sub(ct.colorLastSeen) > ct.config.StopDelay {
		if ct.colorEverFound && now.Sub(ct.searchStarted) > 5*time.Second {
			log.Println("Vertical line tracking session complete - Stopping tracker")
			ct.Stop()
			return
		} else {
			ct.enterState(StateSearching)
			ct.colorLastSeen = time.Time{}
			ct.consecutiveCentered = 0
			ct.resetOrientationTracking()
			return
		}
	}

	// Determine if the line orientation is acceptable for forward movement
	isOrientationAcceptable := ct.isOrientationGoodForFollowing(result)

	// Determine the best action based on position and orientation
	var preferredAction RobotState
	if result.Position == LineCentered && isOrientationAcceptable {
		preferredAction = StateMovingForward
	} else if result.Position == LineCentered && !isOrientationAcceptable {
		// Line is centered but not vertical - need to turn to align with it
		preferredAction = ct.getTurnDirectionForAlignment(result)
	} else {
		// Line is not centered - turn to center it
		switch result.Position {
		case LineLeft:
			preferredAction = StateTurningLeft
		case LineRight:
			preferredAction = StateTurningRight
		default:
			preferredAction = StateSearching
		}
	}

	// State machine logic
	switch ct.currentState {
	case StateSearching:
		if result.Position != LineNotFound {
			if preferredAction == StateMovingForward {
				ct.consecutiveCentered++
				if ct.consecutiveCentered >= 2 {
					log.Printf("Vertical line ready for following")
					ct.enterState(StateMovingForward)
					ct.consecutiveCentered = 0
					ct.resetOrientationTracking()
				}
			} else {
				log.Printf("Need to align with line - %s", ct.stateString(preferredAction))
				ct.enterState(preferredAction)
				ct.consecutiveCentered = 0
			}
		}

	case StateTurningLeft, StateTurningRight:
		// Check if we should stop turning
		if stateAge >= ct.config.PulseDuration {
			ct.enterState(StatePaused)
		} else if result.Position == LineCentered && isOrientationAcceptable {
			log.Println("Achieved proper alignment while turning - stopping")
			ct.enterState(StatePaused)
		}

	case StatePaused:
		pauseDuration := ct.config.PauseDuration
		if ct.lastPosition == LineCentered {
			pauseDuration = ct.config.CenterPauseTime
		}

		if stateAge >= pauseDuration {
			if preferredAction == StateMovingForward {
				ct.consecutiveCentered++
				if ct.consecutiveCentered >= 2 {
					log.Printf("Confirmed proper alignment - moving forward")
					ct.enterState(StateMovingForward)
					ct.consecutiveCentered = 0
					ct.resetOrientationTracking()
				} else {
					ct.stateStartTime = now // Stay paused a bit longer
				}
			} else {
				log.Printf("Still need adjustment - continuing alignment")
				ct.enterState(preferredAction)
				ct.consecutiveCentered = 0
			}
		}

	case StateMovingForward:
		// While moving forward, constantly adjust to stay on the line
		switch result.Position {
		case LineLeft:
			ct.enterState(StateTurningLeft)
			ct.consecutiveCentered = 0
		case LineRight:
			ct.enterState(StateTurningRight)
			ct.consecutiveCentered = 0
		case LineNotFound:
			ct.enterState(StatePaused)
			ct.consecutiveCentered = 0
		case LineCentered:
			// Check if orientation is still good for following
			if !isOrientationAcceptable {
				log.Printf("Line orientation changed while moving - stopping to realign")
				ct.enterState(StatePaused)
				ct.consecutiveCentered = 0
			} else {
				// Continue forward
				ct.consecutiveCentered++
			}
		}

	case StateStopped:
		// Do nothing
	}

}

// isOrientationGoodForFollowing determines if the line orientation is suitable for following
func (ct *ColorTracker) isOrientationGoodForFollowing(result LineDetectionResult) bool {
	switch result.Orientation {
	case OrientationVertical:
		// Perfect - vertical line is exactly what we want to follow
		ct.resetOrientationTracking()
		return true

	case OrientationDiagonal:
		// Check if the angle is close enough to vertical
		absAngle := math.Abs(result.Angle)
		angleFromVertical := math.Min(absAngle, 180-absAngle) // Distance from 90°
		if angleFromVertical <= 30.0 {                        // More lenient threshold for forward movement
			log.Printf("Diagonal angle %.1f° is close enough to vertical (%.1f° from vertical) - allowing forward", result.Angle, angleFromVertical)
			ct.resetOrientationTracking()
			return true
		} else {
			log.Printf("Diagonal angle %.1f° is too far from vertical (%.1f° away) - needs alignment", result.Angle, angleFromVertical)
			ct.resetOrientationTracking()
			return false
		}

	case OrientationHorizontal:
		// Definitely not acceptable for vertical line following
		ct.resetOrientationTracking()
		return false

	case OrientationUnknown:
		// Track unknown orientation attempts
		if ct.firstUnknownTime.IsZero() {
			ct.firstUnknownTime = time.Now()
			ct.unknownOrientationCount = 1
			log.Printf("First unknown orientation detected")
		} else {
			ct.unknownOrientationCount++
			timeSinceFirstUnknown := time.Since(ct.firstUnknownTime)

			// Give up on orientation detection faster and just assume vertical
			if ct.unknownOrientationCount >= 3 || timeSinceFirstUnknown >= 1*time.Second {
				log.Printf("Giving up on orientation detection - assuming vertical for forward movement")
				ct.resetOrientationTracking()
				return true
			}
		}

		log.Printf("Unknown orientation attempt %d/3", ct.unknownOrientationCount)
		return false

	default:
		return false
	}
}

// getTurnDirectionForAlignment determines which way to turn to align with a vertical line
func (ct *ColorTracker) getTurnDirectionForAlignment(result LineDetectionResult) RobotState {
	switch result.Orientation {
	case OrientationDiagonal:
		// For vertical line following, we want to align the robot so the line appears vertical
		// If the line appears tilted, we need to turn to make it vertical
		if result.Angle > 0 {
			// Line tilts to the right, turn left to make it vertical
			log.Printf("Diagonal line with positive angle %.1f° - turning left to align vertically", result.Angle)
			return StateTurningLeft
		} else {
			// Line tilts to the left, turn right to make it vertical
			log.Printf("Diagonal line with negative angle %.1f° - turning right to align vertically", result.Angle)
			return StateTurningRight
		}
	case OrientationHorizontal:
		// For horizontal lines, pick a direction to try to find the vertical part
		log.Printf("Horizontal line detected - turning right to find vertical part")
		return StateTurningRight
	case OrientationUnknown:
		// For unknown orientation, alternate turns to avoid getting stuck
		if ct.unknownOrientationCount%2 == 0 {
			log.Printf("Unknown orientation - trying right turn")
			return StateTurningRight
		} else {
			log.Printf("Unknown orientation - trying left turn")
			return StateTurningLeft
		}
	default:
		// This shouldn't happen if orientation is vertical
		return StateTurningRight
	}
}

// resetOrientationTracking resets the orientation tracking counters
func (ct *ColorTracker) resetOrientationTracking() {
	ct.unknownOrientationCount = 0
	ct.firstUnknownTime = time.Time{}
}
