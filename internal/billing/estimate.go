package billing

// PriceSnapshot 是 authorize 下发给数据面的单价快照。数据面没有能力、也没有
// 机会挑价格版本，定价权始终在控制面。
type PriceSnapshot struct {
	ItemCode       string `json:"item_code"`
	Billable       bool   `json:"billable"`
	UnitPriceMicro int64  `json:"unit_price_micro,omitempty"`
	UnitSize       int64  `json:"unit_size,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

// Estimate 用 authorize 下发的快照估算一批用量的金额，只用于数据面的本地熔断，
// 不产生账单——账单只由控制面的 Compute 生成。
//
// 估算口径一律向上取整：宁可提前熔断，也不要透支。
func Estimate(quantities map[string]int64, snap map[string]PriceSnapshot) int64 {
	var total int64
	for code, qty := range quantities {
		if qty <= 0 {
			continue
		}
		s, ok := snap[code]
		if !ok || !s.Billable {
			continue
		}
		unitSize := s.UnitSize
		if unitSize <= 0 {
			unitSize = 1
		}
		total += MulRoundMicro(qty, s.UnitPriceMicro, unitSize, RoundingCeil)
	}
	return total
}

// EstimateSingle 估算单个计费项的金额，方便逐项累加。
func EstimateSingle(itemCode string, quantity int64, snap map[string]PriceSnapshot) int64 {
	if quantity <= 0 {
		return 0
	}
	return Estimate(map[string]int64{itemCode: quantity}, snap)
}
