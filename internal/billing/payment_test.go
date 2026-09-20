package billing

import "testing"

func TestParseMicro(t *testing.T) {
	good := map[string]int64{
		"0":        0,
		"1":        MicroPerUnit,
		"1.5":      1_500_000,
		"1.05":     1_050_000,
		"0.01":     MicroPerCent,
		".5":       500_000,
		"00012.30": 12_300_000,
		"1.000001": MicroPerUnit + 1, // 微单位就是最细的一档
		" 9.99 ":   9_990_000,        // 网关偶尔带空格
	}
	for in, want := range good {
		got, err := ParseMicro(in)
		if err != nil {
			t.Errorf("ParseMicro(%q) unexpected error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseMicro(%q) = %d, want %d", in, got, want)
		}
	}

	// 这些全部要拒：金额解析放宽一位，入账金额就可能和订单金额对不上。
	for _, bad := range []string{"", "-1", "1.0000001", "1.2.3", "abc", "1e2", "1 0", "¥1", "+1"} {
		if _, err := ParseMicro(bad); err == nil {
			t.Errorf("ParseMicro(%q) should fail", bad)
		}
	}
}

func TestParseMicroRoundTrip(t *testing.T) {
	for _, micro := range []int64{0, 1, MicroPerCent, MicroPerUnit, 9_990_000, 12_345_678} {
		got, err := ParseMicro(FormatMicro(micro))
		if err != nil {
			t.Fatalf("ParseMicro(FormatMicro(%d)): %v", micro, err)
		}
		if got != micro {
			t.Fatalf("round trip %d → %q → %d", micro, FormatMicro(micro), got)
		}
	}
}

func TestParsePayChannel(t *testing.T) {
	for _, ch := range PayChannels() {
		got, err := ParsePayChannel(string(ch))
		if err != nil || got != ch {
			t.Errorf("ParsePayChannel(%q) = %q, %v", ch, got, err)
		}
	}
	for _, bad := range []string{"", "alipay", "epay", "epay:paypal", "EPAY:ALIPAY"} {
		if _, err := ParsePayChannel(bad); err == nil {
			t.Errorf("ParsePayChannel(%q) should fail", bad)
		}
	}
}

// TestRechargeIdempotencyKey 钉住这个键的字面形状。
//
// 它是充值幂等的地基，但拼它的地方有两处：这里，以及 Service.Credit 内部
// （"credit:" + RefType + ":" + RefID）。两边只要差一个字符，重复的回调就会变成
// 两次入账——而那种 bug 在测试里看不出来，只有账单对不上时才会发现。
func TestRechargeIdempotencyKey(t *testing.T) {
	if got, want := RechargeIdempotencyKey("p0123456789abcdef0123"), "credit:order:p0123456789abcdef0123"; got != want {
		t.Fatalf("RechargeIdempotencyKey = %q, want %q", got, want)
	}
}

// TestRefundIdempotencyKey 同理：它必须和 Service.Refund 内部拼的键一致，
// 而且与充值键**不相等**——否则「退款」会撞上「入账」那条唯一索引，钱退不出去。
func TestRefundIdempotencyKey(t *testing.T) {
	const order = "p0123456789abcdef0123"
	if got, want := RefundIdempotencyKey(order), "refund:order:p0123456789abcdef0123"; got != want {
		t.Fatalf("RefundIdempotencyKey = %q, want %q", got, want)
	}
	if RefundIdempotencyKey(order) == RechargeIdempotencyKey(order) {
		t.Fatal("refund and recharge keys must differ")
	}
}
