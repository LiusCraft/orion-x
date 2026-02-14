package wsserver

import (
	"context"
	"testing"

	"github.com/liuscraft/orion-x/internal/config"
)

func TestLocalVoicebotResolver_BindingMatch(t *testing.T) {
	cfg := config.DefaultWSServerConfig()
	cfg.Voicebot.Profiles["bot-a"] = config.VoicebotSessionConfig{
		TTS: config.TTSConfig{Voice: "zhichu"},
	}
	cfg.Voicebot.LocalBindings["dev-1"] = "bot-a"

	resolver := NewLocalVoicebotResolver(cfg)
	resolved, profileID, ok, err := resolver.ResolveVoicebot(context.Background(), "dev-1")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected binding match")
	}
	if profileID != "bot-a" {
		t.Fatalf("expected bot-a, got %q", profileID)
	}
	if resolved.TTS.Voice != "zhichu" {
		t.Fatalf("expected voice override, got %q", resolved.TTS.Voice)
	}
}

func TestChainVoicebotResolver_SkipUnimplementedManager(t *testing.T) {
	cfg := config.DefaultWSServerConfig()
	if cfg.Voicebot.Default == nil {
		t.Fatalf("default voicebot should exist")
	}
	cfg.Voicebot.Default.TTS.Voice = "longxiaochun"

	resolver := NewChainVoicebotResolver(
		NewManagerVoicebotResolver(),
		NewLocalVoicebotResolver(cfg),
	)

	resolved, profileID, ok, err := resolver.ResolveVoicebot(context.Background(), "dev-miss")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected default resolve fallback")
	}
	if profileID != "default" {
		t.Fatalf("expected default profile id, got %q", profileID)
	}
	if resolved.TTS.Voice != "longxiaochun" {
		t.Fatalf("expected default voice override, got %q", resolved.TTS.Voice)
	}
}

func TestChainVoicebotResolver_NoProvisionedConfig(t *testing.T) {
	cfg := config.DefaultWSServerConfig()
	cfg.Voicebot.Default = nil

	resolver := NewChainVoicebotResolver(
		NewManagerVoicebotResolver(),
		NewLocalVoicebotResolver(cfg),
	)

	_, _, ok, err := resolver.ResolveVoicebot(context.Background(), "dev-miss")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if ok {
		t.Fatalf("expected unresolved when no default and no binding")
	}
}
