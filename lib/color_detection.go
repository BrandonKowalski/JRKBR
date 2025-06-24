package lib

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"sync"
	"time"

	"gocv.io/x/gocv"
)

// LinePosition represents the detected position of a line
type LinePosition string

const (
	LineLeft     LinePosition = "LEFT"
	LineRight    LinePosition = "RIGHT"
	LineCentered LinePosition = "CENTERED"
	LineNotFound LinePosition = "NOT FOUND"
)

// LineOrientation represents the orientation of the detected line
type LineOrientation string

const (
	OrientationHorizontal LineOrientation = "HORIZONTAL" // Line runs left-right (perpendicular to robot direction)
	OrientationVertical   LineOrientation = "VERTICAL"   // Line runs up-down (parallel to robot direction - GOOD for following)
	OrientationDiagonal   LineOrientation = "DIAGONAL"   // Line is at an angle
	OrientationUnknown    LineOrientation = "UNKNOWN"    // Cannot determine orientation
)

// LineDetectionResult contains both position and orientation information
type LineDetectionResult struct {
	Position    LinePosition
	Orientation LineOrientation
	Angle       float64 // Angle in degrees (-90 to 90, where 0 is horizontal)
	Confidence  float64 // Confidence in the detection (0.0 to 1.0)
}

// ColorDetectionConfig holds configuration parameters for the color detection
type ColorDetectionConfig struct {
	LowerHSVBound   gocv.Scalar
	UpperHSVBound   gocv.Scalar
	CenterWidth     int // Width of the center region as a fraction of the image width (e.g., 4 means 1/4 of width)
	MinContourArea  float64
	ShowWindow      bool
	WindowName      string
	CameraID        int
	MorphKernelSize int

	// Line orientation detection settings
	OrientationTolerance float64 // Degrees of tolerance for "vertical" (±15 degrees default)
	MinLineLength        float64 // Minimum line length for reliable orientation detection
	UseHoughLines        bool    // Whether to use Hough line detection for orientation
}

// DefaultColorDetectionConfig returns a default configuration for vertical line following
func DefaultColorDetectionConfig() ColorDetectionConfig {
	return ColorDetectionConfig{
		LowerHSVBound:        gocv.NewScalar(35, 100, 100, 0), // Green color in HSV
		UpperHSVBound:        gocv.NewScalar(50, 255, 255, 0),
		CenterWidth:          4, // Much wider center zone for bang-bang control (1/4 of screen width)
		MinContourArea:       300,
		ShowWindow:           false, // Default to headless mode
		WindowName:           "Line Tracking",
		CameraID:             0,
		MorphKernelSize:      5,
		OrientationTolerance: 15.0,  // ±15 degrees is considered "vertical" for line following
		MinLineLength:        30.0,  // Lower minimum length requirement
		UseHoughLines:        false, // Use contour method only
	}
}

// BlueColorDetectionConfig returns a configuration for detecting blue color #1F75E4
func BlueColorDetectionConfig() ColorDetectionConfig {
	config := DefaultColorDetectionConfig()
	config.LowerHSVBound = gocv.NewScalar(100, 200, 200, 0)
	config.UpperHSVBound = gocv.NewScalar(110, 230, 240, 0)
	config.WindowName = "Blue Line Tracking #1F75E4"
	config.ShowWindow = true
	return config
}

// ToleranceBlueColorDetectionConfig - wider range for varying lighting
func ToleranceBlueColorDetectionConfig() ColorDetectionConfig {
	config := DefaultColorDetectionConfig()
	config.LowerHSVBound = gocv.NewScalar(98, 190, 190, 0)
	config.UpperHSVBound = gocv.NewScalar(115, 240, 250, 0)
	config.WindowName = "Tolerant Blue Detection"
	config.ShowWindow = true
	return config
}

// ColorDetector handles detection of colored lines in video feed
type ColorDetector struct {
	Config       ColorDetectionConfig
	webcam       *gocv.VideoCapture
	window       *gocv.Window
	centerRect   image.Rectangle
	lastResult   LineDetectionResult
	lastFrame    gocv.Mat
	displayFrame gocv.Mat
	running      bool
	mu           sync.RWMutex
	stopChan     chan struct{}
}

// NewColorDetector creates a new color detector with the given configuration
func NewColorDetector(config ColorDetectionConfig) (*ColorDetector, error) {
	webcam, err := gocv.OpenVideoCapture(config.CameraID)
	if err != nil {
		return nil, err
	}

	// Only create window if explicitly requested
	var window *gocv.Window
	if config.ShowWindow {
		window = gocv.NewWindow(config.WindowName)
	}

	return &ColorDetector{
		Config: config,
		webcam: webcam,
		window: window,
		lastResult: LineDetectionResult{
			Position:    LineNotFound,
			Orientation: OrientationUnknown,
			Angle:       0.0,
			Confidence:  0.0,
		},
		lastFrame:    gocv.NewMat(),
		displayFrame: gocv.NewMat(),
		running:      false,
		stopChan:     make(chan struct{}),
	}, nil
}

// Start begins the color detection in a separate goroutine
func (cd *ColorDetector) Start() {
	cd.mu.Lock()
	if cd.running {
		cd.mu.Unlock()
		return
	}
	cd.running = true
	cd.mu.Unlock()

	go cd.detectionLoop()
}

// Stop halts the color detection
func (cd *ColorDetector) Stop() {
	cd.mu.Lock()
	if !cd.running {
		cd.mu.Unlock()
		return
	}
	cd.running = false
	cd.mu.Unlock()

	close(cd.stopChan)
}

// Close releases all resources
func (cd *ColorDetector) Close() {
	cd.Stop()

	if cd.webcam != nil {
		cd.webcam.Close()
		cd.webcam = nil
	}

	if cd.window != nil {
		cd.window.Close()
		cd.window = nil
	}

	cd.mu.Lock()
	defer cd.mu.Unlock()

	if !cd.lastFrame.Empty() {
		cd.lastFrame.Close()
	}

	if !cd.displayFrame.Empty() {
		cd.displayFrame.Close()
	}
}

// GetPosition returns the current detected line position (legacy method)
func (cd *ColorDetector) GetPosition() LinePosition {
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	return cd.lastResult.Position
}

// GetDetectionResult returns the complete detection result with orientation
func (cd *ColorDetector) GetDetectionResult() LineDetectionResult {
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	return cd.lastResult
}

// GetLastFrame returns a copy of the last processed frame
func (cd *ColorDetector) GetLastFrame() gocv.Mat {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	if cd.lastFrame.Empty() {
		return gocv.NewMat()
	}
	return cd.lastFrame.Clone()
}

// ShowCurrentFrame displays the current frame in the window
func (cd *ColorDetector) ShowCurrentFrame() bool {
	if !cd.Config.ShowWindow || cd.window == nil {
		return false
	}

	cd.mu.RLock()
	defer cd.mu.RUnlock()

	if cd.displayFrame.Empty() {
		return false
	}

	cd.window.IMShow(cd.displayFrame)
	return true
}

// WaitKey waits for a key press with the given delay
func (cd *ColorDetector) WaitKey(delay int) int {
	if !cd.Config.ShowWindow || cd.window == nil {
		return -1
	}
	return cd.window.WaitKey(delay)
}

func (cd *ColorDetector) detectLineOrientation(contour gocv.PointVector, mask gocv.Mat) LineOrientation {
	// Always use contour method for more reliable results
	return cd.detectOrientationWithContour(contour)
}

// detectOrientationWithContour uses contour analysis to determine orientation
func (cd *ColorDetector) detectOrientationWithContour(contour gocv.PointVector) LineOrientation {
	if contour.Size() < 5 { // Need at least 5 points for MinAreaRect
		return OrientationUnknown
	}

	// Use minimum area rectangle to get orientation
	rect := gocv.MinAreaRect(contour)
	angle := rect.Angle

	width := rect.Width
	height := rect.Height

	// Determine the actual orientation angle
	var orientationAngle float64
	if width > height {
		// Rectangle is wider than tall - angle directly represents line orientation
		orientationAngle = angle
	} else {
		// Rectangle is taller than wide - need to adjust angle by 90 degrees
		orientationAngle = angle + 90
	}

	// Normalize to [-90, 90] range
	for orientationAngle > 90 {
		orientationAngle -= 180
	}
	for orientationAngle < -90 {
		orientationAngle += 180
	}

	// Store the angle in the result for later use
	cd.mu.Lock()
	cd.lastResult.Angle = orientationAngle
	cd.mu.Unlock()

	log.Printf("Contour orientation: rect angle=%.1f°, size=(%.1f,%.1f), final angle=%.1f°",
		angle, width, height, orientationAngle)

	return cd.classifyOrientation(orientationAngle)
}

// classifyOrientation classifies an angle into orientation categories
// For line following, we want VERTICAL lines (90 degrees) to be the target
func (cd *ColorDetector) classifyOrientation(angle float64) LineOrientation {
	absAngle := math.Abs(angle)

	log.Printf("Classifying angle %.1f° (abs=%.1f°) with tolerance %.1f°",
		angle, absAngle, cd.Config.OrientationTolerance)

	// For line following, vertical lines (±90°) are what we want to follow
	if absAngle >= (90 - cd.Config.OrientationTolerance) {
		log.Printf("Classified as VERTICAL (good for following)")
		return OrientationVertical
	} else if absAngle <= cd.Config.OrientationTolerance {
		log.Printf("Classified as HORIZONTAL (perpendicular to direction)")
		return OrientationHorizontal
	} else {
		log.Printf("Classified as DIAGONAL")
		return OrientationDiagonal
	}
}

// detectionLoop is the main processing loop for color detection
func (cd *ColorDetector) detectionLoop() {
	// Prepare images for processing
	img := gocv.NewMat()
	defer img.Close()

	processed := gocv.NewMat()
	defer processed.Close()

	hsvImg := gocv.NewMat()
	defer hsvImg.Close()

	mask := gocv.NewMat()
	defer mask.Close()

	coloredMask := gocv.NewMat()
	defer coloredMask.Close()

	kernel := gocv.GetStructuringElement(gocv.MorphRect, image.Pt(cd.Config.MorphKernelSize, cd.Config.MorphKernelSize))
	defer kernel.Close()

	// Define drawing colors
	green := color.RGBA{0, 255, 0, 0}
	red := color.RGBA{0, 0, 255, 0}
	blue := color.RGBA{255, 0, 0, 0}
	yellow := color.RGBA{0, 255, 255, 0}
	black := color.RGBA{0, 0, 0, 0}
	white := color.RGBA{255, 255, 255, 0}

	for {
		select {
		case <-cd.stopChan:
			return
		default:
			// Read frame from webcam
			if ok := cd.webcam.Read(&img); !ok || img.Empty() {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			// Clone for storage
			originalImg := img.Clone()

			// Set the center rectangle dimensions
			width := img.Cols()
			height := img.Rows()
			centerWidth := width / cd.Config.CenterWidth
			cd.centerRect = image.Rect(
				(width/2)-(centerWidth/2),
				0,
				(width/2)+(centerWidth/2),
				height,
			)

			// Pre-process the image
			gocv.GaussianBlur(img, &processed, image.Pt(5, 5), 0, 0, gocv.BorderDefault)
			gocv.CvtColor(processed, &hsvImg, gocv.ColorBGRToHSV)
			gocv.InRangeWithScalar(hsvImg, cd.Config.LowerHSVBound, cd.Config.UpperHSVBound, &mask)
			gocv.Erode(mask, &mask, kernel)
			gocv.Dilate(mask, &mask, kernel)
			gocv.CvtColor(mask, &coloredMask, gocv.ColorGrayToBGR)

			// Find contours
			contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)

			result := LineDetectionResult{
				Position:    LineNotFound,
				Orientation: OrientationUnknown,
				Angle:       0.0,
				Confidence:  0.0,
			}

			if contours.Size() > 0 {
				// Find the largest contour
				largestIdx := -1
				maxArea := 0.0
				for i := 0; i < contours.Size(); i++ {
					area := gocv.ContourArea(contours.At(i))
					if area > maxArea {
						maxArea = area
						largestIdx = i
					}
				}

				if maxArea > cd.Config.MinContourArea && largestIdx >= 0 {
					contour := contours.At(largestIdx)
					rect := gocv.BoundingRect(contour)

					// Detect orientation
					orientation := cd.detectLineOrientation(contour, mask)
					result.Orientation = orientation

					// Calculate confidence based on contour area and shape
					result.Confidence = math.Min(maxArea/1000.0, 1.0) // Simple confidence metric

					// Determine position
					centerX := rect.Min.X + (rect.Dx() / 2)
					if rect.Overlaps(cd.centerRect) {
						result.Position = LineCentered
					} else if centerX < cd.centerRect.Min.X {
						result.Position = LineLeft
					} else {
						result.Position = LineRight
					}

					// Draw bounding rectangle with color based on orientation
					var rectColor color.RGBA
					switch orientation {
					case OrientationVertical:
						rectColor = green // Green for vertical (good for line following)
					case OrientationHorizontal:
						rectColor = red // Red for horizontal (perpendicular - need to turn a lot)
					case OrientationDiagonal:
						rectColor = yellow // Yellow for diagonal (need adjustment)
					default:
						rectColor = blue // Blue for unknown
					}

					gocv.Rectangle(&originalImg, rect, rectColor, 2)
					gocv.Rectangle(&coloredMask, rect, rectColor, 2)
				}
			}

			// Store the result
			cd.mu.Lock()
			cd.lastResult = result
			cd.mu.Unlock()

			// Draw center region and status
			gocv.Rectangle(&originalImg, cd.centerRect, blue, 1)
			gocv.Rectangle(&coloredMask, cd.centerRect, blue, 1)

			// Create display with status information
			statusBarHeight := 80
			totalHeight := (height * 2) + statusBarHeight
			combinedDisplay := gocv.NewMatWithSize(totalHeight, width, gocv.MatTypeCV8UC3)
			defer combinedDisplay.Close()

			gocv.Rectangle(&combinedDisplay, image.Rect(0, 0, width, totalHeight), black, -1)

			// Copy images
			roi := combinedDisplay.Region(image.Rect(0, 0, width, height))
			originalImg.CopyTo(&roi)
			roi.Close()

			roi = combinedDisplay.Region(image.Rect(0, height+statusBarHeight, width, totalHeight))
			coloredMask.CopyTo(&roi)
			roi.Close()

			// Add status text
			statusText := fmt.Sprintf("Pos: %s | Orient: %s | Angle: %.1f° | Conf: %.2f",
				result.Position, result.Orientation, result.Angle, result.Confidence)

			var statusColor color.RGBA
			if result.Position == LineCentered && result.Orientation == OrientationVertical {
				statusColor = green // Ready for forward movement - line is centered and vertical
			} else {
				statusColor = white
			}

			gocv.PutText(&combinedDisplay, "Original", image.Pt(10, 25), gocv.FontHersheyPlain, 1.2, white, 2)
			gocv.PutText(&combinedDisplay, statusText, image.Pt(10, height+25), gocv.FontHersheyPlain, 1.0, statusColor, 2)
			gocv.PutText(&combinedDisplay, "Color Mask", image.Pt(10, height+statusBarHeight+25), gocv.FontHersheyPlain, 1.2, white, 2)

			// Store the display frame
			cd.mu.Lock()
			if !cd.lastFrame.Empty() {
				cd.lastFrame.Close()
			}
			cd.lastFrame = originalImg

			if !cd.displayFrame.Empty() {
				cd.displayFrame.Close()
			}
			cd.displayFrame = combinedDisplay.Clone()
			cd.mu.Unlock()
		}
	}
}
