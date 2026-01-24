package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

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

type ttsStreamReader struct {
	reader   io.Reader
	doneOnce sync.Once
	onDone   func()
}

func (r *ttsStreamReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err != nil {
		r.done()
	}
	return n, err
}

func (r *ttsStreamReader) done() {
	r.doneOnce.Do(func() {
		if r.onDone != nil {
			r.onDone()
		}
	})
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
	log.Printf("AudioOutPipe: PlayTTS - text: %s, emotion: %s, voice: %s", text, emotion, voice)

	p.mu.Lock()
	ctx := p.ctx
	mixer := p.mixer
	p.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	ttsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cfg := tts.Config{
		APIKey:     p.apiKey,
		Model:      "cosyvoice-v3-flash",
		Voice:      voice,
		Format:     "pcm",
		SampleRate: 16000,
		Volume:     50,
		Rate:       1.0,
		Pitch:      1.0,
		TextType:   "PlainText",
	}

	log.Printf("AudioOutPipe: starting TTS stream...")
	stream, err := p.tts.Start(ttsCtx, cfg)
	if err != nil {
		log.Printf("AudioOutPipe: TTS start error: %v (ctx=%v)", err, ttsCtx.Err())
		if isRetryableTTSError(err) {
			log.Printf("AudioOutPipe: retrying TTS start...")
			time.Sleep(300 * time.Millisecond)
			stream, err = p.tts.Start(ttsCtx, cfg)
		}
		if err != nil {
			log.Printf("AudioOutPipe: TTS start retry failed: %v (ctx=%v)", err, ttsCtx.Err())
			return fmt.Errorf("TTS start error: %w", err)
		}
	}

	p.mu.Lock()
	p.ttsStreams = append(p.ttsStreams, stream)
	p.mu.Unlock()

	audioReader := stream.AudioReader()
	wrappedReader := &ttsStreamReader{
		reader: audioReader,
	}
	wrappedReader.onDone = func() {
		if mixer != nil {
			mixer.OnTTSFinished()
			mixer.RemoveTTSStream()
		}
		p.removeStream(stream)
	}

	if mixer != nil {
		log.Printf("AudioOutPipe: adding TTS stream to mixer...")
		mixer.OnTTSStarted()
		mixer.AddTTSStream(wrappedReader)
	} else {
		go p.drainAudio(wrappedReader)
	}

	log.Printf("AudioOutPipe: writing text chunk to TTS...")
	if err := stream.WriteTextChunk(ttsCtx, text); err != nil {
		log.Printf("AudioOutPipe: TTS write error: %v", err)
		wrappedReader.done()
		return fmt.Errorf("TTS write error: %w", err)
	}

	log.Printf("AudioOutPipe: closing TTS stream...")
	if err := stream.Close(ttsCtx); err != nil {
		log.Printf("AudioOutPipe: TTS close error: %v", err)
		wrappedReader.done()
		return fmt.Errorf("TTS close error: %w", err)
	}

	log.Printf("AudioOutPipe: PlayTTS completed")
	return nil
}

func (p *outPipeImpl) PlayResource(audio io.Reader) error {
	if p.mixer == nil {
		return fmt.Errorf("mixer not set")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	log.Printf("AudioOutPipe: adding resource stream to mixer...")
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

func (p *outPipeImpl) removeStream(target tts.Stream) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.ttsStreams) == 0 {
		return
	}

	for i, stream := range p.ttsStreams {
		if stream == target {
			p.ttsStreams = append(p.ttsStreams[:i], p.ttsStreams[i+1:]...)
			return
		}
	}
}

func (p *outPipeImpl) drainAudio(reader io.Reader) {
	buf := make([]byte, 4096)
	for {
		if _, err := reader.Read(buf); err != nil {
			return
		}
	}
}

func isRetryableTTSError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

func (p *outPipeImpl) getVoice(emotion string) string {
	if voice, ok := p.voiceMap[emotion]; ok {
		return voice
	}
	return p.voiceMap["default"]
}
