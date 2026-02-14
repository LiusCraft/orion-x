package wsserver

import (
	"testing"

	"github.com/liuscraft/orion-x/internal/config"
)

func TestNewSession_UsesServerAudioDefaultsAndVoicebotProfile(t *testing.T) {
	wsCfg := config.DefaultWSServerConfig()
	wsCfg.Server.AudioParams.Format = "pcm"
	wsCfg.Server.AudioParams.Channels = 2
	wsCfg.Server.AudioParams.FrameDurationMs = 40

	voicebot := wsCfg.Voicebot.Default
	if voicebot == nil {
		t.Fatalf("expected default voicebot config")
	}
	voicebotCopy := *voicebot
	voicebotCopy.TTS.Voice = "zhichu"

	s := NewSession(wsCfg, voicebotCopy, "bot-a", nil, "dev-1", "client-1", "sid-1", nil, nil)

	if s.audioParams.Format != "pcm" {
		t.Fatalf("expected pcm audio params, got %q", s.audioParams.Format)
	}
	if s.audioParams.Channels != 2 {
		t.Fatalf("expected channels=2, got %d", s.audioParams.Channels)
	}
	if s.audioParams.FrameDuration != 40 {
		t.Fatalf("expected frame_duration=40, got %d", s.audioParams.FrameDuration)
	}
	if s.voicebot.TTS.Voice != "zhichu" {
		t.Fatalf("expected voicebot profile voice to be stored")
	}
}
