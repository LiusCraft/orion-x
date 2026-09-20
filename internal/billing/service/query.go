package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/store"
)

// Summary 是用户端 /api/billing/summary 的形状：余额 + 账期内消耗 + 按计费项 Top。
type Summary struct {
	Account            *store.BillingAccount  `json:"account"`
	Currency           string                 `json:"currency"`
	BalanceMicro       int64                  `json:"balance_micro"`
	FrozenMicro        int64                  `json:"frozen_micro"`
	CreditLimitMicro   int64                  `json:"credit_limit_micro"`
	PeriodChargedMicro int64                  `json:"period_charged_micro"`
	TopItems           []store.UsageAggregate `json:"top_items,omitempty"`
	From               time.Time              `json:"from"`
	To                 time.Time              `json:"to"`
}

// Summary 汇总某个主体的账户与账期消耗。账期默认是自然月（按配置时区）。
func (s *Service) Summary(ctx context.Context, subjectType, subjectID string, from, to time.Time) (Summary, error) {
	if err := validateSubject(subjectType, subjectID); err != nil {
		return Summary{}, err
	}
	now := s.now()
	if from.IsZero() {
		from = s.periodStart(now)
	}
	if to.IsZero() {
		to = now
	}

	summary := Summary{Currency: s.cfg.Currency, From: from, To: to}
	account, err := s.store.GetAccountBySubject(subjectType, subjectID)
	if err != nil {
		if isNotFound(err) {
			return summary, nil
		}
		return Summary{}, err
	}
	summary.Account = account
	summary.Currency = account.Currency
	summary.BalanceMicro = account.BalanceMicro
	summary.FrozenMicro = account.FrozenMicro
	summary.CreditLimitMicro = account.CreditLimitMicro

	top, err := s.store.SumUsageByItem(account.ID, from, to, 10)
	if err != nil {
		return Summary{}, err
	}
	summary.TopItems = top
	for _, item := range top {
		summary.PeriodChargedMicro += item.AmountMicro
	}
	return summary, nil
}

// Items 返回计费项目录。
func (s *Service) Items(ctx context.Context) ([]store.BillingItem, error) {
	return s.store.ListItems()
}

// SetItemEnabled 启停一个计费项。互斥口径（tts:characters 与 tts:audio:seconds）
// 靠它卡住，避免同一份用量被计两次。
func (s *Service) SetItemEnabled(ctx context.Context, code string, enabled bool) error {
	item, err := s.store.GetItem(code)
	if err != nil {
		if isNotFound(err) {
			return ErrItemNotFound
		}
		return err
	}
	if item.IsSystem && !enabled {
		// 系统内置项也允许停用（互斥口径需要），但不允许删除 code。
		return s.store.SetItemEnabled(code, false)
	}
	return s.store.SetItemEnabled(code, enabled)
}

// Prices 按过滤条件返回价格版本。
func (s *Service) Prices(ctx context.Context, q store.PriceQuery) ([]store.BillingPrice, error) {
	return s.store.ListPrices(q)
}

// PublicPrices 返回当前生效的平台标准价（用户端价格公示用）。
func (s *Service) PublicPrices(ctx context.Context) ([]store.BillingPrice, error) {
	now := s.now()
	return s.store.ListPrices(store.PriceQuery{PlatformOnly: true, ActiveAt: &now, Limit: 200})
}

// CreatePrice 落一版新价格。校验不通过就在录入时报，别留到结算时算出一笔怪账。
func (s *Service) CreatePrice(ctx context.Context, p store.BillingPrice) (*store.BillingPrice, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Currency == "" {
		p.Currency = s.cfg.Currency
	}
	if p.UnitSize <= 0 {
		p.UnitSize = 1
	}
	if p.ResourceType == "" {
		p.ResourceType = string(billing.ResourceItem)
	}
	if p.Rounding == "" {
		if item, err := s.store.GetItem(p.ItemCode); err == nil {
			p.Rounding = string(billing.DefaultRoundingFor(billing.ChargeMode(item.ChargeMode)))
		} else {
			p.Rounding = string(billing.RoundingNone)
		}
	}
	if p.EffectiveFrom.IsZero() {
		p.EffectiveFrom = s.now()
	}
	if err := domainPrice(p).Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if err := s.store.CreatePrice(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdatePrice 局部更新一版价格。要停用已经生效的价格就把 effective_to 收到当前
// 时刻，不删行——历史账单还指着它。
func (s *Service) UpdatePrice(ctx context.Context, id string, updates map[string]any) (*store.BillingPrice, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: price id is required", ErrInvalidRequest)
	}
	updated, err := s.store.UpdatePrice(id, updates)
	if err != nil {
		return nil, err
	}
	if err := domainPrice(*updated).Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	return updated, nil
}

// DeletePrice 删除一版还没生效的价格。已经生效的价格不要删——把 effective_to
// 收到当前时刻即可（§3.2）。
func (s *Service) DeletePrice(ctx context.Context, id string) error {
	return s.store.DeletePrice(id)
}

// Accounts 返回账户列表（管理端）。
func (s *Service) Accounts(ctx context.Context, q store.AccountQuery) ([]store.BillingAccount, int64, error) {
	list, err := s.store.ListAccounts(q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountAccounts(q)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// Ledger 返回流水（管理端 /api/billing/ledger）。
func (s *Service) Ledger(ctx context.Context, q store.LedgerQuery) ([]store.BillingLedger, int64, error) {
	list, err := s.store.ListLedger(q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountLedger(q)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// UsageEvents 返回用量明细（用户端 /api/billing/usage）。
func (s *Service) UsageEvents(ctx context.Context, q store.UsageQuery) ([]store.BillingUsageEvent, int64, error) {
	list, err := s.store.ListUsageEvents(q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountUsageEvents(q)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// Reservations 返回预冻结列表（排查悬挂冻结用）。
func (s *Service) Reservations(ctx context.Context, q store.ReservationQuery) ([]store.BillingReservation, error) {
	return s.store.ListReservations(q)
}
