package billing

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

// ResourceType 是价格的资源维度：一次匹配只停在某一级粒度。
type ResourceType string

const (
	ResourceItem     ResourceType = "item"     // 兜底
	ResourceProvider ResourceType = "provider" // 某厂商
	ResourceModel    ResourceType = "model"    // 某模型
	ResourceVoice    ResourceType = "voice"    // 某音色
)

// Rank 返回资源粒度的具体程度，越大越具体（越优先）。
func (t ResourceType) Rank() int {
	switch t {
	case ResourceProvider:
		return 1
	case ResourceModel:
		return 2
	case ResourceVoice:
		return 3
	default:
		return 0
	}
}

// ParseResourceType 解析资源维度字符串，空值按 item 处理。
func ParseResourceType(value string) (ResourceType, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ResourceItem, nil
	}
	switch ResourceType(value) {
	case ResourceItem, ResourceProvider, ResourceModel, ResourceVoice:
		return ResourceType(value), nil
	}
	return "", fmt.Errorf("billing: unknown resource type %q", value)
}

// ResourceRef 是一次会话的资源快照（provider / model / voice 链）。voice 已
// 隐含 model，model 已隐含 provider，所以价格匹配只在其中一级命中。
//
// System 描述这条链背后的厂商账号是谁的：true = 平台代付（计费），false = 用户
// 自带 key（BYOK，只计量不计费）。这个判断只能有一份，所以由控制面在 authorize
// 时定好，数据面不做这个判断（§6.1 / §7）。
type ResourceRef struct {
	ProviderID string
	ModelID    string
	VoiceID    string
	System     bool
}

// ID 返回该粒度对应的实体 ID；item 级恒为空。
func (r ResourceRef) ID(t ResourceType) string {
	switch t {
	case ResourceProvider:
		return r.ProviderID
	case ResourceModel:
		return r.ModelID
	case ResourceVoice:
		return r.VoiceID
	default:
		return ""
	}
}

// Pairs 返回所有非空的 (粒度, ID) 组合，供 SQL 匹配动态拼接条件。
func (r ResourceRef) Pairs() [][2]string {
	var out [][2]string
	if r.ProviderID != "" {
		out = append(out, [2]string{string(ResourceProvider), r.ProviderID})
	}
	if r.ModelID != "" {
		out = append(out, [2]string{string(ResourceModel), r.ModelID})
	}
	if r.VoiceID != "" {
		out = append(out, [2]string{string(ResourceVoice), r.VoiceID})
	}
	return out
}

// IsZero 判断快照是否为空。
func (r ResourceRef) IsZero() bool {
	return r.ProviderID == "" && r.ModelID == "" && r.VoiceID == ""
}

// Tier 是阶梯的一档。阶梯按账期累计量分档累进（像个税），不是达标后全量按新价。
type Tier struct {
	UpTo           int64 // 本档上限（账期内累计 quantity）；0 = 最后一档，无上限
	UnitPriceMicro int64 // 落在本档区间内的量按这个单价算
}

// Price 是一版价格。生效区间由 EffectiveFrom / EffectiveTo 表达，没有额外的
// enabled 开关：这版价格生不生效只能有一个来源。
type Price struct {
	ID             string
	ItemCode       string
	AccountID      string       // '' = 平台标准价
	ResourceType   ResourceType // 粒度停在哪一级
	ResourceID     string       // 该级实体 ID；item 级为空
	Currency       string
	UnitPriceMicro int64 // 微单位单价
	UnitSize       int64 // 每 N 个 unit 计价
	MinChargeMicro int64 // 起步价
	Rounding       Rounding
	Tiers          []Tier
	EffectiveFrom  time.Time
	EffectiveTo    *time.Time
}

// Validate 校验一版价格。阶梯的两个不变量在这里就要报错，别留到结算时算出一
// 笔怪账。
func (p Price) Validate() error {
	if p.ItemCode == "" {
		return fmt.Errorf("billing: price: item_code is required")
	}
	if !validResourceType(p.ResourceType) {
		return fmt.Errorf("billing: price %s: unknown resource type %q", p.ItemCode, p.ResourceType)
	}
	if p.ResourceType == ResourceItem && p.ResourceID != "" {
		return fmt.Errorf("billing: price %s: item-level price must not carry resource_id", p.ItemCode)
	}
	if p.ResourceType != ResourceItem && p.ResourceID == "" {
		return fmt.Errorf("billing: price %s: %s-level price requires resource_id", p.ItemCode, p.ResourceType)
	}
	if p.UnitSize <= 0 {
		return fmt.Errorf("billing: price %s: unit_size must be positive", p.ItemCode)
	}
	if p.UnitPriceMicro < 0 {
		return fmt.Errorf("billing: price %s: unit_price_micro must not be negative", p.ItemCode)
	}
	if p.MinChargeMicro < 0 {
		return fmt.Errorf("billing: price %s: min_charge_micro must not be negative", p.ItemCode)
	}
	if _, err := ParseRounding(string(p.Rounding)); err != nil {
		return fmt.Errorf("billing: price %s: %w", p.ItemCode, err)
	}
	if p.EffectiveTo != nil && !p.EffectiveTo.After(p.EffectiveFrom) {
		return fmt.Errorf("billing: price %s: effective_to must be after effective_from", p.ItemCode)
	}
	return validateTiers(p.ItemCode, p.Tiers)
}

func validateTiers(itemCode string, tiers []Tier) error {
	prev := int64(0)
	for i, t := range tiers {
		if t.UnitPriceMicro < 0 {
			return fmt.Errorf("billing: price %s: tier %d: unit_price_micro must not be negative", itemCode, i)
		}
		if t.UpTo == 0 {
			if i != len(tiers)-1 {
				return fmt.Errorf("billing: price %s: tier %d: only the last tier may be open-ended", itemCode, i)
			}
			continue
		}
		if t.UpTo <= prev {
			return fmt.Errorf("billing: price %s: tier %d: up_to must be strictly increasing", itemCode, i)
		}
		prev = t.UpTo
	}
	return nil
}

func validResourceType(t ResourceType) bool {
	switch t {
	case ResourceItem, ResourceProvider, ResourceModel, ResourceVoice:
		return true
	}
	return false
}

// RoundingOrDefault 返回价格的舍入口径，空值按 none 处理。
func (p Price) RoundingOrDefault() Rounding {
	r, err := ParseRounding(string(p.Rounding))
	if err != nil {
		return RoundingNone
	}
	return r
}

// EffectiveAt 判断价格在某一时刻是否生效。
func (p Price) EffectiveAt(at time.Time) bool {
	if !p.EffectiveFrom.IsZero() && p.EffectiveFrom.After(at) {
		return false
	}
	return p.EffectiveTo == nil || p.EffectiveTo.After(at)
}

// MatchesScope 判断价格是否是这次匹配的候选：主体维度相同或为平台标准价，
// 资源维度命中同一级粒度。
func (p Price) MatchesScope(accountID string, refs ResourceRef) bool {
	if p.AccountID != "" && p.AccountID != accountID {
		return false
	}
	if p.ResourceType == ResourceItem {
		return true
	}
	return p.ResourceID != "" && p.ResourceID == refs.ID(p.ResourceType)
}

// PriceOf 返回“账期内累计 quantity”对应的全量金额：阶梯累进 + 一次舍入 + 起步价。
// 它是一个纯函数，增量结算靠 charge = max(PriceOf(new) − PriceOf(old), 0) 推出
// 每次应收的增量（§16.1）。
func (p Price) PriceOf(quantity int64) int64 {
	if quantity <= 0 {
		return 0
	}
	if p.UnitPriceMicro == 0 && len(p.Tiers) == 0 {
		return 0
	}
	amount := p.amountOf(quantity)
	if amount < p.MinChargeMicro {
		amount = p.MinChargeMicro
	}
	return amount
}

func (p Price) amountOf(quantity int64) int64 {
	unitSize := p.UnitSize
	if unitSize <= 0 {
		unitSize = 1
	}
	r := p.RoundingOrDefault()
	if len(p.Tiers) == 0 {
		return MulRoundMicro(quantity, p.UnitPriceMicro, unitSize, r)
	}

	total := new(big.Int)
	lower := int64(0)
	remaining := quantity
	for _, t := range p.Tiers {
		upper := t.UpTo
		if upper == 0 {
			upper = quantity
		}
		if upper <= lower {
			break
		}
		take := remaining
		if take > upper-lower {
			take = upper - lower
		}
		if take > 0 {
			total.Add(total, new(big.Int).Mul(big.NewInt(take), big.NewInt(t.UnitPriceMicro)))
			remaining -= take
		}
		lower = upper
		if remaining <= 0 {
			break
		}
	}
	return roundBig(total, unitSize, r)
}

// TierUnitPrice 返回累计量 q 处的边际单价，仅用于展示与排查。
func (p Price) TierUnitPrice(quantity int64) int64 {
	for _, t := range p.Tiers {
		if t.UpTo == 0 || quantity < t.UpTo {
			return t.UnitPriceMicro
		}
	}
	return p.UnitPriceMicro
}

// Select 从候选价格里挑出唯一生效的一版：账户协议价优先 → 资源越具体越优先 →
// 生效时间最新。accountID 为空表示匿名账户（只会命中平台标准价）。
//
// 匹配 SQL 也按同样顺序排序，这里再做一次选择是为了让“谁赢”只有一份实现，
// 单元测试可以直接盯住它。
func Select(candidates []Price, accountID string, refs ResourceRef, at time.Time) (Price, bool) {
	var best *Price
	for i := range candidates {
		p := candidates[i]
		if !p.EffectiveAt(at) || !p.MatchesScope(accountID, refs) {
			continue
		}
		if best == nil || preferPrice(p, *best) {
			best = &p
		}
	}
	if best == nil {
		return Price{}, false
	}
	return *best, true
}

func preferPrice(a, b Price) bool {
	if (a.AccountID != "") != (b.AccountID != "") {
		return a.AccountID != ""
	}
	if a.ResourceType.Rank() != b.ResourceType.Rank() {
		return a.ResourceType.Rank() > b.ResourceType.Rank()
	}
	if !a.EffectiveFrom.Equal(b.EffectiveFrom) {
		return a.EffectiveFrom.After(b.EffectiveFrom)
	}
	return a.ID > b.ID // 稳定兜底：同一 scope 同一时刻理论上只有一版
}

// SortByPreference 按 Select 的优先级排序，供仓储层与调试使用。
func SortByPreference(prices []Price) {
	sort.SliceStable(prices, func(i, j int) bool { return preferPrice(prices[i], prices[j]) })
}
