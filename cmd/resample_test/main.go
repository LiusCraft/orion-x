package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/liuscraft/orion-x/internal/audio"
)

func main() {
	inputRate := flag.Int("input-rate", 16000, "Input sample rate (Hz)")
	outputRate := flag.Int("output-rate", 24000, "Output sample rate (Hz)")
	duration := flag.Float64("duration", 1.0, "Duration in seconds")
	freq := flag.Float64("freq", 440.0, "Frequency in Hz (A4 note)")
	output := flag.String("output", "", "Output file (optional, prints stats if empty)")
	flag.Parse()

	fmt.Printf("Multi-Sample-Rate Resampling Demo\n")
	fmt.Printf("==================================\n")
	fmt.Printf("Input rate:  %d Hz\n", *inputRate)
	fmt.Printf("Output rate: %d Hz\n", *outputRate)
	fmt.Printf("Duration:    %.2f seconds\n", *duration)
	fmt.Printf("Frequency:   %.1f Hz\n\n", *freq)

	// Generate sine wave at input rate
	inputSamples := int(float64(*inputRate) * (*duration))
	input := make([]int16, inputSamples)
	for i := 0; i < inputSamples; i++ {
		t := float64(i) / float64(*inputRate)
		sample := math.Sin(2 * math.Pi * (*freq) * t)
		input[i] = int16(sample * 16000) // Scale to avoid clipping
	}

	// Convert to bytes
	inputBytes := make([]byte, len(input)*2)
	for i, sample := range input {
		inputBytes[i*2] = byte(sample)
		inputBytes[i*2+1] = byte(sample >> 8)
	}

	fmt.Printf("Generated %d input samples (%d bytes)\n", len(input), len(inputBytes))

	// Create resampling reader
	reader := bytes.NewReader(inputBytes)
	resampler := audio.NewLinearResampler()
	resamplingReader := audio.NewResamplingReader(reader, *inputRate, *outputRate, 1, resampler)

	// Read all resampled data
	outputBytes, err := io.ReadAll(resamplingReader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resampling failed: %v\n", err)
		os.Exit(1)
	}

	outputSamples := len(outputBytes) / 2
	fmt.Printf("Resampled to %d output samples (%d bytes)\n", outputSamples, len(outputBytes))
	fmt.Printf("Sample rate ratio: %.3f\n", float64(outputSamples)/float64(len(input)))
	fmt.Printf("Expected ratio:    %.3f\n", float64(*outputRate)/float64(*inputRate))

	// Write to file if requested
	if *output != "" {
		if err := os.WriteFile(*output, outputBytes, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nOutput written to: %s\n", *output)
	}

	fmt.Printf("\n✅ Resampling completed successfully!\n")
}
