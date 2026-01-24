package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/liuscraft/orion-x/internal/asr"
)

const (
	defaultSampleRate     = 16000
	defaultFramesPerBlock = 3200
)

func main() {
	model := flag.String("model", "fun-asr-realtime", "ASR model name")
	endpoint := flag.String("endpoint", "", "WebSocket endpoint (optional)")
	sampleRate := flag.Int("sample-rate", defaultSampleRate, "Sample rate in Hz")
	framesPerBuffer := flag.Int("frames", defaultFramesPerBlock, "Frames per buffer (samples)")
	semanticPunc := flag.Bool("semantic-punctuation", false, "Enable semantic punctuation")
	languageHints := flag.String("language-hints", "", "Comma-separated language hints (e.g. zh,en)")
	flag.Parse()

	apiKey := os.Getenv("DASHSCOPE_API_KEY")
	if apiKey == "" {
		log.Fatal("DASHSCOPE_API_KEY is not set")
	}

	cfg := asr.Config{
		APIKey:     apiKey,
		Endpoint:   strings.TrimSpace(*endpoint),
		Model:      strings.TrimSpace(*model),
		Format:     "pcm",
		SampleRate: *sampleRate,
	}
	if *semanticPunc {
		enabled := true
		cfg.SemanticPunctuationEnabled = &enabled
	}
	if strings.TrimSpace(*languageHints) != "" {
		cfg.LanguageHints = splitComma(*languageHints)
	}

	recognizer, err := asr.NewDashScopeRecognizer(cfg)
	if err != nil {
		log.Fatalf("init recognizer failed: %v", err)
	}
	recognizer.OnResult(func(result asr.Result) {
		label := "partial"
		if result.IsFinal {
			label = "final"
		}
		fmt.Printf("%s: %s\n", label, result.Text)
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := recognizer.Start(ctx); err != nil {
		log.Fatalf("start recognizer failed: %v", err)
	}
	defer func() {
		finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := recognizer.Finish(finishCtx); err != nil {
			log.Printf("finish task failed: %v", err)
		}
		if err := recognizer.Close(); err != nil {
			log.Printf("close recognizer failed: %v", err)
		}
	}()

	if err := portaudio.Initialize(); err != nil {
		log.Fatalf("portaudio init failed: %v", err)
	}
	defer portaudio.Terminate()

	buffer := make([]int16, *framesPerBuffer)
	byteBuffer := make([]byte, len(buffer)*2)
	stream, err := portaudio.OpenDefaultStream(1, 0, float64(*sampleRate), len(buffer), &buffer)
	if err != nil {
		log.Fatalf("open audio stream failed: %v", err)
	}
	defer stream.Close()
	if err := stream.Start(); err != nil {
		log.Fatalf("start audio stream failed: %v", err)
	}
	defer stream.Stop()

	log.Println("listening... press Ctrl+C to stop")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := stream.Read(); err != nil {
			log.Printf("audio read error: %v", err)
			return
		}
		encodeInt16LE(byteBuffer, buffer)
		if err := recognizer.SendAudio(ctx, byteBuffer); err != nil {
			log.Printf("send audio error: %v", err)
			return
		}
	}
}

func encodeInt16LE(dst []byte, src []int16) {
	for i, v := range src {
		binary.LittleEndian.PutUint16(dst[i*2:], uint16(v))
	}
}

func splitComma(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
