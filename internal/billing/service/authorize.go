package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// Authorize 是会话建立时的准入校验：按 device 反查账户、匹配这次会话要用的价格、
// 冻结预留额，返回决策与本地熔断参数（§14.3）。
//
// 幂等靠 session_id 唯一：断线重连再发一次会拿回同一条 reservation，不重复冻结。
func (s *Service) Authorize(ctx context.Context, req billing.AuthorizeRequest) (billing.AuthorizeResponse, error) {
	if s.resolver == nil {
		return billing.AuthorizeResponse{}, fmt.Errorf("%w: subject resolver is not configured", ErrInvalidRequest)
	}
	if req.DeviceID == "" || req.SessionID == "" {
		return billing.AuthorizeResponse{}, fmt.Errorf("%w: device_id and session_id are required", ErrInvalidRequest)
	}
	now := s.now()

	subjectType, subjectID, err := s.resolver.ResolveAccount(ctx, req.DeviceID)
	if err != nil {
		return billing.AuthorizeResponse{}, fmt.Errorf("resolve account for device %s: %w", req.DeviceID, err)
	}
	if subjectID == "" {
		return rejected(billing.RejectNoAccount, "", 0, 0), nil
	}

	account, err := s.ensureAccount(ctx, subjectType, subjectID)
	if err != nil {
		return billing.AuthorizeResponse{}, err
	}
	if account.Status != billing.AccountStatusActive {
		logging.Warnf("billing: authorize rejected (account_suspended) account=%s device=%s", account.ID, req.DeviceID)
		return rejected(billing.RejectAccountSuspended, account.ID, account.BalanceMicro, 0), nil
	}

	// 同一个 session 重连：拿回已有的 reservation，不重复冻结。
	if existing, err := s.store.GetReservationBySession(req.SessionID); err == nil {
		return s.authorizeFromReservation(account, existing), nil
	} else if !isNotFound(err) {
		return billing.AuthorizeResponse{}, err
	}

	profile, err := s.resolver.ResolveSessionProfile(ctx, req.DeviceID)
	if err != nil {
		return billing.AuthorizeResponse{}, fmt.Errorf("resolve session profile for device %s: %w", req.DeviceID, err)
	}

	items, err := s.store.ListItems()
	if err != nil {
		return billing.AuthorizeResponse{}, err
	}

	snap, missing, err := s.buildSnapshot(account.ID, profile, items, now)
	if err != nil {
		return billing.AuthorizeResponse{}, err
	}
	if missing != "" {
		// 价格查不到时必须拒绝会话，不能按 0 元放行：那等于给用户开了免费服务，
		// 而且不会有任何报错（§14.3）。
		logging.Warnf("billing: authorize rejected (price_missing) item=%s account=%s device=%s", missing, account.ID, req.DeviceID)
		resp := rejected(billing.RejectPriceMissing, account.ID, account.BalanceMicro, 0)
		resp.PriceSnapshot = snapshotList(snap)
		return resp, nil
	}

	required := s.reserveAmount(snap, items)
	if required > account.BalanceMicro+account.CreditLimitMicro-account.FrozenMicro {
		logging.Warnf("billing: authorize rejected (insufficient_balance) account=%s balance=%d required=%d",
			account.ID, account.BalanceMicro, required)
		resp := rejected(billing.RejectInsufficientBalance, account.ID, account.BalanceMicro, required)
		resp.PriceSnapshot = snapshotList(snap)
		return resp, nil
	}

	reservation, err := s.createReservation(ctx, account, req, profile, snap, required, now)
	if err != nil {
		if errors.Is(err, ErrInsufficientBalance) {
			resp := rejected(billing.RejectInsufficientBalance, account.ID, account.BalanceMicro, required)
			resp.PriceSnapshot = snapshotList(snap)
			return resp, nil
		}
		return billing.AuthorizeResponse{}, err
	}

	logging.Infof("billing: authorized account=%s device=%s session=%s reserved=%d expires_at=%s",
		account.ID, req.DeviceID, req.SessionID, reservation.ReservedMicro, reservation.ExpiresAt.Format(time.RFC3339))
	return s.authorizeFromReservation(account, reservation), nil
}

func rejected(reason billing.RejectReason, accountID string, balanceMicro, requiredMicro int64) billing.AuthorizeResponse {
	return billing.AuthorizeResponse{
		Allowed:       false,
		AccountID:     accountID,
		RejectReason:  reason,
		BalanceMicro:  balanceMicro,
		RequiredMicro: requiredMicro,
	}
}

// authorizeFromReservation 把一条已有的预冻结还原成 authorize 响应（幂等重放）。
func (s *Service) authorizeFromReservation(account *store.BillingAccount, r *store.BillingReservation) billing.AuthorizeResponse {
	if r.AccountID != "" && r.AccountID != account.ID {
		// 数据面用同一个 session_id 换了 device：不共享别人的预冻结。
		return rejected(billing.RejectNoAccount, account.ID, account.BalanceMicro, 0)
	}
	if r.Status != billing.ReservationStatusOpen && r.Status != billing.ReservationStatusDegraded {
		// 已经结算/回收过的 session_id 又来了：不重新开一个会话（那会让同一笔钱
		// 被花两次），直接拒。
		logging.Warnf("billing: authorize for already %s reservation session=%s", r.Status, r.SessionID)
		return rejected(billing.RejectSessionClosed, account.ID, account.BalanceMicro, 0)
	}
	expiresAt := r.ExpiresAt
	return billing.AuthorizeResponse{
		Allowed:           true,
		AccountID:         account.ID,
		ReservationID:     r.ID,
		ReservedMicro:     r.ReservedMicro,
		MaxSessionSeconds: s.cfg.ReserveSeconds,
		BalanceMicro:      account.BalanceMicro,
		ExpiresAt:         &expiresAt,
		PriceSnapshot:     snapshotList(snapshotFromJSON(r.PriceSnapshot)),
	}
}

// buildSnapshot 为这次会话要用到的每个计费项匹配一版价格，产出下发快照。
// 返回的第一个返回值是快照，第二个非空表示某个计费项没有价格（price_missing）。
func (s *Service) buildSnapshot(accountID string, profile billing.SessionProfile, items []store.BillingItem, at time.Time) (map[string]billing.PriceSnapshot, string, error) {
	snap := make(map[string]billing.PriceSnapshot, len(items))
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		// 只有会话内产生的用量才在 authorize 时定；voice:clone / mcp / session
		// 这些控制面计量点由各自的业务动作直接报账（§6.3 / §6.4）。
		ref := profile.RefFor(item.MeterSource)
		switch item.MeterSource {
		case billing.MeterSourceLLM, billing.MeterSourceTTS, billing.MeterSourceASR:
		default:
			continue
		}

		price, ok, err := s.findPrice(item.Code, accountID, ref, at)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			snap[item.Code] = billing.PriceSnapshot{ItemCode: item.Code, Billable: false, Reason: "no:price"}
			return snap, item.Code, nil
		}
		entry := billing.PriceSnapshot{
			ItemCode:       item.Code,
			Billable:       ref.System,
			UnitPriceMicro: price.UnitPriceMicro,
			UnitSize:       price.UnitSize,
		}
		if !ref.System {
			// BYOK：用户已经直接付给厂商，再收一次是错账（§7）。仍然写用量事件，
			// 金额为 0——“限额”与“计费”两件事解耦。
			entry.Reason = string(billing.SkipReasonBYOK)
		}
		snap[item.Code] = entry
	}
	return snap, "", nil
}

// reserveAmount 是 P1 的预留额：ReserveSeconds 秒的成本，向上取整。
// 配置了 reserve_rate_micro 就用它，否则按 duration 类计费项的单价估算。
func (s *Service) reserveAmount(snap map[string]billing.PriceSnapshot, items []store.BillingItem) int64 {
	if s.cfg.ReserveRateMicro > 0 {
		return int64(s.cfg.ReserveSeconds) * s.cfg.ReserveRateMicro
	}
	durations := make(map[string]int64)
	for _, item := range items {
		if !item.Enabled || billing.ChargeMode(item.ChargeMode) != billing.ChargeModeDuration {
			continue
		}
		if entry, ok := snap[item.Code]; ok && entry.Billable {
			durations[item.Code] = int64(s.cfg.ReserveSeconds)
		}
	}
	return billing.Estimate(durations, snap)
}

// createReservation 在一个事务里锁账户、冻结预留额、写 reservation。
// 账户锁在前、账期行在后，和结算事务的锁序一致（§16.2）。
func (s *Service) createReservation(
	ctx context.Context,
	account *store.BillingAccount,
	req billing.AuthorizeRequest,
	profile billing.SessionProfile,
	snap map[string]billing.PriceSnapshot,
	required int64,
	now time.Time,
) (*store.BillingReservation, error) {
	resourceJSON, err := toJSONMap(profile)
	if err != nil {
		return nil, err
	}
	snapshotJSON, err := toJSONMap(snap)
	if err != nil {
		return nil, err
	}

	reservation := &store.BillingReservation{
		ID:               uuid.NewString(),
		SessionID:        req.SessionID,
		AccountID:        account.ID,
		DeviceID:         req.DeviceID,
		VoicebotID:       profile.VoicebotID,
		ReservedMicro:    required,
		Status:           billing.ReservationStatusOpen,
		ExpiresAt:        now.Add(time.Duration(s.cfg.ReserveSeconds)*time.Second + time.Minute),
		ResourceSnapshot: resourceJSON,
		PriceSnapshot:    snapshotJSON,
	}

	err = s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := s.store.LockAccount(tx, account.ID)
		if err != nil {
			return err
		}
		if required > locked.BalanceMicro+locked.CreditLimitMicro-locked.FrozenMicro {
			return ErrInsufficientBalance
		}
		if err := s.store.CreateReservation(reservation); err != nil {
			return err
		}
		return s.store.UpdateAccountBalance(tx, locked.ID, locked.BalanceMicro, locked.FrozenMicro+required)
	})
	if err != nil {
		return nil, err
	}
	return reservation, nil
}

// ReclaimReservation 释放一条预冻结（会话正常结束或超时回收）。
// 只释放冻结，不取消已经上报的用量：用量是事实（§16.3）。
func (s *Service) releaseFrozen(ctx context.Context, tx *gorm.DB, accountID string, amount int64, idempotencyKey, note string) error {
	if amount <= 0 {
		return nil
	}
	locked, err := s.store.LockAccount(tx, accountID)
	if err != nil {
		return err
	}
	entry := &store.BillingLedger{
		AccountID:         locked.ID,
		Direction:         billing.DirectionCredit,
		AmountMicro:       amount,
		BalanceAfterMicro: locked.BalanceMicro,
		Kind:              billing.LedgerKindRelease,
		RefType:           billing.RefReservation,
		RefID:             idempotencyKey,
		IdempotencyKey:    idempotencyKey,
		OccurredAt:        s.now(),
		Note:              note,
	}
	if err := s.store.InsertLedger(tx, entry); err != nil {
		// 唯一键冲突 = 这笔释放已经做过了，幂等重放直接放过。
		logging.Infof("billing: release already recorded key=%s", idempotencyKey)
		return nil
	}
	return s.store.UpdateAccountBalance(tx, locked.ID, locked.BalanceMicro, locked.FrozenMicro-amount)
}

func toJSONMap(v any) (datatypes.JSONMap, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("billing: marshal snapshot: %w", err)
	}
	out := datatypes.JSONMap{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("billing: unmarshal snapshot: %w", err)
	}
	return out, nil
}

func snapshotFromJSON(raw datatypes.JSONMap) map[string]billing.PriceSnapshot {
	out := make(map[string]billing.PriceSnapshot, len(raw))
	if len(raw) == 0 {
		return out
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return out
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return map[string]billing.PriceSnapshot{}
	}
	return out
}

// profileFromJSON 还原 authorize 时存下的资源快照。
func profileFromJSON(raw datatypes.JSONMap) billing.SessionProfile {
	if len(raw) == 0 {
		return billing.SessionProfile{}
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return billing.SessionProfile{}
	}
	var profile billing.SessionProfile
	if err := json.Unmarshal(payload, &profile); err != nil {
		return billing.SessionProfile{}
	}
	return profile
}

func snapshotList(snap map[string]billing.PriceSnapshot) []billing.PriceSnapshot {
	if len(snap) == 0 {
		return nil
	}
	codes := make([]string, 0, len(snap))
	for code := range snap {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := make([]billing.PriceSnapshot, 0, len(codes))
	for _, code := range codes {
		out = append(out, snap[code])
	}
	return out
}
