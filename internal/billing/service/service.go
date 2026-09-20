// Package service 是计费的控制面实现：仓储读写、结算事务、准入、worker 与回收。
// 它是唯一 import internal/store 的计费包（§19）。
//
// 计费不反向依赖业务：authorize 需要知道的“这个 device 属于谁、用的是哪个模型”
// 通过 billing.SubjectResolver 从外面递进来（resolver.go 是它在 manager 进程内的
// 实现）。
package service

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/store"
)

// 计费服务对外暴露的错误。HTTP 层按它们映射状态码。
var (
	ErrInsufficientBalance = errors.New("billing: insufficient balance")
	ErrAccountNotFound     = errors.New("billing: account not found")
	ErrReservationNotFound = errors.New("billing: reservation not found")
	ErrBatchTooLarge       = errors.New("billing: usage batch too large")
	ErrInvalidRequest      = errors.New("billing: invalid request")
	ErrItemNotFound        = errors.New("billing: item not found")
)

// Config 是结算引擎的运行参数。
type Config struct {
	// Currency 是单币种部署的币种，P1 不做汇率。
	Currency string
	// ReserveSeconds 是预冻结覆盖的会话时长，也是 authorize 下发给数据面的
	// max_session_seconds。它的语义是“沉默多久算死”，不是“会话最多活多久”。
	ReserveSeconds int
	// ReserveRateMicro 是每秒的预留额（微元/秒）。为 0 时按 duration 类计费项的
	// 单价估算（billing.Estimate）。
	ReserveRateMicro int64
	// ReserveTTL 是控制面预冻结（Meter.Reserve）的有效期。
	ReserveTTL time.Duration
	// SettlementMode 是 sync | async。P1 默认 async（worker 每 1~5 秒跑一轮）。
	SettlementMode string
	// OverdraftPolicy 是 deny | allow：余额扣成负数时是停服还是挂账。
	OverdraftPolicy string
	// WorkerTick / WorkerBatch / WorkerBatchTimeout 是结算 worker 的参数。
	WorkerTick         time.Duration
	WorkerBatch        int
	WorkerBatchTimeout time.Duration
	// ReapInterval 是预冻结回收的 tick。
	ReapInterval time.Duration
	// PeriodLocation 是账期（自然月）的时区。
	PeriodLocation *time.Location
	// SignupGrantMicro 是新账户的注册赠送额度（总额），按 SignupGrantItems
	// 均分。赠款走 billing_grants，不写进 balance_micro——否则赠款和充值款在账
	// 上就分不开了（§12）。
	SignupGrantMicro int64
	// SignupGrantItems 是要发赠款的计费项；为空时用内置的按量/按时长计费项。
	SignupGrantItems []string
	// GrantTTL 是赠款的有效期。
	GrantTTL time.Duration
	// Now 便于测试注入。
	Now func() time.Time
}

// DefaultConfig 返回可用的默认配置。
func DefaultConfig() Config {
	return Config{
		Currency:           "CNY",
		ReserveSeconds:     600,
		ReserveRateMicro:   0,
		ReserveTTL:         24 * time.Hour,
		SettlementMode:     billing.SettlementAsync,
		OverdraftPolicy:    billing.OverdraftDeny,
		WorkerTick:         time.Second,
		WorkerBatch:        200,
		WorkerBatchTimeout: 5 * time.Second,
		ReapInterval:       10 * time.Second,
		PeriodLocation:     time.UTC,
		SignupGrantMicro:   5 * billing.MicroPerUnit, // 送 ¥5
		GrantTTL:           30 * 24 * time.Hour,
		Now:                time.Now,
	}
}

// Service 是计费控制面的全部能力。
type Service struct {
	store    *store.BillingStore
	resolver billing.SubjectResolver
	cfg      Config
}

// New 组装计费服务。resolver 为 nil 时只能用账户主体直接驱动的能力
// （Charge / Reserve / Credit / Adjust），数据面三个接口会返回错误。
func New(st *store.BillingStore, resolver billing.SubjectResolver, cfg Config) *Service {
	cfg = normalizeConfig(cfg)
	return &Service{store: st, resolver: resolver, cfg: cfg}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if strings.TrimSpace(cfg.Currency) == "" {
		cfg.Currency = def.Currency
	}
	if cfg.ReserveSeconds <= 0 {
		cfg.ReserveSeconds = def.ReserveSeconds
	}
	if cfg.ReserveTTL <= 0 {
		cfg.ReserveTTL = def.ReserveTTL
	}
	if cfg.SettlementMode == "" {
		cfg.SettlementMode = def.SettlementMode
	}
	if cfg.OverdraftPolicy == "" {
		cfg.OverdraftPolicy = def.OverdraftPolicy
	}
	if cfg.WorkerTick <= 0 {
		cfg.WorkerTick = def.WorkerTick
	}
	if cfg.WorkerBatch <= 0 {
		cfg.WorkerBatch = def.WorkerBatch
	}
	if cfg.WorkerBatchTimeout <= 0 {
		cfg.WorkerBatchTimeout = def.WorkerBatchTimeout
	}
	if cfg.ReapInterval <= 0 {
		cfg.ReapInterval = def.ReapInterval
	}
	if cfg.PeriodLocation == nil {
		cfg.PeriodLocation = def.PeriodLocation
	}
	if cfg.GrantTTL <= 0 {
		cfg.GrantTTL = def.GrantTTL
	}
	if cfg.Now == nil {
		cfg.Now = def.Now
	}
	return cfg
}

// Config 返回生效的配置（规范化之后），供启动日志与实际接线使用。
func (s *Service) Config() Config { return s.cfg }

// Store 暴露底层仓储，供同一进程内的只读查询与业务模块使用。
func (s *Service) Store() *store.BillingStore { return s.store }

func (s *Service) now() time.Time { return s.cfg.Now() }

// periodStart 返回 t 所在账期的起点（自然月，按配置的时区）。跨期就新建一行
// period state，阶梯重新起算（§16.1）。
func (s *Service) periodStart(t time.Time) time.Time {
	loc := s.cfg.PeriodLocation
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
}

// priceTimeOf 决定一条事件按哪个时刻匹配价格。正常情况下用事件发生时刻；时钟
// 偏移超阈值的（数据面时钟不可信）按收到时刻结算（§4 / §14.4）。
func (s *Service) priceTimeOf(ev store.BillingUsageEvent) time.Time {
	if ev.ReceivedAt.IsZero() {
		return ev.OccurredAt
	}
	if ev.OccurredAt.IsZero() {
		return ev.ReceivedAt
	}
	skew := ev.OccurredAt.Sub(ev.ReceivedAt)
	if skew < 0 {
		skew = -skew
	}
	if skew > billing.ClockSkewTolerance {
		return ev.ReceivedAt
	}
	return ev.OccurredAt
}

// newAccountID 生成内部账户 ID：`acct_` 前缀 + 80 位随机十六进制。
// 主键用内部 ID 而不是 user id，将来支持 Organization 时不用回填历史数据（§12）。
func newAccountID() string {
	return "acct_" + shortID()
}

func shortID() string {
	raw := uuid.New()
	return hex.EncodeToString(raw[:10])
}

// defaultGrantItems 是注册赠款默认覆盖的计费项：内置的按量/按时长计费项。
func defaultGrantItems() []string {
	var out []string
	for _, item := range billing.DefaultItems() {
		if !item.Enabled {
			continue
		}
		switch item.ChargeMode {
		case billing.ChargeModeUsage, billing.ChargeModeDuration:
			out = append(out, item.Code)
		}
	}
	return out
}

// domainEvent 把仓储行转成结算引擎看到的最小事件视图。
func domainEvent(ev store.BillingUsageEvent) billing.Event {
	return billing.Event{
		ID:       ev.ID,
		ItemCode: ev.ItemCode,
		Quantity: ev.Quantity,
		Billable: !ev.BYOK,
	}
}

// domainPrice 把仓储行转成领域价格。
func domainPrice(p store.BillingPrice) billing.Price {
	resourceType := billing.ResourceType(p.ResourceType)
	if resourceType == "" {
		resourceType = billing.ResourceItem
	}
	rounding, err := billing.ParseRounding(p.Rounding)
	if err != nil {
		rounding = billing.RoundingNone
	}
	tiers := make([]billing.Tier, 0, len(p.Tiers))
	for _, t := range p.Tiers {
		tiers = append(tiers, billing.Tier{UpTo: t.UpTo, UnitPriceMicro: t.UnitPriceMicro})
	}
	return billing.Price{
		ID:             p.ID,
		ItemCode:       p.ItemCode,
		AccountID:      p.AccountID,
		ResourceType:   resourceType,
		ResourceID:     p.ResourceID,
		Currency:       p.Currency,
		UnitPriceMicro: p.UnitPriceMicro,
		UnitSize:       p.UnitSize,
		MinChargeMicro: p.MinChargeMicro,
		Rounding:       rounding,
		Tiers:          tiers,
		EffectiveFrom:  p.EffectiveFrom,
		EffectiveTo:    p.EffectiveTo,
	}
}

// findPrice 按 scope 匹配一版价格：候选由 SQL 按优先级取，最终选择交给
// billing.Select（“谁赢”只有一份实现）。
func (s *Service) findPrice(itemCode, accountID string, refs billing.ResourceRef, at time.Time) (billing.Price, bool, error) {
	candidates, err := s.store.FindPriceCandidates(itemCode, accountID, refs.Pairs(), at, 20)
	if err != nil {
		return billing.Price{}, false, err
	}
	prices := make([]billing.Price, 0, len(candidates))
	for _, c := range candidates {
		prices = append(prices, domainPrice(c))
	}
	p, ok := billing.Select(prices, accountID, refs, at)
	return p, ok, nil
}

// isNotFound 统一把仓储层的“没找到”翻译成调用方好判断的形式。
func isNotFound(err error) bool {
	return errors.Is(err, store.ErrNotFound)
}

func validateSubject(subjectType, subjectID string) error {
	if strings.TrimSpace(subjectType) == "" || strings.TrimSpace(subjectID) == "" {
		return fmt.Errorf("%w: subject_type and subject_id are required", ErrInvalidRequest)
	}
	return nil
}
