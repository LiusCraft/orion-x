package metrics

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/tools"
)

type agentRunner interface {
	Process(ctx context.Context, input string) (<-chan agent.AgentEvent, error)
	SummarizeToolResult(ctx context.Context, tool string, args map[string]interface{}, result interface{}) (<-chan agent.AgentEvent, error)
}

// NewInstrumentedAgent wraps an Agent with metrics.
func NewInstrumentedAgent(next agentRunner, m *VoicebotMetrics) *InstrumentedAgent {
	if next == nil || m == nil {
		return nil
	}
	return &InstrumentedAgent{next: next, metrics: m}
}

type InstrumentedAgent struct {
	next    agentRunner
	metrics *VoicebotMetrics
}

func (i *InstrumentedAgent) Process(ctx context.Context, input string) (<-chan agent.AgentEvent, error) {
	start := time.Now()
	eventChan, err := i.next.Process(ctx, input)
	if err != nil {
		i.metrics.IncLLMRequests(resultLabelFromErr(err))
		i.metrics.ObserveLLMLatency(time.Since(start))
		return nil, err
	}

	out := make(chan agent.AgentEvent)
	go func() {
		defer close(out)

		var firstTokenOnce sync.Once
		finished := false
		for event := range eventChan {
			switch e := event.(type) {
			case *agent.TextChunkEvent:
				firstTokenOnce.Do(func() {
					i.metrics.ObserveLLMTimeToFirstToken(time.Since(start))
				})
			case *agent.FinishedEvent:
				finished = true
				i.metrics.IncLLMRequests(resultLabelFromErr(e.Error))
				i.metrics.ObserveLLMLatency(time.Since(start))
			}
			out <- event
		}

		if !finished {
			i.metrics.IncLLMRequests("error")
			i.metrics.ObserveLLMLatency(time.Since(start))
		}
	}()

	return out, nil
}

func (i *InstrumentedAgent) SummarizeToolResult(ctx context.Context, tool string, args map[string]interface{}, result interface{}) (<-chan agent.AgentEvent, error) {
	return i.next.SummarizeToolResult(ctx, tool, args, result)
}

// NewInstrumentedToolExecutor wraps a ToolExecutor with metrics.
func NewInstrumentedToolExecutor(next tools.ToolExecutor, m *VoicebotMetrics) tools.ToolExecutor {
	if next == nil || m == nil {
		return next
	}
	return &instrumentedToolExecutor{next: next, metrics: m}
}

type instrumentedToolExecutor struct {
	next    tools.ToolExecutor
	metrics *VoicebotMetrics
}

func (i *instrumentedToolExecutor) Execute(tool string, args map[string]interface{}) (interface{}, io.Reader, error) {
	start := time.Now()
	result, audioReader, err := i.next.Execute(tool, args)
	if err != nil {
		i.metrics.IncToolCalls(tool, "error")
		i.metrics.ObserveToolLatency(tool, time.Since(start))
		return result, audioReader, err
	}
	i.metrics.IncToolCalls(tool, "success")
	i.metrics.ObserveToolLatency(tool, time.Since(start))
	return result, audioReader, nil
}

func (i *instrumentedToolExecutor) RegisterTool(name string, executor tools.ToolExecutorFunc) {
	i.next.RegisterTool(name, executor)
}

// NewInstrumentedAudioInPipe wraps AudioInPipe with metrics.
func NewInstrumentedAudioInPipe(next audio.AudioInPipe, m *VoicebotMetrics) audio.AudioInPipe {
	if next == nil || m == nil {
		return next
	}
	return &instrumentedAudioInPipe{next: next, metrics: m}
}

type instrumentedAudioInPipe struct {
	next    audio.AudioInPipe
	metrics *VoicebotMetrics
}

func (i *instrumentedAudioInPipe) Start(ctx context.Context) error {
	if err := i.next.Start(ctx); err != nil {
		i.metrics.IncASRError("start")
		return err
	}
	return nil
}

func (i *instrumentedAudioInPipe) Stop() error {
	if err := i.next.Stop(); err != nil {
		i.metrics.IncASRError("finish")
		return err
	}
	return nil
}

func (i *instrumentedAudioInPipe) SendAudio(audioBytes []byte) error {
	err := i.next.SendAudio(audioBytes)
	if err != nil && !errors.Is(err, context.Canceled) {
		i.metrics.IncASRError("send")
		return err
	}
	if err == nil {
		i.metrics.AddASRAudioBytes(len(audioBytes))
	}
	return err
}

func (i *instrumentedAudioInPipe) OnASRResult(handler func(text string, isFinal bool)) {
	i.next.OnASRResult(func(text string, isFinal bool) {
		i.metrics.IncASRResult(isFinal)
		if handler != nil {
			handler(text, isFinal)
		}
	})
}

func (i *instrumentedAudioInPipe) OnUserSpeakingDetected(handler func()) {
	i.next.OnUserSpeakingDetected(handler)
}

// NewInstrumentedAudioOutPipe wraps AudioOutPipe with metrics.
func NewInstrumentedAudioOutPipe(next audio.AudioOutPipe, m *VoicebotMetrics) audio.AudioOutPipe {
	if next == nil || m == nil {
		return next
	}
	return &instrumentedAudioOutPipe{next: next, metrics: m}
}

type instrumentedAudioOutPipe struct {
	next    audio.AudioOutPipe
	metrics *VoicebotMetrics
}

func (i *instrumentedAudioOutPipe) Start(ctx context.Context) error {
	if err := i.next.Start(ctx); err != nil {
		return err
	}
	i.startStatsLoop(ctx)
	return nil
}

func (i *instrumentedAudioOutPipe) Stop() error {
	if err := i.next.Stop(); err != nil {
		return err
	}
	i.metrics.SetTTSQueueSize(0)
	i.metrics.SetTTSBufferSize(0)
	i.metrics.SetTTSIsPlaying(false)
	return nil
}

func (i *instrumentedAudioOutPipe) PlayTTS(text string, emotion string) error {
	if err := i.next.PlayTTS(text, emotion); err != nil {
		i.metrics.IncTTSRequests("error")
		return err
	}
	i.metrics.IncTTSRequests("success")
	return nil
}

func (i *instrumentedAudioOutPipe) PlayResource(audioReader io.Reader) error {
	return i.next.PlayResource(audioReader)
}

func (i *instrumentedAudioOutPipe) BeginTTSStream(emotion string) error {
	return i.next.BeginTTSStream(emotion)
}

func (i *instrumentedAudioOutPipe) WriteTTSChunk(chunk string) error {
	return i.next.WriteTTSChunk(chunk)
}

func (i *instrumentedAudioOutPipe) EndTTSStream() error {
	return i.next.EndTTSStream()
}

func (i *instrumentedAudioOutPipe) Interrupt() error {
	err := i.next.Interrupt()
	i.metrics.IncTTSInterrupts()
	return err
}

func (i *instrumentedAudioOutPipe) SetMixer(mixer audio.AudioMixer) {
	i.next.SetMixer(mixer)
}

func (i *instrumentedAudioOutPipe) SetOnPlaybackFinished(callback audio.PlaybackFinishedCallback) {
	i.next.SetOnPlaybackFinished(func() {
		i.metrics.IncTTSSentencesFinished()
		if callback != nil {
			callback()
		}
	})
}

func (i *instrumentedAudioOutPipe) SetOnTTSItemStarted(callback audio.TTSItemStartedCallback) {
	i.next.SetOnTTSItemStarted(func(text string, emotion string) {
		i.metrics.IncTTSSentencesStarted()
		if callback != nil {
			callback(text, emotion)
		}
	})
}

func (i *instrumentedAudioOutPipe) Stats() audio.PipelineStats {
	return i.next.Stats()
}

func (i *instrumentedAudioOutPipe) startStatsLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := i.next.Stats()
				i.metrics.SetTTSQueueSize(stats.TextQueueSize)
				i.metrics.SetTTSBufferSize(stats.TTSBufferSize)
				i.metrics.SetTTSIsPlaying(stats.IsPlaying)
			}
		}
	}()
}

func resultLabelFromErr(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	return "error"
}
