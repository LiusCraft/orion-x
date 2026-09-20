package billing

import (
	"testing"
	"time"
)

func samplePrice() Price {
	return Price{
		ID:             "price_1",
		ItemCode:       ItemLLMInput,
		Currency:       "CNY",
		ResourceType:   ResourceModel,
		ResourceID:     "m_1",
		UnitPriceMicro: 2,
		UnitSize:       1,
		Rounding:       RoundingNone,
		EffectiveFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// 增量结算能替代批结算：分三次小额结算的总金额 == 一次性大额结算的金额。
func TestCompute_SplitEqualsOneShot(t *testing.T) {
	p := samplePrice()

	st := PeriodState{}
	var splitTotal int64
	for _, qty := range []int64{1000, 7, 99999} {
		res := Compute(Event{ID: "ev", ItemCode: p.ItemCode, Quantity: qty, Billable: true}, p, st)
		splitTotal += res.AmountMicro
		st = PeriodState{Quantity: st.Quantity + qty, AmountMicro: st.AmountMicro + res.AmountMicro}
	}

	oneShot := Compute(Event{ItemCode: p.ItemCode, Quantity: 1000 + 7 + 99999, Billable: true}, p, PeriodState{})
	if splitTotal != oneShot.AmountMicro {
		t.Fatalf("split settlement = %d, one-shot = %d", splitTotal, oneShot.AmountMicro)
	}
}

// 阶梯价：跨档时分次结算必须与一次性结算一致，且档内的边际单价生效。
func TestCompute_TieredProgressive(t *testing.T) {
	p := samplePrice()
	p.UnitSize = 1000 // 每 1000 token 计价
	p.Tiers = []Tier{
		{UpTo: 1000, UnitPriceMicro: 2_000},   // 每 1k token 2 微元
		{UpTo: 10_000, UnitPriceMicro: 1_000}, // 第二档半价
		{UpTo: 0, UnitPriceMicro: 500},
	}

	// 累计 5000：1000×2000 + 4000×1000 = 6_000_000，除以 unit_size=1000 → 6000
	if got := p.PriceOf(5000); got != 6000 {
		t.Fatalf("PriceOf(5000) = %d, want 6000", got)
	}
	// 跨档边界：PriceOf 单调不减
	prev := int64(0)
	for q := int64(0); q <= 20_000; q += 137 {
		got := p.PriceOf(q)
		if got < prev {
			t.Fatalf("PriceOf not monotonic at qty=%d: %d < %d", q, got, prev)
		}
		prev = got
	}

	// 分次结算 == 一次性结算
	st := PeriodState{}
	var splitTotal int64
	for i := 0; i < 20; i++ {
		res := Compute(Event{ItemCode: p.ItemCode, Quantity: 600, Billable: true}, p, st)
		if res.AmountMicro < 0 {
			t.Fatalf("negative incremental charge: %d", res.AmountMicro)
		}
		splitTotal += res.AmountMicro
		st = PeriodState{Quantity: st.Quantity + 600, AmountMicro: st.AmountMicro + res.AmountMicro}
	}
	if want := p.PriceOf(12_000); splitTotal != want {
		t.Fatalf("split total = %d, want %d", splitTotal, want)
	}
}

// 起步价：一个账期只收一次，且 PriceOf(0) = 0。
func TestCompute_MinChargeChargedOnce(t *testing.T) {
	p := samplePrice()
	p.UnitPriceMicro = 1
	p.MinChargeMicro = 1_000_000

	if got := p.PriceOf(0); got != 0 {
		t.Fatalf("PriceOf(0) = %d, want 0", got)
	}
	if got := p.PriceOf(10); got != 1_000_000 {
		t.Fatalf("PriceOf(10) = %d, want min charge 1000000", got)
	}

	st := PeriodState{}
	first := Compute(Event{Quantity: 10, Billable: true}, p, st)
	if first.AmountMicro != 1_000_000 {
		t.Fatalf("first charge = %d, want 1000000", first.AmountMicro)
	}

	// 起步价只收一次：用量还在起步价覆盖范围内时不再收费。
	st = PeriodState{Quantity: 10, AmountMicro: first.AmountMicro}
	second := Compute(Event{Quantity: 10, Billable: true}, p, st)
	if second.AmountMicro != 0 {
		t.Fatalf("second charge = %d, want 0 (已经付过起步价)", second.AmountMicro)
	}

	// 用量超过起步价覆盖范围后，只收超出的增量。
	st = PeriodState{Quantity: 20, AmountMicro: first.AmountMicro}
	third := Compute(Event{Quantity: 2_000_000, Billable: true}, p, st)
	if want := p.PriceOf(2_000_020) - first.AmountMicro; third.AmountMicro != want {
		t.Fatalf("third charge = %d, want %d", third.AmountMicro, want)
	}
}

func TestCompute_Skip(t *testing.T) {
	p := samplePrice()
	tests := []struct {
		name   string
		ev     Event
		reason SkipReason
	}{
		{"zero quantity", Event{Quantity: 0, Billable: true}, SkipReasonZeroQuantity},
		{"negative quantity", Event{Quantity: -5, Billable: true}, SkipReasonZeroQuantity},
		{"byok", Event{Quantity: 100, Billable: false}, SkipReasonBYOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Compute(tc.ev, p, PeriodState{})
			if !res.Skipped || res.SkipReason != tc.reason {
				t.Fatalf("got %+v, want skipped with %q", res, tc.reason)
			}
			if res.AmountMicro != 0 || res.BalanceChargedMicro != 0 {
				t.Fatalf("skipped event still produced money: %+v", res)
			}
		})
	}
}

func TestApplyGrant(t *testing.T) {
	tests := []struct {
		name        string
		amount      int64
		available   int64
		wantGrant   int64
		wantBalance int64
	}{
		{"no grant", 500, 0, 0, 500},
		{"partial", 500, 200, 200, 300},
		{"full", 500, 500, 500, 0},
		{"over", 500, 900, 500, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := ChargeResult{AmountMicro: tc.amount, BalanceChargedMicro: tc.amount}.ApplyGrant(tc.available)
			if res.GrantCoveredMicro != tc.wantGrant || res.BalanceChargedMicro != tc.wantBalance {
				t.Fatalf("got grant=%d balance=%d, want grant=%d balance=%d",
					res.GrantCoveredMicro, res.BalanceChargedMicro, tc.wantGrant, tc.wantBalance)
			}
		})
	}
}

// 单价精度不够时抬 unit_size：0.8 微元/token 用 “每 1000 万 token 8 元” 表达。
func TestPrice_UnitSizeCarriesPrecision(t *testing.T) {
	p := samplePrice()
	p.UnitPriceMicro = 8_000_000
	p.UnitSize = 10_000_000

	if got := p.PriceOf(1); got != 0 {
		t.Fatalf("PriceOf(1) = %d, want 0 (不足 1 微元)", got)
	}
	if got := p.PriceOf(10_000_000); got != 8_000_000 {
		t.Fatalf("PriceOf(1e7) = %d, want 8000000", got)
	}
	// 一次结算内只舍入一次：1e7 条单价 0.8 微元 → 8 元，逐条算会全部变成 0
	st := PeriodState{}
	total := int64(0)
	for i := 0; i < 100_000; i++ {
		res := Compute(Event{Quantity: 100, Billable: true}, p, st)
		total += res.AmountMicro
		st = PeriodState{Quantity: st.Quantity + 100, AmountMicro: st.AmountMicro + res.AmountMicro}
	}
	if total != 8_000_000 {
		t.Fatalf("piecewise total = %d, want 8000000", total)
	}
}
