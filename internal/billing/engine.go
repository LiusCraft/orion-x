package billing

// SkipReason 说明一条事件为什么没产生金额。
type SkipReason string

const (
	SkipReasonBYOK         SkipReason = "byok"          // 用户自带 key：只计量不计费
	SkipReasonZeroQuantity SkipReason = "zero:quantity" // quantity 为 0
	SkipReasonItemDisabled SkipReason = "item:disabled" // 计费项未启用
	SkipReasonNoPrice      SkipReason = "no:price"      // 没有命中价格
)

// Event 是结算引擎看到的最小事件视图（由仓储行转换而来）。
type Event struct {
	ID       string
	ItemCode string
	Quantity int64
	// Billable 由控制面判定（BYOK / 系统代付 / 账户协议），数据面不做这个判断。
	Billable bool
}

// PeriodState 是账期内这个 scope（account + item_code + price_id）的累计量。
// 阶梯价与起步价按它做边际计价。
type PeriodState struct {
	Quantity       int64 // 账期内累计 quantity
	AmountMicro    int64 // 账期内累计已计金额（含起步价的影响）
	GrantUsedMicro int64
}

// ChargeResult 是一次结算的产出，字段跟 §16.2 那个事务的步骤一一对应。
type ChargeResult struct {
	AmountMicro         int64  // 应收总额（阶梯 + 舍入 + 起步价之后）
	GrantCoveredMicro   int64  // 由赠送额度覆盖的部分
	BalanceChargedMicro int64  // 真正从余额 / 信用扣的部分
	PriceID             string // 命中的价格版本
	Skipped             bool
	SkipReason          SkipReason
}

// Compute 是唯一的定价入口：算出这次结算应收的增量金额。
//
// 阶梯价和起步价必须按账期累计算，逐条独立计算会把两者都算错（§16.1）。所以
// 收的是增量：charge = max(PriceOf(new) − PriceOf(old), 0)。
func Compute(ev Event, p Price, st PeriodState) ChargeResult {
	if ev.Quantity <= 0 {
		return ChargeResult{Skipped: true, SkipReason: SkipReasonZeroQuantity}
	}
	if !ev.Billable {
		return ChargeResult{PriceID: p.ID, Skipped: true, SkipReason: SkipReasonBYOK}
	}

	previous := p.PriceOf(st.Quantity)
	next := p.PriceOf(st.Quantity + ev.Quantity)
	amount := next - previous
	if amount < 0 {
		amount = 0
	}
	return ChargeResult{
		AmountMicro:         amount,
		BalanceChargedMicro: amount,
		PriceID:             p.ID,
	}
}

// ApplyGrant 按可用的赠送额度拆分金额：先扣 grant，不够的部分走余额。
func (r ChargeResult) ApplyGrant(availableMicro int64) ChargeResult {
	if r.AmountMicro <= 0 || availableMicro <= 0 {
		return r
	}
	if availableMicro >= r.AmountMicro {
		r.GrantCoveredMicro = r.AmountMicro
		r.BalanceChargedMicro = 0
		return r
	}
	r.GrantCoveredMicro = availableMicro
	r.BalanceChargedMicro = r.AmountMicro - availableMicro
	return r
}

// Settled 判断这次结算是否真的动了钱。
func (r ChargeResult) Settled() bool {
	return !r.Skipped && r.AmountMicro > 0
}
