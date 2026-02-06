package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// WSServerMetrics holds WebSocket server metrics.
type WSServerMetrics struct {
	connectionsTotal       *prometheus.CounterVec
	handshakeDuration      *prometheus.HistogramVec
	activeSessions         prometheus.Gauge
	sessionDuration        prometheus.Histogram
	messagesInTotal        *prometheus.CounterVec
	messagesOutTotal       *prometheus.CounterVec
	audioInBytesTotal      prometheus.Counter
	audioOutBytesTotal     prometheus.Counter
	readErrorsTotal        *prometheus.CounterVec
	writeErrorsTotal       *prometheus.CounterVec
	writeQueueDroppedTotal prometheus.Counter
}

// NewWSServerMetrics registers and returns WebSocket metrics.
func NewWSServerMetrics(reg prometheus.Registerer) *WSServerMetrics {
	if reg == nil {
		return nil
	}

	m := &WSServerMetrics{
		connectionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "ws",
			Name:      "connections_total",
			Help:      "Total WebSocket connection attempts by result.",
		}, []string{"result"}),
		handshakeDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "orion",
			Subsystem: "ws",
			Name:      "handshake_duration_seconds",
			Help:      "WebSocket handshake duration in seconds by result.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"result"}),
		activeSessions: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "orion",
			Subsystem: "ws",
			Name:      "active_sessions",
			Help:      "Current active WebSocket sessions.",
		}),
		sessionDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "orion",
			Subsystem: "ws",
			Name:      "session_duration_seconds",
			Help:      "WebSocket session duration in seconds.",
			Buckets:   prometheus.ExponentialBuckets(1, 2, 10),
		}),
		messagesInTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "ws",
			Name:      "messages_in_total",
			Help:      "Total inbound WebSocket messages by type.",
		}, []string{"type"}),
		messagesOutTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "ws",
			Name:      "messages_out_total",
			Help:      "Total outbound WebSocket messages by type.",
		}, []string{"type"}),
		audioInBytesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "ws",
			Name:      "audio_in_bytes_total",
			Help:      "Total inbound audio bytes from WebSocket.",
		}),
		audioOutBytesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "ws",
			Name:      "audio_out_bytes_total",
			Help:      "Total outbound audio bytes to WebSocket.",
		}),
		readErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "ws",
			Name:      "read_errors_total",
			Help:      "Total WebSocket read errors by kind.",
		}, []string{"kind"}),
		writeErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "ws",
			Name:      "write_errors_total",
			Help:      "Total WebSocket write errors by kind.",
		}, []string{"kind"}),
		writeQueueDroppedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "orion",
			Subsystem: "ws",
			Name:      "write_queue_dropped_total",
			Help:      "Total dropped WebSocket outbound messages due to full queue.",
		}),
	}

	reg.MustRegister(
		m.connectionsTotal,
		m.handshakeDuration,
		m.activeSessions,
		m.sessionDuration,
		m.messagesInTotal,
		m.messagesOutTotal,
		m.audioInBytesTotal,
		m.audioOutBytesTotal,
		m.readErrorsTotal,
		m.writeErrorsTotal,
		m.writeQueueDroppedTotal,
	)

	return m
}

func (m *WSServerMetrics) IncConnection(result string) {
	if m == nil {
		return
	}
	m.connectionsTotal.WithLabelValues(result).Inc()
}

func (m *WSServerMetrics) ObserveHandshake(result string, duration time.Duration) {
	if m == nil {
		return
	}
	m.handshakeDuration.WithLabelValues(result).Observe(duration.Seconds())
}

func (m *WSServerMetrics) SetActiveSessions(count int) {
	if m == nil {
		return
	}
	m.activeSessions.Set(float64(count))
}

func (m *WSServerMetrics) ObserveSessionDuration(duration time.Duration) {
	if m == nil {
		return
	}
	m.sessionDuration.Observe(duration.Seconds())
}

func (m *WSServerMetrics) IncMessagesIn(msgType string) {
	if m == nil {
		return
	}
	m.messagesInTotal.WithLabelValues(msgType).Inc()
}

func (m *WSServerMetrics) IncMessagesOut(msgType string) {
	if m == nil {
		return
	}
	m.messagesOutTotal.WithLabelValues(msgType).Inc()
}

func (m *WSServerMetrics) AddAudioInBytes(n int) {
	if m == nil || n <= 0 {
		return
	}
	m.audioInBytesTotal.Add(float64(n))
}

func (m *WSServerMetrics) AddAudioOutBytes(n int) {
	if m == nil || n <= 0 {
		return
	}
	m.audioOutBytesTotal.Add(float64(n))
}

func (m *WSServerMetrics) IncReadError(kind string) {
	if m == nil {
		return
	}
	m.readErrorsTotal.WithLabelValues(kind).Inc()
}

func (m *WSServerMetrics) IncWriteError(kind string) {
	if m == nil {
		return
	}
	m.writeErrorsTotal.WithLabelValues(kind).Inc()
}

func (m *WSServerMetrics) IncWriteQueueDropped() {
	if m == nil {
		return
	}
	m.writeQueueDroppedTotal.Inc()
}
