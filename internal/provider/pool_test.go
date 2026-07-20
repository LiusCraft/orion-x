package provider

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/llm"
	llmprovider "github.com/liuscraft/orion-x/internal/llm/provider"
	"github.com/liuscraft/orion-x/internal/provider/asr"
	"github.com/liuscraft/orion-x/internal/provider/tts"
)

type poolRecognizer struct{ id int32 }

func (poolRecognizer) Start(context.Context) error             { return nil }
func (poolRecognizer) SendAudio(context.Context, []byte) error { return nil }
func (poolRecognizer) Finish(context.Context) error            { return nil }
func (poolRecognizer) Close() error                            { return nil }
func (poolRecognizer) OnResult(func(asr.Result))               {}

type poolTTSProvider struct{ id int32 }

func (poolTTSProvider) Synthesize(context.Context, string, tts.SynthesisOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

type poolLLMAdapter struct{}

func (poolLLMAdapter) Generate(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{}, nil
}
func (poolLLMAdapter) Stream(context.Context, llm.Request) (llm.Stream, error) {
	stream := llm.NewEventStream(nil)
	stream.Finish()
	return stream, nil
}

func TestPoolCachesTTSButCreatesASRPerSession(t *testing.T) {
	var asrConstructed, ttsConstructed int32
	asr.Register("pool-test-asr", func(asr.Config) (asr.Recognizer, error) {
		id := atomic.AddInt32(&asrConstructed, 1)
		return &poolRecognizer{id: id}, nil
	}, asr.ProviderMeta{})
	tts.Register("pool-test-tts", func(tts.Config) (tts.Provider, error) {
		id := atomic.AddInt32(&ttsConstructed, 1)
		return &poolTTSProvider{id: id}, nil
	}, tts.ProviderMeta{})

	pool := NewPool()
	firstASR, err := pool.GetOrCreateASR("pool-test-asr", config.ASRConfig{})
	if err != nil {
		t.Fatalf("GetOrCreateASR() error = %v", err)
	}
	secondASR, err := pool.GetOrCreateASR("pool-test-asr", config.ASRConfig{})
	if err != nil {
		t.Fatalf("GetOrCreateASR() second error = %v", err)
	}
	if firstASR == secondASR || atomic.LoadInt32(&asrConstructed) != 2 {
		t.Fatal("ASR recognizers must not be shared between streams")
	}

	firstTTS, err := pool.GetOrCreateTTS("pool-test-tts", config.TTSConfig{Model: "model"}, 16000)
	if err != nil {
		t.Fatalf("GetOrCreateTTS() error = %v", err)
	}
	secondTTS, err := pool.GetOrCreateTTS("pool-test-tts", config.TTSConfig{Model: "model"}, 16000)
	if err != nil {
		t.Fatalf("GetOrCreateTTS() second error = %v", err)
	}
	if firstTTS != secondTTS || atomic.LoadInt32(&ttsConstructed) != 1 {
		t.Fatal("TTS provider was not cached")
	}
}

func TestPoolCachesLLMClient(t *testing.T) {
	var constructed int32
	llmprovider.Register("pool-test-llm", func(context.Context, llmprovider.Config) (llmprovider.Adapter, error) {
		atomic.AddInt32(&constructed, 1)
		return poolLLMAdapter{}, nil
	}, llmprovider.ProviderMeta{})
	pool := NewPool()
	cfg := config.LLMConfig{APIKey: "key", BaseURL: "https://example.test", Model: "model"}
	first, err := pool.GetOrCreateLLM(context.Background(), "pool-test-llm", cfg)
	if err != nil {
		t.Fatalf("GetOrCreateLLM() error = %v", err)
	}
	second, err := pool.GetOrCreateLLM(context.Background(), "pool-test-llm", cfg)
	if err != nil {
		t.Fatalf("GetOrCreateLLM() second error = %v", err)
	}
	if first != second || atomic.LoadInt32(&constructed) != 1 {
		t.Fatal("LLM client was not cached")
	}
}
