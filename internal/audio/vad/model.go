package vad

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/liuscraft/orion-x/internal/logging"
)

const (
	// DefaultModelURL is the official Silero VAD model download URL
	DefaultModelURL = "https://github.com/snakers4/silero-vad/raw/master/files/silero_vad.onnx"
)

// ensureVADModel ensures the VAD model file exists, downloading it if necessary
func ensureModel(modelPath string) error {
	if modelPath == "" {
		modelPath = DefaultModelPath
	}

	// Expand ~ to home directory
	if strings.HasPrefix(modelPath, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get user home dir: %w", err)
		}
		modelPath = filepath.Join(home, modelPath[2:])
	}

	// Check if model already exists
	if _, err := os.Stat(modelPath); err == nil {
		logging.Infof("Silero VAD model already exists at: %s", modelPath)
		return nil
	}

	// Create parent directory
	dir := filepath.Dir(modelPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create model directory: %w", err)
	}

	// Download the model
	logging.Infof("Downloading Silero VAD model from %s to %s", DefaultModelURL, modelPath)
	if err := downloadFile(DefaultModelURL, modelPath); err != nil {
		return fmt.Errorf("download model: %w", err)
	}

	logging.Infof("Silero VAD model downloaded successfully to: %s", modelPath)
	return nil
}

// downloadFile downloads a file from url to destPath
func downloadFile(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("copy body: %w", err)
	}

	return nil
}
