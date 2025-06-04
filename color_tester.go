package main

import (
	"fmt"
	"jrkbr/lib"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Create a configuration
	config := lib.DefaultColorDetectionConfig()
	config.ShowWindow = true // Enable window display

	// Create the color detector
	detector, err := lib.NewColorDetector(config)
	if err != nil {
		fmt.Printf("Error creating color detector: %v\n", err)
		return
	}
	defer detector.Close()

	// Start the detector
	fmt.Println("Starting color detector...")
	fmt.Println("Press Ctrl+C or ESC to exit")

	detector.Start()

	// Set up signal handling for clean shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create a ticker for position updates
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Main loop - IMPORTANT: Window display functions must run in the main thread
	running := true
	for running {
		select {
		case <-sigCh:
			fmt.Println("\nShutting down...")
			running = false
		case <-ticker.C:
			position := detector.GetPosition()
			fmt.Printf("Current position: %s\n", position)
		default:
			// Show the current frame in the window - runs in main thread
			if detector.ShowCurrentFrame() {
				// Wait for key press
				if key := detector.WaitKey(1); key == 27 { // ESC key
					fmt.Println("\nESC pressed, shutting down...")
					running = false
				}
			}

			// Small sleep to avoid hogging CPU
			time.Sleep(10 * time.Millisecond)
		}
	}
}
