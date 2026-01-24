package logging

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestStartTurnAddsLogFields(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	baseLogger = zap.New(core)
	sugar = baseLogger.Sugar()
	traceID.Store("")
	turnID = 0

	SetTraceID("trace-123")
	StartTurn()
	Infof("hello")

	logs := recorded.All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}

	fields := map[string]interface{}{}
	for _, field := range logs[0].Context {
		fields[field.Key] = field.Interface
		if field.Type == zapcore.StringType {
			fields[field.Key] = field.String
		}
		if field.Type == zapcore.Int64Type {
			fields[field.Key] = field.Integer
		}
	}

	if fields["trace_id"] != "trace-123" {
		t.Fatalf("expected trace_id to be trace-123, got %v", fields["trace_id"])
	}
	if fields["turn_id"] != int64(1) {
		t.Fatalf("expected turn_id to be 1, got %v", fields["turn_id"])
	}
	if fields["log_id"] != "trace-123-1" {
		t.Fatalf("expected log_id to be trace-123-1, got %v", fields["log_id"])
	}
}
