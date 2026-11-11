package realtime

import "testing"

func TestNewPipelineRequiresComponents(t *testing.T) {
	if _, err := NewPipeline(PipelineConfig{}); err == nil {
		t.Fatal("NewPipeline() error = nil, want missing ASR error")
	}
}
