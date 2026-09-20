package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// 这个文件是结算引擎的落库侧（§5 / §16.2）：settleEvents 是唯一的扣费路径，
// 数据面事件、控制面业务动作走的都是它。
//
// 一个事件在同一个事务里走完：锁账户行 → 锁账期行 → Compute → 扣额度/余额 →
// 写流水 → 标记事件 → 累加账期 → upsert 日聚合。锁序固定为「先 account 再
// period state」，多实例并发才不会互相咬死。

// settleEvents 按账户分组结算一批事件，每个账户一个事务：一个账户失败不影响
// 其它账户，重放时该账户的事件仍是 pending，下次会再被认领。
//
// settleSessionEvents 最多轮询几批，避免一个会话的事件多到一次结算不完时
// 在同一个请求里无限循环。
const maxEventSettleAttempts = 3

func (s *Service) settleEvents(ctx context.Context, events []store.BillingUsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	items, err := s.loadItems()
	if err != nil {
		return err
	}

	byAccount := groupByAccount(events)
	var firstErr error
	for _, accountID := range sortedKeys(byAccount) {
		batch := byAccount[accountID]
		if err := s.settleAccountBatch(ctx, accountID, batch, items); err != nil {
			logging.Errorf("billing: settle account=%s events=%d: %v", accountID, len(batch), err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *Service) settleAccountBatch(ctx context.Context, accountID string, events []store.BillingUsageEvent, items map[string]store.BillingItem) error {
	return s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		account, err := s.store.LockAccount(tx, accountID)
		if err != nil {
			return err
		}

		balanceChanged := false
		for i := range events {
			outcome, err := s.settleEventInTx(tx, account, events[i], items)
			if err != nil {
				return err
			}
			if outcome.balanceDelta != 0 {
				account.BalanceMicro += outcome.balanceDelta
				balanceChanged = true
			}
			if outcome.suspend {
				if err := s.suspendAccount(tx, account); err != nil {
					return err
				}
				return nil
			}
		}
		if !balanceChanged {
			return nil
		}
		return s.store.UpdateAccountBalance(tx, account.ID, account.BalanceMicro, account.FrozenMicro)
	})
}

// eventOutcome 是单条事件的结算结果。
type eventOutcome struct {
	balanceDelta int64 // 账户余额的变化量（负数 = 扣款）
	suspend      bool  // deny 策略下透支：事件标 unpaid，账户停服
}

func (s *Service) suspendAccount(tx *gorm.DB, account *store.BillingAccount) error {
	account.Status = billing.AccountStatusSuspended
	if err := s.store.UpdateAccountBalance(tx, account.ID, account.BalanceMicro, account.FrozenMicro); err != nil {
		return err
	}
	return tx.Model(&store.BillingAccount{}).Where("id = ?", account.ID).
		Update("status", billing.AccountStatusSuspended).Error
}

// settleEventInTx 结算一条事件。它不改账户行（余额变化通过 outcome 交回调用方，
// 由调用方在每个账户批次末尾一次性写回），避免每条事件都打一次 UPDATE。
func (s *Service) settleEventInTx(
	tx *gorm.DB,
	account *store.BillingAccount,
	ev store.BillingUsageEvent,
	items map[string]store.BillingItem,
) (eventOutcome, error) {
	priceTime := s.priceTimeOf(ev)

	item, ok := items[ev.ItemCode]
	if !ok {
		return s.markSkipped(tx, ev, "unknown:item")
	}
	if !item.Enabled {
		return s.markSkipped(tx, ev, string(billing.SkipReasonItemDisabled))
	}

	refs := s.refsForEvent(ev)
	price, ok, err := s.findPrice(ev.ItemCode, account.ID, refs, priceTime)
	if err != nil {
		return eventOutcome{}, err
	}
	if !ok {
		// 价格查不到时不能按 0 元记一笔：标 unpaid 留着，人工补价格后重放（§17）。
		logging.Errorf("billing: no price for item=%s account=%s event=%s", ev.ItemCode, account.ID, ev.ID)
		return s.markUnsettled(tx, ev, "no:price")
	}

	// 阶梯价与起步价必须按账期累计量做边际计价（§16.1）。
	state, err := s.store.LockPeriodState(tx, account.ID, ev.ItemCode, price.ID, s.periodStart(priceTime))
	if err != nil {
		return eventOutcome{}, err
	}
	result := billing.Compute(domainEvent(ev), price, billing.PeriodState{
		Quantity:       state.Quantity,
		AmountMicro:    state.AmountMicro,
		GrantUsedMicro: state.GrantUsedMicro,
	})
	if result.Skipped {
		return s.markSkipped(tx, ev, string(result.SkipReason))
	}

	// 免费额度：先扣 grant，不够的部分走余额。额度是限定计费项的，按 item 取。
	available, err := s.availableGrant(tx, account.ID, ev.ItemCode, priceTime)
	if err != nil {
		return eventOutcome{}, err
	}
	result = result.ApplyGrant(available)

	balanceAfter := account.BalanceMicro - result.BalanceChargedMicro
	if balanceAfter < -account.CreditLimitMicro {
		if s.cfg.OverdraftPolicy != billing.OverdraftAllow {
			// deny（默认，预付费）：事件标 unpaid 并告警，账户置 suspended，
			// 数据面准入随即拒绝。扣款不落账，事件留着人工重放。
			logging.Errorf("billing: overdraft denied account=%s event=%s balance=%d charge=%d limit=%d",
				account.ID, ev.ID, account.BalanceMicro, result.BalanceChargedMicro, account.CreditLimitMicro)
			outcome, err := s.markUnsettled(tx, ev, "overdraft")
			outcome.suspend = true
			return outcome, err
		}
		logging.Warnf("billing: account=%s overdrawn to %d (limit %d) by event=%s",
			account.ID, balanceAfter, account.CreditLimitMicro, ev.ID)
	}

	note := ""
	if result.GrantCoveredMicro > 0 {
		note = fmt.Sprintf("grant_covered=%d", result.GrantCoveredMicro)
	}
	entry := &store.BillingLedger{
		AccountID:         account.ID,
		Direction:         billing.DirectionDebit,
		AmountMicro:       result.AmountMicro,
		BalanceAfterMicro: balanceAfter,
		Kind:              billing.LedgerKindCharge,
		ItemCode:          ev.ItemCode,
		RefType:           billing.RefUsageEvent,
		RefID:             ev.ID,
		IdempotencyKey:    billing.EventIdempotencyKey(ev.ID),
		OccurredAt:        s.now(),
		Note:              note,
	}
	if err := s.store.InsertLedger(tx, entry); err != nil {
		// 幂等键冲突 = 这条事件已经入过账（worker 重放或并发 settle），
		// 直接把事件标成 charged 就好，不再动钱。
		logging.Infof("billing: event already charged event=%s: %v", ev.ID, err)
		return eventOutcome{}, s.store.MarkEventSettled(tx, ev.ID, billing.EventStatusCharged, price.ID, result.AmountMicro, "")
	}

	if err := s.consumeGrants(tx, account.ID, ev.ItemCode, result.GrantCoveredMicro, priceTime); err != nil {
		return eventOutcome{}, err
	}
	if err := s.store.MarkEventSettled(tx, ev.ID, billing.EventStatusCharged, price.ID, result.AmountMicro, ""); err != nil {
		return eventOutcome{}, err
	}
	state.Quantity += ev.Quantity
	state.AmountMicro += result.AmountMicro
	state.GrantUsedMicro += result.GrantCoveredMicro
	if err := s.store.UpdatePeriodState(tx, *state); err != nil {
		return eventOutcome{}, err
	}
	if err := s.store.UpsertDailyStat(tx, store.BillingDailyStat{
		AccountID:   account.ID,
		Date:        priceTime.In(s.cfg.PeriodLocation).Format("2006-01-02"),
		ItemCode:    ev.ItemCode,
		AIModelID:   ev.AIModelID,
		VoiceID:     ev.VoiceID,
		Quantity:    ev.Quantity,
		AmountMicro: result.AmountMicro,
		EventCount:  1,
	}); err != nil {
		return eventOutcome{}, err
	}

	return eventOutcome{balanceDelta: -result.BalanceChargedMicro}, nil
}

func (s *Service) markSkipped(tx *gorm.DB, ev store.BillingUsageEvent, reason string) (eventOutcome, error) {
	if err := s.store.MarkEventSettled(tx, ev.ID, billing.EventStatusSkipped, "", 0, reason); err != nil {
		return eventOutcome{}, err
	}
	return eventOutcome{}, nil
}

func (s *Service) markUnsettled(tx *gorm.DB, ev store.BillingUsageEvent, reason string) (eventOutcome, error) {
	if err := s.store.MarkEventSettled(tx, ev.ID, billing.EventStatusUnpaid, "", 0, reason); err != nil {
		return eventOutcome{}, err
	}
	return eventOutcome{}, nil
}

// availableGrant 返回该账户这个计费项当前可用的赠送额度（未过期、未用尽的部分之和）。
//
// billing_grants 是**限定计费项**的：tts:characters 的赠款不能拿来抵 llm 的 token，
// 所以这里按 item_code 过滤，不能把账户名下的额度全加起来。
func (s *Service) availableGrant(tx *gorm.DB, accountID, itemCode string, at time.Time) (int64, error) {
	grants, err := s.store.ListGrants(tx, accountID, at)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, g := range grants {
		if g.ItemCode != itemCode {
			continue
		}
		if remaining := g.GrantedMicro - g.UsedMicro; remaining > 0 {
			total += remaining
		}
	}
	return total, nil
}

// consumeGrants 按到期先后消耗这个计费项的赠送额度；额度不够的部分由余额承担。
func (s *Service) consumeGrants(tx *gorm.DB, accountID, itemCode string, amount int64, at time.Time) error {
	if amount <= 0 {
		return nil
	}
	grants, err := s.store.ListGrants(tx, accountID, at)
	if err != nil {
		return err
	}
	remaining := amount
	for _, g := range grants {
		if remaining <= 0 {
			break
		}
		if g.ItemCode != itemCode {
			continue
		}
		room := g.GrantedMicro - g.UsedMicro
		if room <= 0 {
			continue
		}
		use := min(room, remaining)
		if err := s.store.UseGrant(tx, g.ID, use); err != nil {
			// 并发把额度用光了：差额留给余额，不算失败。
			logging.Warnf("billing: grant exhausted concurrently account=%s grant=%s", accountID, g.ID)
			continue
		}
		remaining -= use
	}
	return nil
}

// refsForEvent 决定这条事件按哪份资源快照匹配价格。
//
// 以 authorize 时快照进 reservation 的那份为准，事件里带的只用于校验（§14.2）：
// 会话的 pipeline 是连接时按当时加载的配置建的，之后改了 voicebot 的模型配置并
// 不影响这个会话实际在调谁。降级会话没有快照，才退回用事件里带的那份。
func (s *Service) refsForEvent(ev store.BillingUsageEvent) billing.ResourceRef {
	if ev.SessionID != "" {
		if reservation, err := s.store.GetReservationBySession(ev.SessionID); err == nil {
			profile := profileFromJSON(reservation.ResourceSnapshot)
			if ref := profile.RefFor(itemMeterSource(ev.ItemCode)); !ref.IsZero() {
				return ref
			}
		}
	}
	return billing.ResourceRef{ProviderID: ev.ProviderID, ModelID: ev.AIModelID, VoiceID: ev.VoiceID, System: true}
}

// itemMeterSource 返回计费项对应的计量点；不在内置目录里的项按 llm 处理。
func itemMeterSource(itemCode string) string {
	if item, ok := billing.DefaultItem(itemCode); ok {
		return item.MeterSource
	}
	return billing.MeterSourceLLM
}

func (s *Service) loadItems() (map[string]store.BillingItem, error) {
	items, err := s.store.ListItems()
	if err != nil {
		return nil, err
	}
	out := make(map[string]store.BillingItem, len(items))
	for _, item := range items {
		out[item.Code] = item
	}
	return out, nil
}

func groupByAccount(events []store.BillingUsageEvent) map[string][]store.BillingUsageEvent {
	out := make(map[string][]store.BillingUsageEvent)
	for _, ev := range events {
		out[ev.AccountID] = append(out[ev.AccountID], ev)
	}
	return out
}

func sortedKeys(m map[string][]store.BillingUsageEvent) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---------------------------------------------------------------- 控制面业务动作

// Charge 用于控制面里“动作已经完成、立即扣费”的场景（复刻音色、MCP 调用）。
//
// 实现上它写一条幂等键确定的用量事件，再走同一条结算路径：幂等键由外部引用
// 拼出（charge:<ref_type>:<ref_id>），所以重试、重放都安全，也让“这笔账对应
// 哪次业务操作”永远查得到（§6.4）。事务失败时事件还留在 pending，worker 会补扣。
func (s *Service) Charge(ctx context.Context, req billing.ChargeRequest) (billing.ChargeResult, error) {
	if err := validateSubject(req.SubjectType, req.SubjectID); err != nil {
		return billing.ChargeResult{}, err
	}
	if req.ItemCode == "" || req.Quantity <= 0 {
		return billing.ChargeResult{}, fmt.Errorf("%w: item_code and a positive quantity are required", ErrInvalidRequest)
	}

	account, err := s.ensureAccount(ctx, req.SubjectType, req.SubjectID)
	if err != nil {
		return billing.ChargeResult{}, err
	}

	eventID := billing.ChargeIdempotencyKey(req.RefType, req.RefID)
	now := s.now()
	event := store.BillingUsageEvent{
		ID:         eventID,
		AccountID:  account.ID,
		ItemCode:   req.ItemCode,
		Quantity:   req.Quantity,
		ProviderID: dimString(req.Dims, "provider_id"),
		AIModelID:  dimString(req.Dims, "model_id"),
		VoiceID:    dimString(req.Dims, "voice_id"),
		OccurredAt: now,
		ReceivedAt: now,
		Status:     billing.EventStatusPending,
		Dimensions: dimsOf(req.Dims),
	}
	if _, err := s.store.InsertUsageEvents([]store.BillingUsageEvent{event}); err != nil {
		return billing.ChargeResult{}, err
	}

	if err := s.settleEvents(ctx, []store.BillingUsageEvent{event}); err != nil {
		return billing.ChargeResult{}, err
	}

	settled, err := s.store.GetUsageEvent(eventID)
	if err != nil {
		return billing.ChargeResult{}, err
	}
	result := billing.ChargeResult{
		AmountMicro:         settled.AmountMicro,
		BalanceChargedMicro: settled.AmountMicro,
		PriceID:             settled.PriceID,
	}
	switch settled.Status {
	case billing.EventStatusSkipped:
		result.AmountMicro = 0
		result.BalanceChargedMicro = 0
		result.Skipped = true
		result.SkipReason = billing.SkipReason(settled.LastError)
	case billing.EventStatusUnpaid:
		return result, fmt.Errorf("%w: event %s left unpaid (%s)", ErrInsufficientBalance, eventID, settled.LastError)
	}

	// 同一次业务动作可能先前冻结过钱（Reserve → 调厂商 → Charge），捕获之后
	// 把冻结释放掉，别让这笔钱一直占着。
	if err := s.releaseByRef(ctx, req.RefType, req.RefID); err != nil {
		logging.Warnf("billing: release reservation for %s/%s: %v", req.RefType, req.RefID, err)
	}
	return result, nil
}

// Reserve 用于控制面里“动作还没做”的场景：按报价先占住钱，做完再 Charge 或
// 失败时 Release。占住的额度进 billing_accounts.frozen_micro，准入公式统一为
// `amount ≤ balance + credit_limit − frozen`。
func (s *Service) Reserve(ctx context.Context, req billing.ReserveRequest) (billing.Reservation, error) {
	if err := validateSubject(req.SubjectType, req.SubjectID); err != nil {
		return billing.Reservation{}, err
	}
	if req.ItemCode == "" || req.Quantity <= 0 {
		return billing.Reservation{}, fmt.Errorf("%w: item_code and a positive quantity are required", ErrInvalidRequest)
	}

	account, err := s.ensureAccount(ctx, req.SubjectType, req.SubjectID)
	if err != nil {
		return billing.Reservation{}, err
	}

	price, ok, err := s.findPrice(req.ItemCode, account.ID, refsOfDims(req.Dims), s.now())
	if err != nil {
		return billing.Reservation{}, err
	}
	if !ok {
		// 价格查不到时必须拒绝，不能按 0 元放行（§14.3）。
		return billing.Reservation{}, fmt.Errorf("%w: no price for item %s", ErrItemNotFound, req.ItemCode)
	}
	amount := price.PriceOf(req.Quantity)

	key := billing.ReserveIdempotencyKey(req.RefType, req.RefID)
	if existing, err := s.store.GetReservationBySession(key); err == nil {
		return reservationFromStore(existing), nil
	} else if !isNotFound(err) {
		return billing.Reservation{}, err
	}

	now := s.now()
	reservation := &store.BillingReservation{
		ID:            shortID(),
		SessionID:     key,
		AccountID:     account.ID,
		ReservedMicro: amount,
		Status:        billing.ReservationStatusOpen,
		ExpiresAt:     now.Add(s.cfg.ReserveTTL),
	}
	err = s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := s.store.LockAccount(tx, account.ID)
		if err != nil {
			return err
		}
		if amount > locked.BalanceMicro+locked.CreditLimitMicro-locked.FrozenMicro {
			return ErrInsufficientBalance
		}
		if err := s.store.CreateReservation(reservation); err != nil {
			return err
		}
		entry := &store.BillingLedger{
			AccountID:         locked.ID,
			Direction:         billing.DirectionDebit,
			AmountMicro:       amount,
			BalanceAfterMicro: locked.BalanceMicro,
			Kind:              billing.LedgerKindReserve,
			ItemCode:          req.ItemCode,
			RefType:           billing.RefReservation,
			RefID:             key,
			IdempotencyKey:    billing.ReserveIdempotencyKey(req.RefType, req.RefID),
			OccurredAt:        now,
			Note:              fmt.Sprintf("frozen for %s/%s", req.RefType, req.RefID),
		}
		if err := s.store.InsertLedger(tx, entry); err != nil {
			return err
		}
		return s.store.UpdateAccountBalance(tx, locked.ID, locked.BalanceMicro, locked.FrozenMicro+amount)
	})
	if err != nil {
		return billing.Reservation{}, err
	}
	return reservationFromStore(reservation), nil
}

// Release 释放一笔还没捕获的预冻结（业务动作失败）。参数是 reservation ID。
func (s *Service) Release(ctx context.Context, reservationID string) error {
	if reservationID == "" {
		return fmt.Errorf("%w: reservation_id is required", ErrInvalidRequest)
	}
	reservation, err := s.store.GetReservationBySession(reservationID)
	if err != nil {
		if isNotFound(err) {
			return ErrReservationNotFound
		}
		return err
	}
	return s.releaseReservation(ctx, reservation)
}

// releaseByRef 按外部引用释放预冻结（Charge 捕获后调用）。
func (s *Service) releaseByRef(ctx context.Context, refType, refID string) error {
	reservation, err := s.store.GetReservationBySession(billing.ReserveIdempotencyKey(refType, refID))
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	return s.releaseReservation(ctx, reservation)
}

func (s *Service) releaseReservation(ctx context.Context, reservation *store.BillingReservation) error {
	if reservation.Status != billing.ReservationStatusOpen && reservation.Status != billing.ReservationStatusDegraded {
		return nil // 已经结算/回收过，重复调用是幂等的
	}
	return s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		key := billing.SettleIdempotencyKey(reservation.SessionID)
		if err := s.releaseFrozen(ctx, tx, reservation.AccountID, reservation.ReservedMicro, key, "reservation released"); err != nil {
			return err
		}
		return s.store.UpdateReservation(reservation.ID, map[string]any{
			"status":         billing.ReservationStatusSettled,
			"settled_at":     s.now(),
			"released_micro": reservation.ReservedMicro,
			"last_error":     "",
		})
	})
}

func reservationFromStore(r *store.BillingReservation) billing.Reservation {
	return billing.Reservation{
		ID:          r.ID,
		AccountID:   r.AccountID,
		AmountMicro: r.ReservedMicro,
		ExpiresAt:   r.ExpiresAt,
	}
}

// ---------------------------------------------------------------- 入账 / 人工调整

// CreditRequest 是一笔入账（充值、退款回滚、赠款）。
type CreditRequest struct {
	SubjectType string
	SubjectID   string
	ItemCode    string
	AmountMicro int64
	RefType     string
	RefID       string
	Note        string
	Creator     string
}

// AdjustRequest 是一次人工调整，金额可正可负，必须带备注。
type AdjustRequest struct {
	SubjectType string
	SubjectID   string
	ItemCode    string
	AmountMicro int64
	Note        string
	Creator     string
}

// Credit 入账：钱的来路对计费是黑盒，它只认“一笔钱进来了”。幂等键用外部引用
// （ref_type + ref_id）拼，重放安全（§19）。
func (s *Service) Credit(ctx context.Context, req CreditRequest) (*store.BillingAccount, error) {
	if err := validateSubject(req.SubjectType, req.SubjectID); err != nil {
		return nil, err
	}
	if req.AmountMicro == 0 {
		return nil, fmt.Errorf("%w: amount_micro must not be zero", ErrInvalidRequest)
	}
	account, err := s.ensureAccount(ctx, req.SubjectType, req.SubjectID)
	if err != nil {
		return nil, err
	}
	refType := req.RefType
	if refType == "" {
		refType = billing.RefManual
	}
	key := "credit:" + refType + ":" + req.RefID
	if req.RefID == "" {
		key = "credit:" + refType + ":" + shortID()
	}
	return s.applyBalanceChange(ctx, account.ID, req.AmountMicro, billing.LedgerKindRecharge, req.ItemCode, refType, key, req.Note, req.Creator)
}

// Adjust 人工调整：写 adjust 流水，必须带备注和操作者，不动历史事件行。
func (s *Service) Adjust(ctx context.Context, req AdjustRequest) (*store.BillingAccount, error) {
	if err := validateSubject(req.SubjectType, req.SubjectID); err != nil {
		return nil, err
	}
	if req.AmountMicro == 0 {
		return nil, fmt.Errorf("%w: amount_micro must not be zero", ErrInvalidRequest)
	}
	if strings.TrimSpace(req.Note) == "" {
		return nil, fmt.Errorf("%w: note is required for manual adjustments", ErrInvalidRequest)
	}
	account, err := s.ensureAccount(ctx, req.SubjectType, req.SubjectID)
	if err != nil {
		return nil, err
	}
	return s.applyBalanceChange(ctx, account.ID, req.AmountMicro, billing.LedgerKindAdjust, req.ItemCode, billing.RefManual, "adjust:"+shortID(), req.Note, req.Creator)
}

// Grant 给账户发一笔限定计费项的赠送额度。赠款不写进 balance_micro：那样赠款和
// 充值款在账上就分不开了，退款、对账、算实收全说不清（§12）。
func (s *Service) Grant(ctx context.Context, subjectType, subjectID, itemCode string, amountMicro int64, source, note string) (*store.BillingAccount, error) {
	if err := validateSubject(subjectType, subjectID); err != nil {
		return nil, err
	}
	if amountMicro <= 0 {
		return nil, fmt.Errorf("%w: amount_micro must be positive", ErrInvalidRequest)
	}
	if itemCode == "" {
		return nil, fmt.Errorf("%w: item_code is required", ErrInvalidRequest)
	}
	account, err := s.ensureAccount(ctx, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	err = s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := s.store.LockAccount(tx, account.ID)
		if err != nil {
			return err
		}
		if err := createGrant(tx, locked.ID, itemCode, amountMicro, source, s.cfg.GrantTTL, s.now()); err != nil {
			return err
		}
		entry := &store.BillingLedger{
			AccountID:         locked.ID,
			Direction:         billing.DirectionCredit,
			AmountMicro:       amountMicro,
			BalanceAfterMicro: locked.BalanceMicro,
			Kind:              billing.LedgerKindGrant,
			ItemCode:          itemCode,
			RefType:           billing.RefManual,
			RefID:             source,
			IdempotencyKey:    "grant:" + source + ":" + locked.ID + ":" + itemCode,
			OccurredAt:        s.now(),
			Note:              note,
		}
		return s.store.InsertLedger(tx, entry)
	})
	if err != nil {
		return nil, err
	}
	return s.store.GetAccount(account.ID)
}

func (s *Service) applyBalanceChange(ctx context.Context, accountID string, delta int64, kind, itemCode, refType, idempotencyKey, note, creator string) (*store.BillingAccount, error) {
	var updated *store.BillingAccount
	err := s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := s.store.LockAccount(tx, accountID)
		if err != nil {
			return err
		}
		direction := billing.DirectionCredit
		if delta < 0 {
			direction = billing.DirectionDebit
		}
		entry := &store.BillingLedger{
			AccountID:         locked.ID,
			Direction:         direction,
			AmountMicro:       delta,
			BalanceAfterMicro: locked.BalanceMicro + delta,
			Kind:              kind,
			ItemCode:          itemCode,
			RefType:           refType,
			RefID:             idempotencyKey,
			IdempotencyKey:    idempotencyKey,
			OccurredAt:        s.now(),
			Note:              note,
			Creator:           creator,
		}
		if err := s.store.InsertLedger(tx, entry); err != nil {
			return err
		}
		if err := s.store.UpdateAccountBalance(tx, locked.ID, locked.BalanceMicro+delta, locked.FrozenMicro); err != nil {
			return err
		}
		account, err := s.store.GetAccount(locked.ID)
		if err != nil {
			return err
		}
		updated = account
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ---------------------------------------------------------------- 账户

// ensureAccount 按主体找账户，没有就建一个并发出注册赠款。
// 新用户注册完第一次连设备就被拒绝是必须避免的（§12）。
func (s *Service) ensureAccount(ctx context.Context, subjectType, subjectID string) (*store.BillingAccount, error) {
	if err := validateSubject(subjectType, subjectID); err != nil {
		return nil, err
	}
	account, err := s.store.GetAccountBySubject(subjectType, subjectID)
	if err == nil {
		return account, nil
	}
	if !isNotFound(err) {
		return nil, err
	}

	created := &store.BillingAccount{
		ID:          newAccountID(),
		SubjectType: subjectType,
		SubjectID:   subjectID,
		Currency:    s.cfg.Currency,
		Status:      billing.AccountStatusActive,
	}
	createErr := s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.store.CreateAccount(created); err != nil {
			return err
		}
		return s.issueSignupGrant(tx, created.ID)
	})
	if createErr != nil {
		// 并发建账户：另一个请求赢了唯一索引，回读它。
		if existing, getErr := s.store.GetAccountBySubject(subjectType, subjectID); getErr == nil {
			return existing, nil
		}
		return nil, createErr
	}
	logging.Infof("billing: created account %s for %s/%s", created.ID, subjectType, subjectID)
	return created, nil
}

// issueSignupGrant 发注册赠款：总额按 SignupGrantItems 均分，走 billing_grants。
func (s *Service) issueSignupGrant(tx *gorm.DB, accountID string) error {
	if s.cfg.SignupGrantMicro <= 0 {
		return nil
	}
	items := s.cfg.SignupGrantItems
	if len(items) == 0 {
		items = defaultGrantItems()
	}
	if len(items) == 0 {
		return nil
	}
	per := s.cfg.SignupGrantMicro / int64(len(items))
	if per <= 0 {
		return nil
	}
	for _, code := range items {
		if err := createGrant(tx, accountID, code, per, "signup", s.cfg.GrantTTL, s.now()); err != nil {
			return err
		}
	}
	return nil
}

func createGrant(tx *gorm.DB, accountID, itemCode string, amountMicro int64, source string, ttl time.Duration, now time.Time) error {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	grant := &store.BillingGrant{
		ID:           shortID(),
		AccountID:    accountID,
		ItemCode:     itemCode,
		Source:       source,
		GrantedMicro: amountMicro,
		ExpiresAt:    now.Add(ttl),
	}
	if err := tx.Create(grant).Error; err != nil {
		return fmt.Errorf("billing: create grant: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------- 小工具

func dimsOf(dims map[string]any) map[string]any {
	if len(dims) == 0 {
		return nil
	}
	out := make(map[string]any, len(dims))
	for k, v := range dims {
		out[k] = v
	}
	return out
}

func dimString(dims map[string]any, key string) string {
	if dims == nil {
		return ""
	}
	if v, ok := dims[key].(string); ok {
		return v
	}
	return ""
}

func refsOfDims(dims map[string]any) billing.ResourceRef {
	return billing.ResourceRef{
		ProviderID: dimString(dims, "provider_id"),
		ModelID:    dimString(dims, "model_id"),
		VoiceID:    dimString(dims, "voice_id"),
		System:     true,
	}
}
