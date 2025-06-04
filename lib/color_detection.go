package lib

import (
	"image"
	"image/color"
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

// ColorDetectionConfig holds configuration parameters for the color detection
type ColorDetectionConfig struct {
	LowerHSVBound   gocv.Scalar
	UpperHSVBound   gocv.Scalar
	CenterWidth     int // Width of the center region as a fraction of the image width (e.g., 12 means 1/12 of width)
	MinContourArea  float64
	ShowWindow      bool
	WindowName      string
	CameraID        int
	MorphKernelSize int
}

// DefaultColorDetectionConfig returns a default configuration for green line detection
func DefaultColorDetectionConfig() ColorDetectionConfig {
	return ColorDetectionConfig{
		LowerHSVBound:   gocv.NewScalar(35, 100, 100, 0), // Green color in HSV
		UpperHSVBound:   gocv.NewScalar(50, 255, 255, 0),
		CenterWidth:     12, // Center region is 1/12 of the frame width
		MinContourArea:  300,
		ShowWindow:      false, // Default to headless mode
		WindowName:      "Line Tracking",
		CameraID:        0,
		MorphKernelSize: 5,
	}
}

// ColorDetector handles detection of colored lines in video feed
type ColorDetector struct {
	Config       ColorDetectionConfig
	webcam       *gocv.VideoCapture
	window       *gocv.Window
	centerRect   image.Rectangle
	position     LinePosition
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
		Config:       config,
		webcam:       webcam,
		window:       window,
		position:     LineNotFound,
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
	}

	if cd.window != nil {
		cd.window.Close()
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

// GetPosition returns the current detected line position
func (cd *ColorDetector) GetPosition() LinePosition {
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	return cd.position
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

// GetDisplayFrame returns a copy of the last display frame with annotations
func (cd *ColorDetector) GetDisplayFrame() gocv.Mat {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	if cd.displayFrame.Empty() {
		return gocv.NewMat()
	}
	return cd.displayFrame.Clone()
}

// ShowCurrentFrame displays the current frame in the window
// IMPORTANT: This must be called from the main thread
func (cd *ColorDetector) ShowCurrentFrame() bool {
	if !cd.Config.ShowWindow || cd.window == nil {
		return false
	}

	cd.mu.RLock()
	defer cd.mu.RUnlock()

	if cd.displayFrame.Empty() {
		return false
	}

	// Show the frame in the window
	cd.window.IMShow(cd.displayFrame)
	return true
}

// WaitKey waits for a key press with the given delay
// IMPORTANT: This must be called from the main thread
func (cd *ColorDetector) WaitKey(delay int) int {
	if !cd.Config.ShowWindow || cd.window == nil {
		return -1
	}
	return cd.window.WaitKey(delay)
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
	black := color.RGBA{0, 0, 0, 0}
	white := color.RGBA{255, 255, 255, 0}

	for {
		select {
		case <-cd.stopChan:
			return
		default:
			// Read frame from webcam
			if ok := cd.webcam.Read(&img); !ok || img.Empty() {
				time.Sleep(10 * time.Millisecond) // Small delay to avoid busy waiting
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

			// Pre-process the image to improve detection
			gocv.GaussianBlur(img, &processed, image.Pt(5, 5), 0, 0, gocv.BorderDefault)
			gocv.CvtColor(processed, &hsvImg, gocv.ColorBGRToHSV)
			gocv.InRangeWithScalar(hsvImg, cd.Config.LowerHSVBound, cd.Config.UpperHSVBound, &mask)
			gocv.Erode(mask, &mask, kernel)
			gocv.Dilate(mask, &mask, kernel)
			gocv.CvtColor(mask, &coloredMask, gocv.ColorGrayToBGR)

			// Find contours in the mask
			contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)

			foundColor := false
			var largestIdx int = -1
			maxArea := 0.0
			var statusText string
			var statusColor color.RGBA
			var position LinePosition

			// Process contours if any are found
			if contours.Size() > 0 {
				// Find the largest contour
				for i := 0; i < contours.Size(); i++ {
					area := gocv.ContourArea(contours.At(i))
					if area > maxArea {
						maxArea = area
						largestIdx = i
					}
				}

				// Only process if the contour is large enough
				if maxArea > cd.Config.MinContourArea && largestIdx >= 0 {
					foundColor = true
					rect := gocv.BoundingRect(contours.At(largestIdx))

					// Draw the bounding rectangle on both original and mask
					gocv.Rectangle(&originalImg, rect, green, 2)
					gocv.Rectangle(&coloredMask, rect, green, 2)

					// Calculate center of the contour
					centerX := rect.Min.X + (rect.Dx() / 2)

					// Determine position relative to center
					if rect.Overlaps(cd.centerRect) {
						position = LineCentered
						statusText = string(LineCentered)
						statusColor = green
					} else if centerX < cd.centerRect.Min.X {
						position = LineLeft
						statusText = string(LineLeft)
						statusColor = red
					} else {
						position = LineRight
						statusText = string(LineRight)
						statusColor = red
					}
				}
			}

			// If no color was found or contour was too small
			if !foundColor {
				position = LineNotFound
				statusText = string(LineNotFound)
				statusColor = red
			}

			// Draw center region for reference on both views
			gocv.Rectangle(&originalImg, cd.centerRect, blue, 1)
			gocv.Rectangle(&coloredMask, cd.centerRect, blue, 1)

			// Create the display with the original on top and mask on bottom
			statusBarHeight := 60
			totalHeight := (height * 2) + statusBarHeight

			combinedDisplay := gocv.NewMatWithSize(totalHeight, width, gocv.MatTypeCV8UC3)
			defer combinedDisplay.Close()

			gocv.Rectangle(&combinedDisplay, image.Rect(0, 0, width, totalHeight), black, -1)

			// Copy the original image to the top part
			roi := combinedDisplay.Region(image.Rect(0, 0, width, height))
			originalImg.CopyTo(&roi)
			roi.Close()

			// Copy the mask to the bottom part
			roi = combinedDisplay.Region(image.Rect(0, height+statusBarHeight, width, totalHeight))
			coloredMask.CopyTo(&roi)
			roi.Close()

			// Add labels to identify each view
			gocv.PutText(&combinedDisplay, "Original", image.Pt(10, 25), gocv.FontHersheyPlain, 1.2, white, 2)
			gocv.PutText(&combinedDisplay, "Color Mask", image.Pt(10, height+statusBarHeight+25), gocv.FontHersheyPlain, 1.2, white, 2)

			// Add the status text in the middle section
			textSize := gocv.GetTextSize(statusText, gocv.FontHersheyDuplex, 1.5, 2)
			textX := (width - textSize.X) / 2
			textY := height + (statusBarHeight / 2) + 10
			gocv.PutText(&combinedDisplay, statusText, image.Pt(textX, textY), gocv.FontHersheyDuplex, 1.5, statusColor, 2)

			// Update the stored position and frames
			cd.mu.Lock()
			cd.position = position

			// Update stored frames (close old ones first)
			if !cd.lastFrame.Empty() {
				cd.lastFrame.Close()
			}
			if !cd.displayFrame.Empty() {
				cd.displayFrame.Close()
			}

			cd.lastFrame = originalImg.Clone()
			cd.displayFrame = combinedDisplay.Clone()
			cd.mu.Unlock()

			// Clean up
			originalImg.Close()
		}
	}
}
