package audio

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/liuscraft/orion-x/internal/tts"
)

type outPipeImpl struct {
	mixer      AudioMixer
	tts        tts.Provider
	ttsStreams []tts.Stream
	voiceMap   map[string]string
	apiKey     string
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
}

func NewOutPipe(apiKey string) AudioOutPipe {
	return &outPipeImpl{
		voiceMap: map[string]string{
			"happy":   "longanyang",
			"sad":     "zhichu",
			"angry":   "zhimeng",
			"calm":    "longxiaochun",
			"excited": "longanyang",
			"default": "longanyang",
		},
		apiKey: apiKey,
		tts:    tts.NewDashScopeProvider(),
	}
}

func (p *outPipeImpl) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ctx, p.cancel = context.WithCancel(ctx)
	log.Printf("AudioOutPipe: started")
	return nil
}

func (p *outPipeImpl) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancel != nil {
		p.cancel()
	}

	for _, stream := range p.ttsStreams {
		_ = stream.Close(p.ctx)
	}
	p.ttsStreams = nil

	log.Printf("AudioOutPipe: stopped")
	return nil
}

func (p *outPipeImpl) SetMixer(mixer AudioMixer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mixer = mixer
}

func (p *outPipeImpl) PlayTTS(text string, emotion string) error {
	if text == "" {
		return nil
	}

	voice := p.getVoice(emotion)

	cfg := tts.Config{
		APIKey:     p.apiKey,
		Model:      "cosyvoice-v3-flash",
		Voice:      voice,
		Format:     "mp3",
		SampleRate: 16000,
		Volume:     50,
		Rate:       1.0,
		Pitch:      1.0,
		TextType:   "PlainText",
	}

	stream, err := p.tts.Start(p.ctx, cfg)
	if err != nil {
		return fmt.Errorf("TTS start error: %w", err)
	}

	p.mu.Lock()
	p.ttsStreams = append(p.ttsStreams, stream)
	p.mu.Unlock()

	if err := stream.WriteTextChunk(p.ctx, text); err != nil {
		return fmt.Errorf("TTS write error: %w", err)
	}

	if err := stream.Close(p.ctx); err != nil {
		return fmt.Errorf("TTS close error: %w", err)
	}

	audioReader := stream.AudioReader()

	if p.mixer != nil {
		p.mixer.OnTTSStarted()
		p.mixer.AddTTSStream(audioReader)
		p.mixer.OnTTSFinished()
		p.mixer.RemoveTTSStream()
	}

	return nil
}

func (p *outPipeImpl) PlayResource(audio io.Reader) error {
	if p.mixer == nil {
		return fmt.Errorf("mixer not set")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.mixer.AddResourceStream(audio)
	return nil
}

func (p *outPipeImpl) Interrupt() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, stream := range p.ttsStreams {
		_ = stream.Close(p.ctx)
	}
	p.ttsStreams = nil

	if p.mixer != nil {
		p.mixer.RemoveTTSStream()
		p.mixer.RemoveResourceStream()
	}

	log.Printf("AudioOutPipe: interrupted")
	return nil
}

func (p *outPipeImpl) getVoice(emotion string) string {
	if voice, ok := p.voiceMap[emotion]; ok {
		return voice
	}
	return p.voiceMap["default"]
}
