package service

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// Settle 结算一个会话：立刻把它的 pending 事件结算掉（不等 worker 下一轮 tick）、
// 释放预冻结、返回实际金额。挂断电话余额就定下来了（§14.5）。
//
// 有一条硬要求落在数据面那边：先 flush 成功，再调 settle。反过来的话，pending
// 事件会在 settle 之后才到——worker 最终还是会结算它们，但用户会先看到余额没变、
// 过一会儿又变了。
//
// 幂等键是 settle:<session_id>：重复调用返回第一次的结果。
func (s *Service) Settle(ctx context.Context, req billing.SettleRequest) (billing.SettleResponse, error) {
	if req.SessionID == "" {
		return billing.SettleResponse{}, fmt.Errorf("%w: session_id is required", ErrInvalidRequest)
	}

	reservation, err := s.store.GetReservationBySession(req.SessionID)
	if err != nil {
		if isNotFound(err) {
			logging.Warnf("billing: settle for unknown session=%s", req.SessionID)
			return billing.SettleResponse{Settled: false}, nil
		}
		return billing.SettleResponse{}, err
	}

	if err := s.settleSessionEvents(ctx, req.SessionID); err != nil {
		return billing.SettleResponse{}, err
	}

	if err := s.releaseReservation(ctx, reservation); err != nil {
		return billing.SettleResponse{}, err
	}

	charged, err := s.store.SumSessionAmount(req.SessionID)
	if err != nil {
		return billing.SettleResponse{}, err
	}
	balance := int64(0)
	if account, err := s.store.GetAccount(reservation.AccountID); err == nil {
		balance = account.BalanceMicro
	}
	return billing.SettleResponse{
		ChargedMicro:  charged,
		ReleasedMicro: reservation.ReservedMicro,
		BalanceMicro:  balance,
		Settled:       true,
	}, nil
}

// settleSessionEvents 立刻结算这个会话的 pending 事件。并发场景下由账户行锁与
// 流水幂等键兜底，不会重复扣钱。
func (s *Service) settleSessionEvents(ctx context.Context, sessionID string) error {
	for attempt := 0; attempt < maxEventSettleAttempts; attempt++ {
		events, err := s.store.ListPendingEventsBySession(sessionID, s.cfg.WorkerBatch)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		if err := s.settleEvents(ctx, events); err != nil {
			return err
		}
		if len(events) < s.cfg.WorkerBatch {
			return nil
		}
	}
	return nil
}

// ReapExpiredReservations 回收沉默超时的预冻结：把 open 标记成 expired 并释放冻结。
//
// 这里容易搞错的一点：回收只释放冻结，**不取消已经上报的用量**。用量是事实，事件
// 照旧会被 worker 结算；回收之后余额可能被扣成负数，那由透支策略去兜（§16.3）。
func (s *Service) ReapExpiredReservations(ctx context.Context) (int, error) {
	now := s.now()
	var expired []store.BillingReservation
	err := s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := s.store.ExpireReservations(tx, now)
		if err != nil {
			return err
		}
		expired = rows
		for i := range rows {
			row := rows[i]
			key := "release:" + row.ID
			if row.ReservedMicro > 0 {
				if err := s.releaseFrozen(ctx, tx, row.AccountID, row.ReservedMicro, key, "reservation expired"); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, row := range expired {
		// 这条日志的意思是“数据面漏了一次 settle，而且已经沉默过一个窗口”，
		// 出现频率值得盯（§16.3）。
		logging.Warnf("billing: reservation expired session=%s account=%s released=%d",
			row.SessionID, row.AccountID, row.ReservedMicro)
	}
	return len(expired), nil
}

// ExpireGrants 把过期赠款作废：写一条 expire 的反向流水，别删行、也别悄悄改数（§12）。
func (s *Service) ExpireGrants(ctx context.Context) (int, error) {
	now := s.now()
	var expired []store.BillingGrant
	err := s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := s.store.ExpiredGrants(tx, now)
		if err != nil {
			return err
		}
		expired = rows
		for i := range rows {
			row := rows[i]
			entry := &store.BillingLedger{
				AccountID:         row.AccountID,
				Direction:         billing.DirectionDebit,
				AmountMicro:       row.GrantedMicro - row.UsedMicro,
				BalanceAfterMicro: 0,
				Kind:              billing.LedgerKindExpire,
				ItemCode:          row.ItemCode,
				RefType:           billing.RefManual,
				RefID:             row.ID,
				IdempotencyKey:    "expire:" + row.ID,
				OccurredAt:        now,
				Note:              "grant expired",
			}
			if err := s.store.InsertLedger(tx, entry); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(expired), nil
}

// BalanceOf 返回某个主体的账户余额快照（查询用）。账户不存在时返回 (nil, nil)。
func (s *Service) BalanceOf(ctx context.Context, subjectType, subjectID string) (*store.BillingAccount, error) {
	account, err := s.store.GetAccountBySubject(subjectType, subjectID)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return account, nil
}
