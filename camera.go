package main

import (
	"fmt"
	"image"
	"image/color"
	"log"

	"gocv.io/x/gocv"
)

func main() {
	webcam, err := gocv.OpenVideoCapture(0)
	if err != nil {
		log.Fatalf("Error opening webcam: %v", err)
	}
	defer webcam.Close()

	window := gocv.NewWindow("Color Tracking")
	defer window.Close()

	img := gocv.NewMat()
	defer img.Close()

	processed := gocv.NewMat()
	defer processed.Close()

	hsvImg := gocv.NewMat()
	defer hsvImg.Close()

	mask := gocv.NewMat()
	defer mask.Close()

	// Mat for displaying the color mask in BGR format for visualization
	coloredMask := gocv.NewMat()
	defer coloredMask.Close()

	// Create combined display image
	combinedDisplay := gocv.NewMat()
	defer combinedDisplay.Close()

	kernel := gocv.GetStructuringElement(gocv.MorphRect, image.Pt(5, 5))
	defer kernel.Close()

	// Define color to detect (lime green #9BF44B)
	// Wider range for better detection in various lighting conditions
	lowerColorBound := gocv.NewScalar(35, 100, 100, 0) // Lower HSV bounds - more tolerant
	upperColorBound := gocv.NewScalar(50, 255, 255, 0) // Upper HSV bounds

	green := color.RGBA{0, 255, 0, 0}
	red := color.RGBA{0, 0, 255, 0}
	blue := color.RGBA{255, 0, 0, 0}
	black := color.RGBA{0, 0, 0, 0}
	white := color.RGBA{255, 255, 255, 0}

	var centerRect image.Rectangle

	for {
		if ok := webcam.Read(&img); !ok {
			log.Println("Cannot read from webcam")
			break
		}
		if img.Empty() {
			continue
		}

		originalImg := img.Clone()
		defer originalImg.Close()

		width := img.Cols()
		height := img.Rows()
		centerWidth := width / 12
		centerRect = image.Rect(
			(width/2)-(centerWidth/2),
			0,
			(width/2)+(centerWidth/2),
			height,
		)

		// Pre-process the image to improve detection
		// 1. Apply Gaussian blur to reduce noise
		gocv.GaussianBlur(img, &processed, image.Pt(5, 5), 0, 0, gocv.BorderDefault)

		// 2. Convert to HSV color space for better color detection
		gocv.CvtColor(processed, &hsvImg, gocv.ColorBGRToHSV)

		// 3. Create mask for the specified color range
		gocv.InRangeWithScalar(hsvImg, lowerColorBound, upperColorBound, &mask)

		// 4. Apply morphological operations to clean up the mask
		// Erode to remove small noise
		gocv.Erode(mask, &mask, kernel)
		// Dilate to fill gaps
		gocv.Dilate(mask, &mask, kernel)

		// Convert mask to BGR for display (white becomes green)
		gocv.CvtColor(mask, &coloredMask, gocv.ColorGrayToBGR)

		contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)

		foundColor := false
		var largestIdx int = -1
		maxArea := 0.0
		var statusText string
		var statusColor color.RGBA

		if contours.Size() > 0 {
			for i := 0; i < contours.Size(); i++ {
				area := gocv.ContourArea(contours.At(i))
				if area > maxArea {
					maxArea = area
					largestIdx = i
				}
			}

			// Lower the threshold for detection to make it more sensitive
			if maxArea > 300 && largestIdx >= 0 {
				foundColor = true
				// Get the bounding rectangle of the contour
				rect := gocv.BoundingRect(contours.At(largestIdx))

				// Draw the bounding rectangle on both original and mask
				gocv.Rectangle(&originalImg, rect, green, 2)
				gocv.Rectangle(&coloredMask, rect, green, 2)

				// Calculate center of the contour
				centerX := rect.Min.X + (rect.Dx() / 2)

				// Determine position relative to center
				if rect.Overlaps(centerRect) {
					statusText = "CENTERED"
					statusColor = green
					fmt.Println("Color is CENTERED")
				} else if centerX < centerRect.Min.X {
					statusText = "LEFT"
					statusColor = red
					fmt.Println("Color is to the LEFT")
				} else {
					statusText = "RIGHT"
					statusColor = red
					fmt.Println("Color is to the RIGHT")
				}
			}
		}

		// If no color was found or contour was too small
		if !foundColor {
			statusText = "NOT FOUND"
			statusColor = red
			fmt.Println("Color NOT FOUND")
		}

		// Draw center region for reference on both views
		gocv.Rectangle(&originalImg, centerRect, blue, 1)
		gocv.Rectangle(&coloredMask, centerRect, blue, 1)

		statusBarHeight := 60
		totalHeight := (height * 2) + statusBarHeight

		combinedDisplay = gocv.NewMatWithSize(totalHeight, width, gocv.MatTypeCV8UC3)
		gocv.Rectangle(&combinedDisplay, image.Rect(0, 0, width, totalHeight), black, -1)

		roi := combinedDisplay.Region(image.Rect(0, 0, width, height))
		originalImg.CopyTo(&roi)
		roi.Close()

		roi = combinedDisplay.Region(image.Rect(0, height+statusBarHeight, width, totalHeight))
		coloredMask.CopyTo(&roi)
		roi.Close()

		gocv.PutText(&combinedDisplay, "Original", image.Pt(10, 25), gocv.FontHersheyPlain, 1.2, white, 2)
		gocv.PutText(&combinedDisplay, "Color Mask", image.Pt(10, height+statusBarHeight+25), gocv.FontHersheyPlain, 1.2, white, 2)

		textSize := gocv.GetTextSize(statusText, gocv.FontHersheyDuplex, 1.5, 2)
		textX := (width - textSize.X) / 2
		textY := height + (statusBarHeight / 2) + 10
		gocv.PutText(&combinedDisplay, statusText, image.Pt(textX, textY), gocv.FontHersheyDuplex, 1.5, statusColor, 2)

		window.IMShow(combinedDisplay)

		if window.WaitKey(1) == 27 {
			break
		}
	}
}
