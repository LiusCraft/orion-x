package billing

import "testing"

func TestEstimate(t *testing.T) {
	snap := map[string]PriceSnapshot{
		ItemLLMInput:      {ItemCode: ItemLLMInput, Billable: true, UnitPriceMicro: 2, UnitSize: 1},
		ItemTTSCharacters: {ItemCode: ItemTTSCharacters, Billable: false, Reason: "byok"},
		ItemLLMOutput:     {ItemCode: ItemLLMOutput, Billable: true, UnitPriceMicro: 8_000_000, UnitSize: 10_000_000},
	}

	tests := []struct {
		name       string
		quantities map[string]int64
		want       int64
	}{
		{"empty", nil, 0},
		{"single item", map[string]int64{ItemLLMInput: 1000}, 2000},
		{"byok is free", map[string]int64{ItemTTSCharacters: 100}, 0},
		{"unknown item is free", map[string]int64{"unknown:item": 100}, 0},
		{"zero quantity", map[string]int64{ItemLLMInput: 0}, 0},
		{"ceil on unit_size", map[string]int64{ItemLLMOutput: 1}, 1}, // 0.8 微元 → 向上取整 1
		{"sum", map[string]int64{ItemLLMInput: 1000, ItemLLMOutput: 5_000_000}, 2000 + 4_000_000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Estimate(tc.quantities, snap); got != tc.want {
				t.Fatalf("Estimate() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEstimateOnlyUsesKnownBillableItems(t *testing.T) {
	snap := map[string]PriceSnapshot{
		ItemASRAudioSecond: {ItemCode: ItemASRAudioSecond, Billable: true, UnitPriceMicro: 24, UnitSize: 1},
	}
	got := EstimateSingle(ItemASRAudioSecond, 30, snap)
	if got != 720 {
		t.Fatalf("got %d, want 720", got)
	}
	if got := EstimateSingle(ItemTTSCharacters, 30, snap); got != 0 {
		t.Fatalf("unknown item estimated %d, want 0", got)
	}
}
