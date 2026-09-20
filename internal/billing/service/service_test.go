package service

import (
	"strings"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/store"
)

func testService(t *testing.T) *Service {
	t.Helper()
	cfg := DefaultConfig()
	cfg.PeriodLocation = time.UTC
	cfg.Now = func() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC) }
	return New(nil, nil, cfg)
}

func TestPeriodStart(t *testing.T) {
	svc := testService(t)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	svc.cfg.PeriodLocation = shanghai

	// 账期是自然月，按配置的时区切。
	at := time.Date(2026, 9, 30, 17, 0, 0, 0, time.UTC) // 上海时间 10-01 01:00
	got := svc.periodStart(at)
	want := time.Date(2026, 10, 1, 0, 0, 0, 0, shanghai)
	if !got.Equal(want) {
		t.Fatalf("periodStart = %s, want %s", got, want)
	}

	// 同一个 UTC 时刻在 UTC 账期下还属于 9 月——时区确实参与了切分。
	svc.cfg.PeriodLocation = time.UTC
	if got := svc.periodStart(at); !got.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("periodStart in UTC = %s, want 2026-09-01", got)
	}
}

func TestPriceTimeOf(t *testing.T) {
	svc := testService(t)
	base := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		ev   store.BillingUsageEvent
		want time.Time
	}{
		{"normal", store.BillingUsageEvent{OccurredAt: base, ReceivedAt: base.Add(time.Second)}, base},
		{"missing received", store.BillingUsageEvent{OccurredAt: base}, base},
		{"missing occurred", store.BillingUsageEvent{ReceivedAt: base}, base},
		{
			name: "clock skew uses received time",
			ev:   store.BillingUsageEvent{OccurredAt: base.Add(-time.Hour), ReceivedAt: base},
			want: base,
		},
		{
			name: "skew within tolerance keeps occurred time",
			ev: store.BillingUsageEvent{
				OccurredAt: base.Add(-billing.ClockSkewTolerance + time.Second),
				ReceivedAt: base,
			},
			want: base.Add(-billing.ClockSkewTolerance + time.Second),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := svc.priceTimeOf(tc.ev); !got.Equal(tc.want) {
				t.Fatalf("priceTimeOf = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestReserveAmount(t *testing.T) {
	svc := testService(t)
	items := []store.BillingItem{
		{Code: billing.ItemASRAudioSecond, ChargeMode: string(billing.ChargeModeDuration), Enabled: true},
		{Code: billing.ItemLLMInput, ChargeMode: string(billing.ChargeModeUsage), Enabled: true},
		{Code: billing.ItemTTSCharacters, ChargeMode: string(billing.ChargeModeUsage), Enabled: true},
	}
	snap := map[string]billing.PriceSnapshot{
		billing.ItemASRAudioSecond: {ItemCode: billing.ItemASRAudioSecond, Billable: true, UnitPriceMicro: 24, UnitSize: 1},
		billing.ItemLLMInput:       {ItemCode: billing.ItemLLMInput, Billable: true, UnitPriceMicro: 2, UnitSize: 1},
		billing.ItemTTSCharacters:  {ItemCode: billing.ItemTTSCharacters, Billable: true, UnitPriceMicro: 100, UnitSize: 1},
	}

	// 只按 duration 类项估算：600 秒 × 24 微元/秒。
	if got := svc.reserveAmount(snap, items); got != 600*24 {
		t.Fatalf("reserveAmount = %d, want %d", got, 600*24)
	}

	// 显式配了费率就用它。
	svc.cfg.ReserveRateMicro = 1000
	if got := svc.reserveAmount(snap, items); got != 600*1000 {
		t.Fatalf("reserveAmount with rate = %d, want %d", got, 600*1000)
	}

	// 降级/无时长项时预留额为 0。
	svc.cfg.ReserveRateMicro = 0
	if got := svc.reserveAmount(map[string]billing.PriceSnapshot{}, items); got != 0 {
		t.Fatalf("reserveAmount without snapshot = %d, want 0", got)
	}
}

func TestNewUsageEvent(t *testing.T) {
	svc := testService(t)
	now := svc.now()
	profile := billing.SessionProfile{
		VoicebotID: "vb_1",
		LLM:        billing.ResourceRef{ProviderID: "p_llm", ModelID: "m_llm", System: true},
		TTS:        billing.ResourceRef{ProviderID: "p_byok", ModelID: "m_tts", VoiceID: "v_1", System: false},
	}
	reservation := &store.BillingReservation{
		ID: "r_1", SessionID: "s_1", AccountID: "acct_1", DeviceID: "d_1", VoicebotID: "vb_1",
	}
	snap := map[string]billing.PriceSnapshot{
		billing.ItemLLMInput:      {ItemCode: billing.ItemLLMInput, Billable: true, UnitPriceMicro: 2, UnitSize: 1},
		billing.ItemTTSCharacters: {ItemCode: billing.ItemTTSCharacters, Billable: false, Reason: "byok"},
	}
	llmItem := store.BillingItem{Code: billing.ItemLLMInput, MeterSource: billing.MeterSourceLLM}
	ttsItem := store.BillingItem{Code: billing.ItemTTSCharacters, MeterSource: billing.MeterSourceTTS}

	t.Run("platform key is billable and uses the snapshot resources", func(t *testing.T) {
		// 事件里带的 model 与快照不一致：应该以快照为准并记一个维度。
		ev := svc.newUsageEvent(billing.UsageEventReport{
			EventID: "e1", ItemCode: billing.ItemLLMInput, Quantity: 100,
			ModelID: "m_stale", OccurredAt: now, TurnIndex: 3,
			Dimensions: map[string]any{"step": 2},
		}, llmItem, reservation, profile, snap, now)

		if ev.AccountID != "acct_1" || ev.AIModelID != "m_llm" || ev.ProviderID != "p_llm" {
			t.Fatalf("event resources = %+v, want them to come from the authorize snapshot", ev)
		}
		if ev.BYOK {
			t.Fatal("platform key must be billable")
		}
		if ev.Status != billing.EventStatusPending {
			t.Fatalf("status = %q, want pending", ev.Status)
		}
		if ev.Dimensions["resource_mismatch"] != "model" || ev.Dimensions["step"] != 2 {
			t.Fatalf("dimensions = %+v, want resource_mismatch=model plus the caller's step", ev.Dimensions)
		}
		if ev.TurnIndex != 3 || !ev.ReceivedAt.Equal(now) {
			t.Fatalf("event = %+v, want turn index and received_at filled", ev)
		}
	})

	t.Run("byok item is recorded but not billable", func(t *testing.T) {
		ev := svc.newUsageEvent(billing.UsageEventReport{
			EventID: "e2", ItemCode: billing.ItemTTSCharacters, Quantity: 42, OccurredAt: now,
		}, ttsItem, reservation, profile, snap, now)
		if !ev.BYOK {
			t.Fatal("BYOK item must be flagged so the engine skips it")
		}
	})

	t.Run("clock skew is flagged", func(t *testing.T) {
		ev := svc.newUsageEvent(billing.UsageEventReport{
			EventID: "e3", ItemCode: billing.ItemLLMInput, Quantity: 1,
			OccurredAt: now.Add(-time.Hour),
		}, llmItem, reservation, profile, snap, now)
		if _, ok := ev.Dimensions["clock_skew_ms"]; !ok {
			t.Fatalf("dimensions = %+v, want clock_skew_ms", ev.Dimensions)
		}
		if !ev.OccurredAt.Equal(now.Add(-time.Hour)) {
			t.Fatal("occurred_at must stay the fact the data plane reported")
		}
	})

	t.Run("zero occurred_at falls back to now", func(t *testing.T) {
		ev := svc.newUsageEvent(billing.UsageEventReport{
			EventID: "e4", ItemCode: billing.ItemLLMInput, Quantity: 1,
		}, llmItem, reservation, profile, snap, now)
		if !ev.OccurredAt.Equal(now) {
			t.Fatalf("occurred_at = %s, want %s", ev.OccurredAt, now)
		}
	})
}

func TestSnapshotRoundTrip(t *testing.T) {
	in := map[string]billing.PriceSnapshot{
		billing.ItemLLMInput: {ItemCode: billing.ItemLLMInput, Billable: true, UnitPriceMicro: 2, UnitSize: 1},
	}
	raw, err := toJSONMap(in)
	if err != nil {
		t.Fatalf("toJSONMap: %v", err)
	}
	out := snapshotFromJSON(raw)
	if out[billing.ItemLLMInput].UnitPriceMicro != 2 {
		t.Fatalf("round trip lost the price: %+v", out)
	}

	profile := billing.SessionProfile{
		VoicebotID: "vb",
		TTS:        billing.ResourceRef{ProviderID: "p", ModelID: "m", VoiceID: "v", System: false},
	}
	rawProfile, err := toJSONMap(profile)
	if err != nil {
		t.Fatalf("toJSONMap profile: %v", err)
	}
	got := profileFromJSON(rawProfile)
	if got.VoicebotID != "vb" || got.TTS.VoiceID != "v" || got.TTS.System {
		t.Fatalf("profile round trip = %+v", got)
	}

	if _, err := toJSONMap(make(chan int)); err == nil {
		t.Fatal("marshal failure must be reported")
	}
	if got := snapshotFromJSON(nil); len(got) != 0 {
		t.Fatalf("nil snapshot must decode to an empty map, got %+v", got)
	}
}

func TestSnapshotListIsStable(t *testing.T) {
	snap := map[string]billing.PriceSnapshot{
		"b:item": {ItemCode: "b:item"},
		"a:item": {ItemCode: "a:item"},
	}
	first := snapshotList(snap)
	second := snapshotList(snap)
	if len(first) != 2 || first[0].ItemCode != "a:item" {
		t.Fatalf("snapshotList = %+v, want sorted by item code", first)
	}
	if first[0].ItemCode != second[0].ItemCode || first[1].ItemCode != second[1].ItemCode {
		t.Fatal("snapshotList must be deterministic")
	}
}

func TestNewAccountIDFitsColumn(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := newAccountID()
		if !strings.HasPrefix(id, "acct_") {
			t.Fatalf("account id %q must carry the acct_ prefix", id)
		}
		// accounts.id 是 varchar(36)：前缀 + uuid 会超长，所以用短 ID。
		if len(id) > 36 {
			t.Fatalf("account id %q is %d chars, column is varchar(36)", id, len(id))
		}
		if seen[id] {
			t.Fatalf("duplicate account id %q", id)
		}
		seen[id] = true
	}
}

func TestDomainPriceConversion(t *testing.T) {
	to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	row := store.BillingPrice{
		ID: "p1", ItemCode: billing.ItemLLMInput, ResourceType: "voice", ResourceID: "v1",
		UnitPriceMicro: 10, UnitSize: 1000, MinChargeMicro: 5, Rounding: "ceil",
		Tiers:         store.BillingTiers{{UpTo: 100, UnitPriceMicro: 3}, {UpTo: 0, UnitPriceMicro: 2}},
		EffectiveFrom: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EffectiveTo: &to,
	}
	p := domainPrice(row)
	if p.ResourceType != billing.ResourceVoice || p.ResourceID != "v1" {
		t.Fatalf("resource scope lost: %+v", p)
	}
	if p.Rounding != billing.RoundingCeil || len(p.Tiers) != 2 || p.Tiers[0].UpTo != 100 {
		t.Fatalf("price fields lost: %+v", p)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("converted price must be valid: %v", err)
	}
}

func TestItemMeterSource(t *testing.T) {
	if got := itemMeterSource(billing.ItemASRAudioSecond); got != billing.MeterSourceASR {
		t.Fatalf("itemMeterSource = %q, want asr", got)
	}
	if got := itemMeterSource("unknown:item"); got != billing.MeterSourceLLM {
		t.Fatalf("unknown item must fall back to llm, got %q", got)
	}
}

func TestResourceMismatch(t *testing.T) {
	ref := billing.ResourceRef{ProviderID: "p", ModelID: "m", VoiceID: "v"}
	tests := []struct {
		report billing.UsageEventReport
		want   string
	}{
		{billing.UsageEventReport{}, ""},
		{billing.UsageEventReport{ModelID: "m"}, ""},
		{billing.UsageEventReport{ModelID: "other"}, "model"},
		{billing.UsageEventReport{ProviderID: "other"}, "provider"},
		{billing.UsageEventReport{VoiceID: "other"}, "voice"},
	}
	for _, tc := range tests {
		if got := resourceMismatch(tc.report, ref); got != tc.want {
			t.Fatalf("resourceMismatch(%+v) = %q, want %q", tc.report, got, tc.want)
		}
	}
}

func TestDefaultGrantItemsAreUsageItemsOnly(t *testing.T) {
	items := defaultGrantItems()
	if len(items) == 0 {
		t.Fatal("expected default grant items")
	}
	for _, code := range items {
		item, ok := billing.DefaultItem(code)
		if !ok {
			t.Fatalf("grant item %q is not a built-in item", code)
		}
		if item.ChargeMode != billing.ChargeModeUsage && item.ChargeMode != billing.ChargeModeDuration {
			t.Fatalf("grant item %q has charge mode %q, only usage/duration items should be granted", code, item.ChargeMode)
		}
	}
}

func TestNormalizeConfigDefaults(t *testing.T) {
	cfg := normalizeConfig(Config{})
	def := DefaultConfig()
	if cfg.WorkerTick != def.WorkerTick || cfg.ReserveSeconds != def.ReserveSeconds ||
		cfg.OverdraftPolicy != def.OverdraftPolicy || cfg.Currency != def.Currency {
		t.Fatalf("normalizeConfig = %+v, want defaults", cfg)
	}
	if cfg.PeriodLocation == nil || cfg.Now == nil {
		t.Fatal("normalizeConfig must fill PeriodLocation and Now")
	}
	// 显式值不被覆盖。
	custom := normalizeConfig(Config{ReserveSeconds: 30, OverdraftPolicy: billing.OverdraftAllow, ReserveTTL: time.Minute})
	if custom.ReserveSeconds != 30 || custom.OverdraftPolicy != billing.OverdraftAllow || custom.ReserveTTL != time.Minute {
		t.Fatalf("normalizeConfig overrode explicit values: %+v", custom)
	}
}

func TestValidateSubject(t *testing.T) {
	if err := validateSubject("user", "u1"); err != nil {
		t.Fatalf("validateSubject() = %v, want nil", err)
	}
	for _, tc := range [][2]string{{"", "u1"}, {"user", ""}, {"", ""}} {
		err := validateSubject(tc[0], tc[1])
		if err == nil {
			t.Fatalf("validateSubject(%q, %q) = nil, want error", tc[0], tc[1])
		}
		if !strings.Contains(err.Error(), "subject_type and subject_id are required") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}
