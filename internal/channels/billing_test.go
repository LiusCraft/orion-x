package channels

import (
	"context"
	"testing"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/billing"
)

// fakeBillingSession 记录通道层映射出来的计费项，用来验证 §13 的口径。
type fakeBillingSession struct {
	records map[string]int64
	dims    map[string]map[string]any
}

func newFakeBillingSession() *fakeBillingSession {
	return &fakeBillingSession{records: map[string]int64{}, dims: map[string]map[string]any{}}
}

func (f *fakeBillingSession) Authorize(context.Context) (billing.AuthorizeResponse, error) {
	return billing.AuthorizeResponse{Allowed: true}, nil
}
func (f *fakeBillingSession) Start(context.Context) {}
func (f *fakeBillingSession) SetTurn(int64)         {}
func (f *fakeBillingSession) NearLimit() bool       { return false }
func (f *fakeBillingSession) Exhausted() bool       { return false }
func (f *fakeBillingSession) Close()                {}
func (f *fakeBillingSession) Flush(context.Context) error {
	return nil
}
func (f *fakeBillingSession) Settle(context.Context, string) (billing.SettleResponse, error) {
	return billing.SettleResponse{}, nil
}

func (f *fakeBillingSession) Record(itemCode string, quantity int64, dims map[string]any) {
	f.records[itemCode] += quantity
	f.dims[itemCode] = dims
}

func TestRecordLLMUsageMapping(t *testing.T) {
	usage := agent.Usage{
		InputTokens:      100,
		OutputTokens:     20,
		ReasoningTokens:  7,
		CacheReadTokens:  30,
		CacheWriteTokens: 5,
		Steps:            2,
	}

	t.Run("all quantities go to distinct items", func(t *testing.T) {
		sess := newFakeBillingSession()
		RecordLLMUsage(sess, usage, nil)

		want := map[string]int64{
			billing.ItemLLMInput:      100,
			billing.ItemLLMOutput:     20,
			billing.ItemLLMReasoning:  7,
			billing.ItemLLMCacheRead:  30,
			billing.ItemLLMCacheWrite: 5,
		}
		if len(sess.records) != len(want) {
			t.Fatalf("recorded %d items, want %d: %+v", len(sess.records), len(want), sess.records)
		}
		for code, quantity := range want {
			if sess.records[code] != quantity {
				t.Fatalf("item %s = %d, want %d", code, sess.records[code], quantity)
			}
		}
		if step, ok := sess.dims[billing.ItemLLMInput]["step"].(int64); !ok || step != 2 {
			t.Fatalf("step dimension = %+v, want 2", sess.dims[billing.ItemLLMInput])
		}
	})

	t.Run("zero quantities produce no events", func(t *testing.T) {
		sess := newFakeBillingSession()
		RecordLLMUsage(sess, agent.Usage{InputTokens: 10, Steps: 1}, nil)
		if len(sess.records) != 1 || sess.records[billing.ItemLLMInput] != 10 {
			t.Fatalf("records = %+v, want only the input item", sess.records)
		}
	})

	t.Run("zero usage is a no-op", func(t *testing.T) {
		sess := newFakeBillingSession()
		RecordLLMUsage(sess, agent.Usage{}, nil)
		if len(sess.records) != 0 {
			t.Fatalf("records = %+v, want none", sess.records)
		}
	})

	t.Run("nil sink is safe", func(t *testing.T) {
		RecordLLMUsage(nil, usage, nil)
	})
}

func TestRecordTTSAndASR(t *testing.T) {
	sess := newFakeBillingSession()
	RecordTTSSynthesis(sess, 12, nil)
	RecordTTSynthesis := sess.records[billing.ItemTTSCharacters]
	if RecordTTSynthesis != 12 {
		t.Fatalf("tts characters = %d, want 12", RecordTTSynthesis)
	}

	RecordASRSeconds(sess, 30, nil)
	if got := sess.records[billing.ItemASRAudioSecond]; got != 30 {
		t.Fatalf("asr seconds = %d, want 30", got)
	}

	// 0 与负值都不发事件（免得给事件表塞一堆 0）。
	RecordTTSSynthesis(sess, 0, nil)
	RecordASRSeconds(sess, -5, nil)
	if sess.records[billing.ItemTTSCharacters] != 12 || sess.records[billing.ItemASRAudioSecond] != 30 {
		t.Fatalf("zero/negative quantities changed the buffer: %+v", sess.records)
	}

	RecordTTSSynthesis(nil, 5, nil)
	RecordASRSeconds(nil, 5, nil)
}

func TestWithDimsCopiesBase(t *testing.T) {
	base := map[string]any{"source": "main"}
	out := withDims(base, "step", int64(1))
	if len(out) != 2 || out["source"] != "main" || out["step"] != int64(1) {
		t.Fatalf("withDims = %+v", out)
	}
	if len(base) != 1 {
		t.Fatalf("withDims mutated the caller's map: %+v", base)
	}
	if got := withDims(nil, "step", 1); got["step"] != 1 {
		t.Fatalf("withDims(nil) = %+v", got)
	}
}
