//go:build !cgo

package vad

import "fmt"

func NewSilero(string, int, float64, int, int) (Detector, error) {
	return nil, fmt.Errorf("vad: Silero requires CGO_ENABLED=1 and ONNX Runtime")
}
