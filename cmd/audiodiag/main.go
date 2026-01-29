package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gordonklaus/portaudio"
)

func main() {
	testFullDuplex := flag.Bool("test-duplex", false, "Run full-duplex test (simultaneous input/output)")
	duplexDuration := flag.Int("duration", 5, "Duration of full-duplex test in seconds")
	flag.Parse()

	fmt.Println("=== PortAudio Audio Device Diagnostics ===")
	fmt.Println()

	if err := portaudio.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize PortAudio: %v\n", err)
		os.Exit(1)
	}
	defer portaudio.Terminate()

	if *testFullDuplex {
		runFullDuplexTest(*duplexDuration)
		return
	}

	// Get host APIs
	hostAPIs, err := portaudio.HostApis()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get host APIs: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d Host API(s):\n", len(hostAPIs))
	for i, api := range hostAPIs {
		fmt.Printf("  [%d] %s (devices: %d)\n", i, api.Name, len(api.Devices))
	}
	fmt.Println()

	// Get default devices
	defaultInput, err := portaudio.DefaultInputDevice()
	if err != nil {
		fmt.Printf("Default Input Device: (error: %v)\n", err)
	} else {
		fmt.Printf("Default Input Device: %s\n", defaultInput.Name)
	}

	defaultOutput, err := portaudio.DefaultOutputDevice()
	if err != nil {
		fmt.Printf("Default Output Device: (error: %v)\n", err)
	} else {
		fmt.Printf("Default Output Device: %s\n", defaultOutput.Name)
	}
	fmt.Println()

	// List all devices
	devices, err := portaudio.Devices()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get devices: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== All Devices (%d) ===\n\n", len(devices))

	for i, dev := range devices {
		isDefault := ""
		if defaultInput != nil && dev.Name == defaultInput.Name && dev.MaxInputChannels > 0 {
			isDefault = " [DEFAULT INPUT]"
		}
		if defaultOutput != nil && dev.Name == defaultOutput.Name && dev.MaxOutputChannels > 0 {
			if isDefault != "" {
				isDefault += " [DEFAULT OUTPUT]"
			} else {
				isDefault = " [DEFAULT OUTPUT]"
			}
		}

		// Detect if it's likely a Bluetooth device
		isBluetooth := strings.Contains(strings.ToLower(dev.Name), "bluetooth") ||
			strings.Contains(strings.ToLower(dev.Name), "airpods") ||
			strings.Contains(strings.ToLower(dev.Name), "buds") ||
			strings.Contains(strings.ToLower(dev.Name), "wireless") ||
			strings.Contains(strings.ToLower(dev.Name), "bt") ||
			strings.Contains(strings.ToLower(dev.Name), "headset")

		btMarker := ""
		if isBluetooth {
			btMarker = " 🎧 (Bluetooth?)"
		}

		fmt.Printf("[%d] %s%s%s\n", i, dev.Name, isDefault, btMarker)
		fmt.Printf("    Max Input Channels:  %d\n", dev.MaxInputChannels)
		fmt.Printf("    Max Output Channels: %d\n", dev.MaxOutputChannels)
		fmt.Printf("    Default Sample Rate: %.0f Hz\n", dev.DefaultSampleRate)
		fmt.Printf("    Input Latency:  Low=%.1fms, High=%.1fms\n",
			dev.DefaultLowInputLatency.Seconds()*1000,
			dev.DefaultHighInputLatency.Seconds()*1000)
		fmt.Printf("    Output Latency: Low=%.1fms, High=%.1fms\n",
			dev.DefaultLowOutputLatency.Seconds()*1000,
			dev.DefaultHighOutputLatency.Seconds()*1000)

		// Provide recommendations for input devices
		if dev.MaxInputChannels > 0 {
			fmt.Printf("    --- Recommendations for Input ---\n")

			// Check sample rate
			if dev.DefaultSampleRate != 16000 {
				fmt.Printf("    ⚠️  Sample rate is %.0f Hz, not 16000 Hz\n", dev.DefaultSampleRate)
				fmt.Printf("       Consider setting sample_rate to %.0f in config\n", dev.DefaultSampleRate)
			}

			// Check latency
			if dev.DefaultHighInputLatency.Seconds()*1000 > 100 {
				fmt.Printf("    ⚠️  High input latency (%.1fms) - consider using high_latency: true\n",
					dev.DefaultHighInputLatency.Seconds()*1000)
			}

			// Calculate recommended buffer size
			// Buffer should be at least 2x the high latency to avoid overflow
			recommendedBufferMs := int(dev.DefaultHighInputLatency.Seconds() * 1000 * 3)
			if recommendedBufferMs < 200 {
				recommendedBufferMs = 200
			}
			recommendedBufferSamples := int(dev.DefaultSampleRate) * recommendedBufferMs / 1000
			fmt.Printf("    💡 Recommended buffer_size: %d (%.0fms at %.0f Hz)\n",
				recommendedBufferSamples, float64(recommendedBufferMs), dev.DefaultSampleRate)
		}

		fmt.Println()
	}

	// Print config recommendations for default input
	if defaultInput != nil && defaultInput.MaxInputChannels > 0 {
		fmt.Println("=== Recommended Config for Default Input Device ===")
		fmt.Println()

		sampleRate := int(defaultInput.DefaultSampleRate)
		if sampleRate == 0 {
			sampleRate = 16000
		}

		highLatency := defaultInput.DefaultHighInputLatency.Seconds()*1000 > 50

		bufferMs := int(defaultInput.DefaultHighInputLatency.Seconds() * 1000 * 3)
		if bufferMs < 200 {
			bufferMs = 200
		}
		bufferSize := sampleRate * bufferMs / 1000

		fmt.Println("Add this to your config/voicebot.json:")
		fmt.Println()
		fmt.Println("\"in_pipe\": {")
		fmt.Printf("    \"sample_rate\": %d,\n", sampleRate)
		fmt.Println("    \"channels\": 1,")
		fmt.Println("    \"enable_vad\": true,")
		fmt.Println("    \"vad_threshold\": 0.5,")
		fmt.Printf("    \"buffer_size\": %d,\n", bufferSize)
		fmt.Printf("    \"high_latency\": %v\n", highLatency)
		fmt.Println("}")
		fmt.Println()

		if defaultInput.DefaultSampleRate != 16000 {
			fmt.Printf("⚠️  NOTE: Your device uses %.0f Hz, but ASR expects 16000 Hz.\n", defaultInput.DefaultSampleRate)
			fmt.Println("   Audio resampling may be needed, which could affect quality.")
			fmt.Println("   For best results, try to use a device that supports 16000 Hz.")
		}
	}
}

func runFullDuplexTest(durationSec int) {
	fmt.Println("=== Full-Duplex Test ===")
	fmt.Println("This test will simultaneously open input and output streams.")
	fmt.Println("If you're using Bluetooth, this may cause issues on macOS.")
	fmt.Println()

	defaultInput, err := portaudio.DefaultInputDevice()
	if err != nil {
		fmt.Printf("❌ Failed to get default input device: %v\n", err)
		return
	}
	defaultOutput, err := portaudio.DefaultOutputDevice()
	if err != nil {
		fmt.Printf("❌ Failed to get default output device: %v\n", err)
		return
	}

	fmt.Printf("Input Device:  %s (%.0f Hz)\n", defaultInput.Name, defaultInput.DefaultSampleRate)
	fmt.Printf("Output Device: %s (%.0f Hz)\n", defaultOutput.Name, defaultOutput.DefaultSampleRate)
	fmt.Println()

	// Test 1: Output only
	fmt.Println("Test 1: Output stream only...")
	outputBuffer := make([]float32, 1024)
	outputStream, err := portaudio.OpenDefaultStream(0, 1, 24000, len(outputBuffer), &outputBuffer)
	if err != nil {
		fmt.Printf("❌ Failed to open output stream: %v\n", err)
		return
	}
	if err := outputStream.Start(); err != nil {
		fmt.Printf("❌ Failed to start output stream: %v\n", err)
		outputStream.Close()
		return
	}
	fmt.Println("✅ Output stream started successfully")
	time.Sleep(500 * time.Millisecond)

	// Test 2: Input while output is running
	fmt.Println()
	fmt.Println("Test 2: Opening input stream while output is running...")
	inputBuffer := make([]int16, 3200)
	inputStream, err := portaudio.OpenDefaultStream(1, 0, 16000, len(inputBuffer), &inputBuffer)
	if err != nil {
		fmt.Printf("❌ Failed to open input stream: %v\n", err)
		fmt.Println("   This may indicate a full-duplex conflict with Bluetooth.")
		outputStream.Stop()
		outputStream.Close()
		return
	}
	if err := inputStream.Start(); err != nil {
		fmt.Printf("❌ Failed to start input stream: %v\n", err)
		fmt.Println("   This may indicate a full-duplex conflict with Bluetooth.")
		inputStream.Close()
		outputStream.Stop()
		outputStream.Close()
		return
	}
	fmt.Println("✅ Input stream started successfully (full-duplex working)")

	// Test 3: Read from input
	fmt.Println()
	fmt.Printf("Test 3: Reading audio for %d seconds...\n", durationSec)

	successReads := 0
	failedReads := 0
	totalReadTime := time.Duration(0)

	deadline := time.Now().Add(time.Duration(durationSec) * time.Second)
	for time.Now().Before(deadline) {
		readStart := time.Now()
		err := inputStream.Read()
		readDuration := time.Since(readStart)
		totalReadTime += readDuration

		if err != nil {
			failedReads++
			if failedReads <= 3 {
				fmt.Printf("❌ Read error: %v (took %.1fms)\n", err, readDuration.Seconds()*1000)
			}
		} else {
			successReads++
			// Expected read time for 3200 samples at 16kHz = 200ms
			if readDuration > 600*time.Millisecond {
				fmt.Printf("⚠️  Read blocked for %.1fms (expected ~200ms)\n", readDuration.Seconds()*1000)
			}
		}
	}

	fmt.Println()
	fmt.Println("=== Test Results ===")
	fmt.Printf("Successful reads: %d\n", successReads)
	fmt.Printf("Failed reads:     %d\n", failedReads)
	if successReads > 0 {
		avgReadTime := totalReadTime / time.Duration(successReads+failedReads)
		fmt.Printf("Average read time: %.1fms\n", avgReadTime.Seconds()*1000)
	}

	if failedReads > 0 {
		fmt.Println()
		fmt.Println("⚠️  DIAGNOSIS: Full-duplex issues detected!")
		fmt.Println("   If using Bluetooth, try one of these solutions:")
		fmt.Println("   1. Use wired headphones/microphone")
		fmt.Println("   2. Use MacBook's built-in mic with Bluetooth output")
		fmt.Println("   3. Use separate devices for input and output")
	} else if successReads > 0 {
		fmt.Println()
		fmt.Println("✅ Full-duplex appears to be working correctly!")
	}

	// Cleanup
	inputStream.Stop()
	inputStream.Close()
	outputStream.Stop()
	outputStream.Close()
}
