package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestWSServerMetricsCounters(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWSServerMetrics(reg)
	if m == nil {
		t.Fatalf("expected metrics instance")
	}

	m.IncConnection("success")
	m.IncMessagesIn("text")
	m.IncMessagesOut("binary")
	m.IncReadError("timeout")
	m.IncWriteError("other")
	m.IncWriteQueueDropped()

	if got := testutil.ToFloat64(m.connectionsTotal.WithLabelValues("success")); got != 1 {
		t.Fatalf("connections_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.messagesInTotal.WithLabelValues("text")); got != 1 {
		t.Fatalf("messages_in_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.messagesOutTotal.WithLabelValues("binary")); got != 1 {
		t.Fatalf("messages_out_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.readErrorsTotal.WithLabelValues("timeout")); got != 1 {
		t.Fatalf("read_errors_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.writeErrorsTotal.WithLabelValues("other")); got != 1 {
		t.Fatalf("write_errors_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.writeQueueDroppedTotal); got != 1 {
		t.Fatalf("write_queue_dropped_total expected 1, got %v", got)
	}
}

func TestVoicebotMetricsCounters(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewVoicebotMetrics(reg)
	if m == nil {
		t.Fatalf("expected metrics instance")
	}

	m.IncLLMRequests("success")
	m.IncASRResult(true)
	m.IncASRError("send")
	m.IncTTSRequests("error")
	m.IncTTSSentencesStarted()
	m.IncTTSSentencesFinished()
	m.IncTTSInterrupts()
	m.SetTTSQueueSize(3)
	m.SetTTSBufferSize(2)
	m.SetTTSIsPlaying(true)
	m.IncToolCalls("getTime", "success")

	if got := testutil.ToFloat64(m.llmRequestsTotal.WithLabelValues("success")); got != 1 {
		t.Fatalf("llm_requests_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.asrResultsTotal.WithLabelValues("true")); got != 1 {
		t.Fatalf("asr_results_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.asrErrorsTotal.WithLabelValues("send")); got != 1 {
		t.Fatalf("asr_errors_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.ttsRequestsTotal.WithLabelValues("error")); got != 1 {
		t.Fatalf("tts_requests_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.ttsSentencesStartedTotal); got != 1 {
		t.Fatalf("tts_sentences_started_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.ttsSentencesFinishedTotal); got != 1 {
		t.Fatalf("tts_sentences_finished_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.ttsInterruptsTotal); got != 1 {
		t.Fatalf("tts_interrupts_total expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.ttsQueueSize); got != 3 {
		t.Fatalf("tts_queue_size expected 3, got %v", got)
	}
	if got := testutil.ToFloat64(m.ttsBufferSize); got != 2 {
		t.Fatalf("tts_buffer_size expected 2, got %v", got)
	}
	if got := testutil.ToFloat64(m.ttsIsPlaying); got != 1 {
		t.Fatalf("tts_is_playing expected 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.toolCallsTotal.WithLabelValues("getTime", "success")); got != 1 {
		t.Fatalf("tool_calls_total expected 1, got %v", got)
	}
}
