package billing

import "testing"

func TestRoundMicro(t *testing.T) {
	tests := []struct {
		name        string
		numerator   int64
		denominator int64
		rounding    Rounding
		want        int64
	}{
		{"exact", 10, 2, RoundingNone, 5},
		{"truncate", 7, 2, RoundingNone, 3},
		{"ceil rounds up", 7, 2, RoundingCeil, 4},
		{"ceil exact keeps value", 8, 2, RoundingCeil, 4},
		{"half up rounds up", 5, 2, RoundingHalfUp, 3},
		{"half up rounds down", 4, 3, RoundingHalfUp, 1},
		{"zero", 0, 7, RoundingCeil, 0},
		{"bad denominator treated as 1", 3, 0, RoundingNone, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RoundMicro(tc.numerator, tc.denominator, tc.rounding); got != tc.want {
				t.Fatalf("RoundMicro(%d, %d, %s) = %d, want %d",
					tc.numerator, tc.denominator, tc.rounding, got, tc.want)
			}
		})
	}
}

func TestMulRoundMicroNoOverflow(t *testing.T) {
	// 1e12 token × 1e6 微元 = 1e18，仍在 int64 内；再大也由 big.Int 兜住。
	got := MulRoundMicro(1_000_000_000_000, 1_000_000, 1_000_000, RoundingNone)
	if got != 1_000_000_000_000 {
		t.Fatalf("got %d, want 1000000000000", got)
	}
	// 乘法溢出边界：2^62 × 8 会溢出 int64，big.Int 下必须算对。
	if got := MulRoundMicro(1<<62, 8, 1, RoundingNone); got < 0 {
		t.Fatalf("overflowed to %d", got)
	}
}

func TestFormatMicro(t *testing.T) {
	tests := []struct {
		micro int64
		want  string
	}{
		{0, "0"},
		{1_000_000, "1"},
		{1_500_000, "1.5"},
		{1_234_567, "1.234567"},
		{-2_000_000, "-2"},
	}
	for _, tc := range tests {
		if got := FormatMicro(tc.micro); got != tc.want {
			t.Fatalf("FormatMicro(%d) = %q, want %q", tc.micro, got, tc.want)
		}
	}
}
