package billing

import (
	"testing"
	"time"
)

func TestValidateTiers(t *testing.T) {
	tests := []struct {
		name    string
		tiers   []Tier
		wantErr bool
	}{
		{"empty", nil, false},
		{"single open-ended", []Tier{{UpTo: 0, UnitPriceMicro: 1}}, false},
		{"progressive", []Tier{{UpTo: 100, UnitPriceMicro: 3}, {UpTo: 0, UnitPriceMicro: 1}}, false},
		{"open-ended not last", []Tier{{UpTo: 0, UnitPriceMicro: 1}, {UpTo: 100, UnitPriceMicro: 1}}, true},
		{"not increasing", []Tier{{UpTo: 100, UnitPriceMicro: 1}, {UpTo: 100, UnitPriceMicro: 1}}, true},
		{"negative price", []Tier{{UpTo: 0, UnitPriceMicro: -1}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTiers(ItemLLMInput, tc.tiers)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateTiers() err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestPriceValidate(t *testing.T) {
	base := func() Price {
		return Price{
			ItemCode:       ItemLLMInput,
			ResourceType:   ResourceItem,
			UnitPriceMicro: 1,
			UnitSize:       1,
			Rounding:       RoundingNone,
		}
	}
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		mutate  func(*Price)
		wantErr bool
	}{
		{"ok", func(*Price) {}, false},
		{"item level with resource id", func(p *Price) { p.ResourceID = "m_1" }, true},
		{"model level without resource id", func(p *Price) { p.ResourceType = ResourceModel }, true},
		{"model level", func(p *Price) { p.ResourceType = ResourceModel; p.ResourceID = "m_1" }, false},
		{"zero unit size", func(p *Price) { p.UnitSize = 0 }, true},
		{"bad rounding", func(p *Price) { p.Rounding = "round:nearest" }, true},
		{"effective_to before from", func(p *Price) {
			p.EffectiveFrom = to
			p.EffectiveTo = &to
		}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := base()
			tc.mutate(&p)
			err := p.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestSelectPriority(t *testing.T) {
	from := func(t time.Time) time.Time { return t }
	day := func(n int) time.Time { return time.Date(2026, 1, n, 0, 0, 0, 0, time.UTC) }
	_ = from

	refs := ResourceRef{ProviderID: "p_1", ModelID: "m_1", VoiceID: "v_1"}
	at := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)

	itemPrice := Price{ID: "item", ResourceType: ResourceItem, AccountID: "", UnitPriceMicro: 10, UnitSize: 1, EffectiveFrom: day(1)}
	providerPrice := Price{ID: "provider", ResourceType: ResourceProvider, ResourceID: "p_1", UnitPriceMicro: 8, UnitSize: 1, EffectiveFrom: day(1)}
	modelPrice := Price{ID: "model", ResourceType: ResourceModel, ResourceID: "m_1", UnitPriceMicro: 6, UnitSize: 1, EffectiveFrom: day(1)}
	voicePrice := Price{ID: "voice", ResourceType: ResourceVoice, ResourceID: "v_1", UnitPriceMicro: 4, UnitSize: 1, EffectiveFrom: day(1)}
	accountVoicePrice := Price{ID: "account-voice", ResourceType: ResourceVoice, ResourceID: "v_1", AccountID: "acct_1", UnitPriceMicro: 2, UnitSize: 1, EffectiveFrom: day(2)}

	t.Run("most specific resource wins", func(t *testing.T) {
		got, ok := Select([]Price{itemPrice, providerPrice, modelPrice, voicePrice}, "", refs, at)
		if !ok || got.ID != "voice" {
			t.Fatalf("got %+v, want voice price", got)
		}
	})
	t.Run("account price wins", func(t *testing.T) {
		got, ok := Select([]Price{itemPrice, voicePrice, accountVoicePrice}, "acct_1", refs, at)
		if !ok || got.ID != "account-voice" {
			t.Fatalf("got %+v, want account price", got)
		}
	})
	t.Run("account price for another account is ignored", func(t *testing.T) {
		got, ok := Select([]Price{accountVoicePrice, voicePrice}, "acct_2", refs, at)
		if !ok || got.ID != "voice" {
			t.Fatalf("got %+v, want platform voice price", got)
		}
	})
	t.Run("latest version of the same scope wins", func(t *testing.T) {
		old := accountVoicePrice
		old.ID = "account-voice-old"
		old.EffectiveFrom = day(2)
		newer := accountVoicePrice
		newer.ID = "account-voice-new"
		newer.EffectiveFrom = day(5)
		got, ok := Select([]Price{old, newer}, "acct_1", refs, at)
		if !ok || got.ID != "account-voice-new" {
			t.Fatalf("got %+v, want newest version", got)
		}
	})
	t.Run("expired price is ignored", func(t *testing.T) {
		expired := voicePrice
		closed := day(5)
		expired.EffectiveTo = &closed
		got, ok := Select([]Price{expired, modelPrice}, "", refs, at)
		if !ok || got.ID != "model" {
			t.Fatalf("got %+v, want fallback model price", got)
		}
	})
	t.Run("no price", func(t *testing.T) {
		if _, ok := Select([]Price{modelPrice}, "", ResourceRef{ModelID: "other"}, at); ok {
			t.Fatal("expected no match")
		}
	})
	t.Run("resource_mismatch falls back to item price", func(t *testing.T) {
		got, ok := Select([]Price{itemPrice, modelPrice}, "", ResourceRef{ModelID: "m_2"}, at)
		if !ok || got.ID != "item" {
			t.Fatalf("got %+v, want item fallback", got)
		}
	})
}

func TestResourceRefPairs(t *testing.T) {
	refs := ResourceRef{ProviderID: "p", ModelID: "m", VoiceID: "v"}
	pairs := refs.Pairs()
	if len(pairs) != 3 {
		t.Fatalf("got %d pairs, want 3", len(pairs))
	}
	if refs.ID(ResourceVoice) != "v" || refs.ID(ResourceItem) != "" {
		t.Fatalf("unexpected ID resolution: %v %v", refs.ID(ResourceVoice), refs.ID(ResourceItem))
	}
}

func TestSessionProfileRefFor(t *testing.T) {
	profile := SessionProfile{
		LLM: ResourceRef{ProviderID: "p_llm", ModelID: "m_llm"},
		TTS: ResourceRef{ProviderID: "p_tts", ModelID: "m_tts", VoiceID: "v_tts"},
		ASR: ResourceRef{ProviderID: "p_asr", ModelID: "m_asr"},
	}
	if got := profile.RefFor(MeterSourceTTS); got.VoiceID != "v_tts" {
		t.Fatalf("tts ref = %+v", got)
	}
	if got := profile.RefFor(MeterSourceLLM); got.ModelID != "m_llm" {
		t.Fatalf("llm ref = %+v", got)
	}
	if got := profile.RefFor("unknown"); !got.IsZero() {
		t.Fatalf("unknown meter source should be zero, got %+v", got)
	}
}
