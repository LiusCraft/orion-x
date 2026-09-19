package tts

import "context"

// VoiceCloner is an optional TTS capability for creating a custom voice.
// Providers implement this interface with their own voice-cloning protocol.
type VoiceCloner interface {
	Synthesizer
	CloneVoice(ctx context.Context, req VoiceCloneRequest) (*VoiceCloneResult, error)
}

// VoiceCloneRequest describes the provider-independent inputs for voice cloning.
// Provider-specific options can be passed through Extra when needed.
type VoiceCloneRequest struct {
	TargetModel    string
	Prefix         string
	Format         AudioFormat // 参考音频格式，如 wav、mp3、m4a
	SourceAudioURL string
	LanguageHints  []string
	Extra          map[string]any
}

// VoiceCloneResult contains the provider-generated custom voice identifier.
type VoiceCloneResult struct {
	VoiceID     string
	TargetModel string
	RequestID   string
}
