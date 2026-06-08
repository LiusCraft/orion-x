package sink

import (
	"errors"
	"testing"

	"github.com/gordonklaus/portaudio"
)

func TestIsOutputUnderflow(t *testing.T) {
	if !isOutputUnderflow(portaudio.OutputUnderflowed) {
		t.Fatal("expected OutputUnderflowed to be recognized")
	}
	if isOutputUnderflow(errors.New("Output underflowed")) {
		t.Fatal("expected plain string error not to be recognized as PortAudio underflow")
	}
	if isOutputUnderflow(nil) {
		t.Fatal("expected nil not to be recognized as PortAudio underflow")
	}
}
