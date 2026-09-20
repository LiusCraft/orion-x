package billing

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// MicroPerUnit 是一个货币单位（元）对应的微单位数。全链路金额都用 int64
// 微单位表示，不用浮点。
const MicroPerUnit = 1_000_000

// MicroPerCent 是一分钱（0.01 元）的微单位数。第三方支付网关的 money 字段通常
// 只有两位小数，所以走支付渠道的金额必须是它的整数倍（见 gateway/epay）。
const MicroPerCent = MicroPerUnit / 100

// Rounding 是一次结算内的舍入口径。
type Rounding string

const (
	RoundingNone   Rounding = "none"    // 截断（丢弃不足 1 微元的部分），usage 类默认
	RoundingCeil   Rounding = "ceil"    // 向上取整（远离 0），duration 类默认
	RoundingHalfUp Rounding = "half:up" // 四舍五入
)

// ParseRounding 解析舍入口径，空值按 none 处理。
func ParseRounding(value string) (Rounding, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return RoundingNone, nil
	}
	switch Rounding(value) {
	case RoundingNone, RoundingCeil, RoundingHalfUp:
		return Rounding(value), nil
	}
	return "", fmt.Errorf("billing: unknown rounding %q", value)
}

// DefaultRoundingFor 返回计费模式对应的默认舍入口径：
// duration 向上取整，其余精确。
func DefaultRoundingFor(mode ChargeMode) Rounding {
	if mode == ChargeModeDuration {
		return RoundingCeil
	}
	return RoundingNone
}

// RoundMicro 计算 numerator/denominator 并按 r 舍入。中间量用 big.Int，
// 逐 token 相乘不会溢出。denominator ≤ 0 时按 1 处理。
//
// 一次结算只应该调用一次这个函数（§3.3）：逐条舍入会累积误差，跨档金额也
// 会对不上。
func RoundMicro(numerator, denominator int64, r Rounding) int64 {
	if numerator == 0 {
		return 0
	}
	return roundBig(big.NewInt(numerator), denominator, r)
}

// MulRoundMicro 计算 qty × unitPriceMicro ÷ unitSize 并按 r 舍入，乘法在
// big.Int 内完成，不会溢出。
func MulRoundMicro(qty, unitPriceMicro, unitSize int64, r Rounding) int64 {
	if qty == 0 || unitPriceMicro == 0 {
		return 0
	}
	return roundBig(new(big.Int).Mul(big.NewInt(qty), big.NewInt(unitPriceMicro)), unitSize, r)
}

// roundBig 用 r 把 numerator/denominator 舍入到整数微元。
func roundBig(numerator *big.Int, denominator int64, r Rounding) int64 {
	if numerator == nil || numerator.Sign() == 0 {
		return 0
	}
	if denominator <= 0 {
		denominator = 1
	}
	den := big.NewInt(denominator)
	quo, rem := new(big.Int).QuoRem(numerator, den, new(big.Int))
	if rem.Sign() == 0 {
		return quo.Int64()
	}
	negative := rem.Sign() < 0
	if negative {
		rem.Neg(rem)
		quo.Neg(quo)
	}

	switch r {
	case RoundingCeil:
		if !negative {
			quo.Add(quo, big.NewInt(1)) // 向上取整 = 远离 0
		}
	case RoundingHalfUp:
		twice := new(big.Int).Lsh(rem, 1)
		if twice.Cmp(den) >= 0 {
			if negative {
				quo.Sub(quo, big.NewInt(1))
			} else {
				quo.Add(quo, big.NewInt(1))
			}
		}
	default: // RoundingNone 与未知取值都退化为截断（QuoRem 已按 0 截断）
	}
	return quo.Int64()
}

// ParseMicro 是 FormatMicro 的逆：把货币字符串（"1"、"1.5"、"0.000001"）解析成
// 微单位，最多 6 位小数。支付网关回调里的 money 就是这种字符串。
//
// 先解析再比较，不要用 float64 中转：金额比对的任何精度损失都是直接的钱。
func ParseMicro(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("billing: empty amount")
	}
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("billing: negative amount %q", s)
	}

	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}
	if len(frac) > 6 {
		return 0, fmt.Errorf("billing: amount %q is finer than a micro unit", s)
	}
	for len(frac) < 6 {
		frac += "0"
	}
	for _, r := range whole + frac {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("billing: malformed amount %q", s)
		}
	}

	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("billing: malformed amount %q: %w", s, err)
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("billing: malformed amount %q: %w", s, err)
	}
	return w*MicroPerUnit + f, nil
}

// FormatMicro 把微单位金额格式化成货币字符串，供展示用（不做汇率换算）。
func FormatMicro(micro int64) string {
	sign := ""
	if micro < 0 {
		sign = "-"
		micro = -micro
	}
	whole := micro / MicroPerUnit
	frac := micro % MicroPerUnit
	if frac == 0 {
		return sign + strconv.FormatInt(whole, 10)
	}
	text := strings.TrimRight(fmt.Sprintf("%06d", frac), "0")
	return sign + strconv.FormatInt(whole, 10) + "." + text
}
