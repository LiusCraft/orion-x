package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/text"
	"github.com/liuscraft/orion-x/internal/tts"
)

func main() {
	inputText := flag.String("text", "Tell me how to cook tomato and egg stir-fry.", "Text to synthesize")
	chunkSize := flag.Int("chunk-size", 12, "Chunk size in runes for fake streaming")
	chunkDelay := flag.Duration("chunk-delay", 120*time.Millisecond, "Delay between chunks")
	model := flag.String("model", "cosyvoice-v3-flash", "TTS model name")
	voice := flag.String("voice", "longanyang", "Voice name")
	format := flag.String("format", "mp3", "Audio format (mp3/wav/pcm/opus)")
	sampleRate := flag.Int("sample-rate", 22050, "Sample rate in Hz")
	volume := flag.Int("volume", 50, "Volume (0-100)")
	rate := flag.Float64("rate", 1.0, "Speech rate")
	pitch := flag.Float64("pitch", 1.0, "Pitch")
	segmenter := flag.Bool("segmenter", true, "Enable sentence segmentation")
	segmenterMax := flag.Int("segmenter-max", 120, "Max runes per sentence when segmenting")
	endpoint := flag.String("endpoint", "", "WebSocket endpoint (optional)")
	workspace := flag.String("workspace", "", "DashScope workspace ID (optional)")
	output := flag.String("output", "", "Write audio to file instead of playing")
	player := flag.String("player", "ffplay", "Player executable for streaming playback")
	dataInspection := flag.Bool("data-inspection", true, "Enable X-DashScope-DataInspection header")
	flag.Parse()
	if err := logging.InitFromEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logging.Sync()
	logging.SetTraceID(logging.NewTraceID())

	apiKey := os.Getenv("DASHSCOPE_API_KEY")
	if apiKey == "" {
		logging.Fatalf("DASHSCOPE_API_KEY is not set")
	}

	cfg := tts.Config{
		APIKey:               apiKey,
		Endpoint:             strings.TrimSpace(*endpoint),
		Workspace:            strings.TrimSpace(*workspace),
		Model:                strings.TrimSpace(*model),
		Voice:                strings.TrimSpace(*voice),
		Format:               strings.TrimSpace(*format),
		SampleRate:           *sampleRate,
		Volume:               *volume,
		Rate:                 *rate,
		Pitch:                *pitch,
		EnableDataInspection: dataInspection,
	}

	provider := tts.NewDashScopeProvider()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	stream, err := provider.Start(ctx, cfg)
	if err != nil {
		logging.Fatalf("start tts stream failed: %v", err)
	}

	var wg sync.WaitGroup
	playErrCh := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		playErrCh <- playAudio(ctx, stream.AudioReader(), *output, *player)
	}()

	var seg *text.Segmenter
	if *segmenter {
		seg = text.NewSegmenter(*segmenterMax)
	}

	for _, chunk := range chunkText(*inputText, *chunkSize) {
		if seg == nil {
			if err := stream.WriteTextChunk(ctx, chunk); err != nil {
				logging.Fatalf("send text chunk failed: %v", err)
			}
		} else {
			for _, sentence := range seg.Feed(chunk) {
				if err := stream.WriteTextChunk(ctx, sentence); err != nil {
					logging.Fatalf("send text chunk failed: %v", err)
				}
			}
		}
		if *chunkDelay > 0 {
			time.Sleep(*chunkDelay)
		}
	}

	if seg != nil {
		if sentence := seg.Flush(); sentence != "" {
			if err := stream.WriteTextChunk(ctx, sentence); err != nil {
				logging.Fatalf("send text chunk failed: %v", err)
			}
		}
	}

	finishCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := stream.Close(finishCtx); err != nil {
		logging.Errorf("finish task failed: %v", err)
	}

	wg.Wait()
	if err := <-playErrCh; err != nil {
		logging.Errorf("playback error: %v", err)
	}
}

func chunkText(text string, size int) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if size <= 0 {
		return []string{trimmed}
	}
	runes := []rune(trimmed)
	if len(runes) <= size {
		return []string{trimmed}
	}
	chunks := make([]string, 0, (len(runes)+size-1)/size)
	for i := 0; i < len(runes); i += size {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		chunk := strings.TrimSpace(string(runes[i:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

func playAudio(ctx context.Context, reader io.ReadCloser, outputPath, player string) error {
	defer reader.Close()
	if outputPath != "" {
		file, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, reader)
		return err
	}

	if player == "" {
		return io.ErrClosedPipe
	}
	path, err := exec.LookPath(player)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, path, "-autoexit", "-nodisp", "-loglevel", "warning", "-i", "pipe:0")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}

	_, copyErr := io.Copy(stdin, reader)
	_ = stdin.Close()
	waitErr := cmd.Wait()
	if copyErr != nil {
		return copyErr
	}
	return waitErr
}
