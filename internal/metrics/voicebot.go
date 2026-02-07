package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// VoicebotMetrics holds LLM/ASR/TTS/Tool metrics.
type VoicebotMetrics struct {
	llmRequestsTotal          *prometheus.CounterVec
	llmLatency                prometheus.Histogram
	llmTTFB                   prometheus.Histogram
	asrResultsTotal           *prometheus.CounterVec
	asrAudioBytesTotal        prometheus.Counter
	asrErrorsTotal            *prometheus.CounterVec
	ttsRequestsTotal          *prometheus.CounterVec
	ttsSentencesStartedTotal  prometheus.Counter
	ttsSentencesFinishedTotal prometheus.Counter
	ttsInterruptsTotal        prometheus.Counter
	ttsQueueSize              prometheus.Gauge
	ttsBufferSize             prometheus.Gauge
	ttsIsPlaying              prometheus.Gauge
	toolCallsTotal            *prometheus.CounterVec
	toolLatency               *prometheus.HistogramVec
	turnASRToTTSStart         prometheus.Histogram
}

// NewVoicebotMetrics registers and returns voicebot metrics.
func NewVoicebotMetrics(reg prometheus.Registerer) *VoicebotMetrics {
	if reg == nil {
		return nil
	}

	m := &VoicebotMetrics{
		llmRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "llm",
			Name:      "requests_total",
			Help:      "Total LLM requests by result.",
		}, []string{"result"}),
		llmLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "orion",
			Subsystem: "llm",
			Name:      "latency_seconds",
			Help:      "LLM end-to-end latency in seconds.",
			Buckets:   prometheus.DefBuckets,
		}),
		llmTTFB: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "orion",
			Subsystem: "llm",
			Name:      "time_to_first_token_seconds",
			Help:      "Time to first token from LLM in seconds.",
			Buckets:   prometheus.DefBuckets,
		}),
		asrResultsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "asr",
			Name:      "results_total",
			Help:      "Total ASR results by final flag.",
		}, []string{"final"}),
		asrAudioBytesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "asr",
			Name:      "audio_bytes_total",
			Help:      "Total ASR audio bytes sent.",
		}),
		asrErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "asr",
			Name:      "errors_total",
			Help:      "Total ASR errors by stage.",
		}, []string{"stage"}),
		ttsRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "tts",
			Name:      "requests_total",
			Help:      "Total TTS requests by result.",
		}, []string{"result"}),
		ttsSentencesStartedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "tts",
			Name:      "sentences_started_total",
			Help:      "Total TTS sentences started.",
		}),
		ttsSentencesFinishedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "tts",
			Name:      "sentences_finished_total",
			Help:      "Total TTS sentences finished.",
		}),
		ttsInterruptsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "tts",
			Name:      "interrupts_total",
			Help:      "Total TTS interrupts.",
		}),
		ttsQueueSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "orion",
			Subsystem: "tts",
			Name:      "queue_size",
			Help:      "Current TTS text queue size.",
		}),
		ttsBufferSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "orion",
			Subsystem: "tts",
			Name:      "buffer_size",
			Help:      "Current TTS buffer size.",
		}),
		ttsIsPlaying: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "orion",
			Subsystem: "tts",
			Name:      "is_playing",
			Help:      "Whether TTS is currently playing (1/0).",
		}),
		toolCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "tool",
			Name:      "calls_total",
			Help:      "Total tool calls by tool and result.",
		}, []string{"tool", "result"}),
		toolLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "orion",
			Subsystem: "tool",
			Name:      "latency_seconds",
			Help:      "Tool execution latency in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"tool"}),
		turnASRToTTSStart: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "orion",
			Subsystem: "turn",
			Name:      "asr_to_tts_start_seconds",
			Help:      "Latency from ASR final to TTS start per turn.",
			Buckets:   prometheus.DefBuckets,
		}),
	}

	reg.MustRegister(
		m.llmRequestsTotal,
		m.llmLatency,
		m.llmTTFB,
		m.asrResultsTotal,
		m.asrAudioBytesTotal,
		m.asrErrorsTotal,
		m.ttsRequestsTotal,
		m.ttsSentencesStartedTotal,
		m.ttsSentencesFinishedTotal,
		m.ttsInterruptsTotal,
		m.ttsQueueSize,
		m.ttsBufferSize,
		m.ttsIsPlaying,
		m.toolCallsTotal,
		m.toolLatency,
		m.turnASRToTTSStart,
	)

	return m
}

func (m *VoicebotMetrics) IncLLMRequests(result string) {
	if m == nil {
		return
	}
	m.llmRequestsTotal.WithLabelValues(result).Inc()
}

func (m *VoicebotMetrics) ObserveLLMLatency(duration time.Duration) {
	if m == nil {
		return
	}
	m.llmLatency.Observe(duration.Seconds())
}

func (m *VoicebotMetrics) ObserveLLMTimeToFirstToken(duration time.Duration) {
	if m == nil {
		return
	}
	m.llmTTFB.Observe(duration.Seconds())
}

func (m *VoicebotMetrics) IncASRResult(final bool) {
	if m == nil {
		return
	}
	label := "false"
	if final {
		label = "true"
	}
	m.asrResultsTotal.WithLabelValues(label).Inc()
}

func (m *VoicebotMetrics) AddASRAudioBytes(n int) {
	if m == nil || n <= 0 {
		return
	}
	m.asrAudioBytesTotal.Add(float64(n))
}

func (m *VoicebotMetrics) IncASRError(stage string) {
	if m == nil {
		return
	}
	m.asrErrorsTotal.WithLabelValues(stage).Inc()
}

func (m *VoicebotMetrics) IncTTSRequests(result string) {
	if m == nil {
		return
	}
	m.ttsRequestsTotal.WithLabelValues(result).Inc()
}

func (m *VoicebotMetrics) IncTTSSentencesStarted() {
	if m == nil {
		return
	}
	m.ttsSentencesStartedTotal.Inc()
}

func (m *VoicebotMetrics) IncTTSSentencesFinished() {
	if m == nil {
		return
	}
	m.ttsSentencesFinishedTotal.Inc()
}

func (m *VoicebotMetrics) IncTTSInterrupts() {
	if m == nil {
		return
	}
	m.ttsInterruptsTotal.Inc()
}

func (m *VoicebotMetrics) SetTTSQueueSize(size int) {
	if m == nil {
		return
	}
	m.ttsQueueSize.Set(float64(size))
}

func (m *VoicebotMetrics) SetTTSBufferSize(size int) {
	if m == nil {
		return
	}
	m.ttsBufferSize.Set(float64(size))
}

func (m *VoicebotMetrics) SetTTSIsPlaying(isPlaying bool) {
	if m == nil {
		return
	}
	if isPlaying {
		m.ttsIsPlaying.Set(1)
	} else {
		m.ttsIsPlaying.Set(0)
	}
}

func (m *VoicebotMetrics) IncToolCalls(tool string, result string) {
	if m == nil {
		return
	}
	m.toolCallsTotal.WithLabelValues(tool, result).Inc()
}

func (m *VoicebotMetrics) ObserveToolLatency(tool string, duration time.Duration) {
	if m == nil {
		return
	}
	m.toolLatency.WithLabelValues(tool).Observe(duration.Seconds())
}

func (m *VoicebotMetrics) ObserveTurnASRToTTSStart(duration time.Duration) {
	if m == nil {
		return
	}
	m.turnASRToTTSStart.Observe(duration.Seconds())
}
