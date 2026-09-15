//go:build !cgo

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "voicebot requires CGO_ENABLED=1 and a PortAudio development library")
	os.Exit(1)
}
