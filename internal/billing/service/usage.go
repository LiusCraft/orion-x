package service

import (
	"context"
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// ReportUsage 接收数据面批量上报的用量事实（§14.4）。
//
// 这个接口落库就返回，**不做定价**：定价要匹配价格、要锁账户行，把这两件事塞进
// 上报路径，等于让语音热路径去等数据库的锁。
//
// 账户由控制面自己推：按 session_id 反查 reservation，绝不信任上报体（§14.2）。
func (s *Service) ReportUsage(ctx context.Context, req billing.UsageBatchRequest) (billing.UsageBatchResponse, error) {
	if req.SessionID == "" {
		return billing.UsageBatchResponse{}, fmt.Errorf("%w: session_id is required", ErrInvalidRequest)
	}
	if len(req.Events) > billing.MaxUsageEventsPerBatch {
		return billing.UsageBatchResponse{}, fmt.Errorf("%w: %d events > %d",
			ErrBatchTooLarge, len(req.Events), billing.MaxUsageEventsPerBatch)
	}
	if len(req.Events) == 0 {
		return billing.UsageBatchResponse{}, nil
	}

	now := s.now()
	reservation, err := s.resolveReservation(ctx, req.SessionID, req.DeviceID, now)
	if err != nil {
		return billing.UsageBatchResponse{}, err
	}
	if reservation == nil {
		// 隔离队列：既不静默丢掉，也不记账。
		logging.Warnf("billing: usage batch rejected (unknown_session) session=%s events=%d",
			req.SessionID, len(req.Events))
		return billing.UsageBatchResponse{Rejected: rejectAll(req.Events, "unknown_session")}, nil
	}

	items, err := s.loadItems()
	if err != nil {
		return billing.UsageBatchResponse{}, err
	}
	profile := profileFromJSON(reservation.ResourceSnapshot)
	snap := snapshotFromJSON(reservation.PriceSnapshot)

	events := make([]store.BillingUsageEvent, 0, len(req.Events))
	rejected := make([]billing.RejectedEvent, 0)
	for _, report := range req.Events {
		item, ok := items[report.ItemCode]
		if !ok {
			rejected = append(rejected, billing.RejectedEvent{EventID: report.EventID, Reason: "unknown_item"})
			continue
		}
		if report.EventID == "" {
			rejected = append(rejected, billing.RejectedEvent{Reason: "invalid_event_id"})
			continue
		}
		if report.Quantity < 0 {
			rejected = append(rejected, billing.RejectedEvent{EventID: report.EventID, Reason: "invalid_quantity"})
			continue
		}
		events = append(events, s.newUsageEvent(report, item, reservation, profile, snap, now))
	}

	inserted, err := s.insertUsageEvents(ctx, events, reservation, now)
	if err != nil {
		return billing.UsageBatchResponse{}, err
	}

	// settlement_mode: sync 时就在这个请求里把刚落的账结掉（§5 的开关）。默认的
	// async 让语音热路径只写事实——定价要匹配价格、要锁账户行，塞进上报路径等于让
	// 热路径去等数据库的锁。
	if s.cfg.SettlementMode == billing.SettlementSync && len(events) > 0 {
		if err := s.settleEvents(ctx, events); err != nil {
			// 事件已经落库，重发这批只会撞主键（duplicated），不会重复扣钱。
			return billing.UsageBatchResponse{}, err
		}
	}

	return billing.UsageBatchResponse{
		Accepted:   inserted,
		Duplicated: len(events) - inserted,
		Rejected:   rejected,
	}, nil
}

// newUsageEvent 把一条上报归一成待结算事实。资源 ID 以 authorize 快照为准，事件
// 里带的只用于校验，对不上就记维度并告警。
func (s *Service) newUsageEvent(
	report billing.UsageEventReport,
	item store.BillingItem,
	reservation *store.BillingReservation,
	profile billing.SessionProfile,
	snap map[string]billing.PriceSnapshot,
	now time.Time,
) store.BillingUsageEvent {
	occurredAt := report.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = now
	}

	ref := profile.RefFor(item.MeterSource)
	billable := ref.System
	if entry, ok := snap[report.ItemCode]; ok && !entry.Billable {
		billable = false
	}

	dims := make(map[string]any, len(report.Dimensions)+2)
	for k, v := range report.Dimensions {
		dims[k] = v
	}
	if skew := occurredAt.Sub(now); skew > billing.ClockSkewTolerance || skew < -billing.ClockSkewTolerance {
		// 数据面时钟不可信：记个维度，结算时按 received_at 匹配价格（§4）。
		dims["clock_skew_ms"] = skew.Milliseconds()
		logging.Warnf("billing: clock skew %v session=%s event=%s", skew, reservation.SessionID, report.EventID)
	}
	if mismatch := resourceMismatch(report, ref); mismatch != "" {
		// 会话的 pipeline 是连接时按当时配置建的，事件里的 ID 只是佐证。
		dims["resource_mismatch"] = mismatch
		logging.Warnf("billing: resource mismatch (%s) session=%s event=%s", mismatch, reservation.SessionID, report.EventID)
	}
	if len(dims) == 0 {
		dims = nil
	}

	return store.BillingUsageEvent{
		ID:         report.EventID,
		AccountID:  reservation.AccountID,
		VoicebotID: reservation.VoicebotID,
		DeviceID:   reservation.DeviceID,
		SessionID:  reservation.SessionID,
		ItemCode:   report.ItemCode,
		Quantity:   report.Quantity,
		AIModelID:  ref.ModelID,
		ProviderID: ref.ProviderID,
		VoiceID:    ref.VoiceID,
		BYOK:       !billable,
		OccurredAt: occurredAt,
		ReceivedAt: now,
		TurnIndex:  report.TurnIndex,
		Dimensions: datatypes.JSONMap(dims),
		Status:     billing.EventStatusPending,
	}
}

// resourceMismatch 返回事件与快照对不上的字段名，对得上时返回空串。
func resourceMismatch(report billing.UsageEventReport, ref billing.ResourceRef) string {
	switch {
	case report.ProviderID != "" && ref.ProviderID != "" && report.ProviderID != ref.ProviderID:
		return "provider"
	case report.ModelID != "" && ref.ModelID != "" && report.ModelID != ref.ModelID:
		return "model"
	case report.VoiceID != "" && ref.VoiceID != "" && report.VoiceID != ref.VoiceID:
		return "voice"
	default:
		return ""
	}
}

// insertUsageEvents 落库 + 顺延预冻结（心跳），同一个事务（§14.4）。
func (s *Service) insertUsageEvents(ctx context.Context, events []store.BillingUsageEvent, reservation *store.BillingReservation, now time.Time) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	expiresAt := now.Add(time.Duration(s.cfg.ReserveSeconds)*time.Second + time.Minute)
	var inserted int
	err := s.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		count, err := s.store.InsertUsageEvents(events)
		if err != nil {
			return err
		}
		inserted = count
		// 上报顺带当心跳用：只有真正沉默的会话才会被回收任务释放冻结。
		return s.store.TouchReservation(tx, reservation.SessionID, expiresAt)
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

// resolveReservation 按 session_id 找预冻结。查不到时分两步，别直接丢（§14.2）：
//  1. 用 device_id 反查账户，能查到就补一条 degraded 的 reservation，事件照常落库
//     （这就是 authorize 超时 fail open 的那些会话）；
//  2. device_id 也反查不出账户，才返回 nil 走隔离队列。
func (s *Service) resolveReservation(ctx context.Context, sessionID, deviceID string, now time.Time) (*store.BillingReservation, error) {
	reservation, err := s.store.GetReservationBySession(sessionID)
	if err == nil {
		return reservation, nil
	}
	if !isNotFound(err) {
		return nil, err
	}
	if deviceID == "" || s.resolver == nil {
		return nil, nil
	}

	subjectType, subjectID, err := s.resolver.ResolveAccount(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("resolve degraded session %s: %w", sessionID, err)
	}
	if subjectID == "" {
		return nil, nil
	}
	account, err := s.ensureAccount(ctx, subjectType, subjectID)
	if err != nil {
		return nil, err
	}

	profile, err := s.resolver.ResolveSessionProfile(ctx, deviceID)
	if err != nil {
		logging.Warnf("billing: degraded session %s has no profile: %v", sessionID, err)
		profile = billing.SessionProfile{}
	}
	resourceJSON, err := toJSONMap(profile)
	if err != nil {
		return nil, err
	}
	snapshotJSON, err := toJSONMap(map[string]billing.PriceSnapshot{})
	if err != nil {
		return nil, err
	}

	degraded := &store.BillingReservation{
		ID:               shortID(),
		SessionID:        sessionID,
		AccountID:        account.ID,
		DeviceID:         deviceID,
		VoicebotID:       profile.VoicebotID,
		ReservedMicro:    0, // 降级会话不预扣、没有冻结
		Status:           billing.ReservationStatusDegraded,
		ExpiresAt:        now.Add(time.Duration(s.cfg.ReserveSeconds)*time.Second + time.Minute),
		ResourceSnapshot: resourceJSON,
		PriceSnapshot:    snapshotJSON,
	}
	if err := s.store.CreateReservation(degraded); err != nil {
		// 并发补行：回读那条。
		if existing, getErr := s.store.GetReservationBySession(sessionID); getErr == nil {
			return existing, nil
		}
		return nil, err
	}
	logging.Warnf("billing: degraded reservation created session=%s account=%s (authorize never completed)",
		sessionID, account.ID)
	return degraded, nil
}

func rejectAll(events []billing.UsageEventReport, reason string) []billing.RejectedEvent {
	out := make([]billing.RejectedEvent, 0, len(events))
	for _, ev := range events {
		out = append(out, billing.RejectedEvent{EventID: ev.EventID, Reason: reason})
	}
	return out
}
