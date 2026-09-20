package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 会话级预冻结。三种归宿（§16.3）：open → settled（数据面老老实实调了 settle）、
// open → expired（沉默超过窗口，那边崩了或者断网了）、degraded → settled（authorize
// 超时放行的事后补行，没有冻结可释放）。每种转换都靠条件更新，保证只有一个赢家。

// 预冻结状态字面量，与 billing.ReservationStatus* 保持一致。
const (
	reservationStatusOpen    = "open"
	reservationStatusExpired = "expired"
)

// ReservationQuery 是预冻结列表的过滤条件。
type ReservationQuery struct {
	AccountID string
	DeviceID  string
	Status    string
	Limit     int
	Offset    int
}

// GetReservationBySession 按 session_id 查预冻结，未找到返回 ErrNotFound。
//
// 用量上报的账户归属就走这条路径反查：数据面带 session_id，控制面查 reservation
// 拿 account_id，绝不信任上报体里的账户（§14.2）。
func (s *BillingStore) GetReservationBySession(sessionID string) (*BillingReservation, error) {
	var r BillingReservation
	if err := s.db.Where("session_id = ?", sessionID).Take(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("billing store: get reservation by session: %w", err)
	}
	return &r, nil
}

// CreateReservation 新建一条预冻结。ID 为空时生成 uuid。
//
// session_id 唯一索引就是 authorize 的幂等键：断线重连再发一次 authorize 会撞冲突，
// 调用方应该改去查 GetReservationBySession 而不是重复冻结（§14.3）。
func (s *BillingStore) CreateReservation(r *BillingReservation) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if err := s.db.Create(r).Error; err != nil {
		return fmt.Errorf("billing store: create reservation: %w", err)
	}
	return nil
}

// UpdateReservation 局部更新一条预冻结（结算结果、失败原因等），未找到返回 ErrNotFound。
func (s *BillingStore) UpdateReservation(id string, updates map[string]any) error {
	res := s.db.Model(&BillingReservation{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("billing store: update reservation: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListReservations 按过滤条件分页返回预冻结，最新的在前。
func (s *BillingStore) ListReservations(q ReservationQuery) ([]BillingReservation, error) {
	var list []BillingReservation
	err := s.db.Model(&BillingReservation{}).Scopes(reservationScope(q)).
		Order("created_at DESC").Order("id DESC").
		Offset(q.Offset).Limit(normalizeLimit(q.Limit)).
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: list reservations: %w", err)
	}
	return list, nil
}

func reservationScope(q ReservationQuery) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if q.AccountID != "" {
			db = db.Where("account_id = ?", q.AccountID)
		}
		if q.DeviceID != "" {
			db = db.Where("device_id = ?", q.DeviceID)
		}
		if q.Status != "" {
			db = db.Where("status = ?", q.Status)
		}
		return db
	}
}

// TouchReservation 顺延会话的过期时间：上报顺带当心跳用（§14.4）。
//
// 只对 open 状态生效，而且 0 行受影响**不是**错误——批次晚于 settle 到达时，这条
// 预冻结已经不是 open 了，顺延本来就该什么都不做。少了这个心跳，活得久的会话会在
// 窗口到点后被回收任务当成死会话，同一笔钱就能再开一个会话花第二次。
func (s *BillingStore) TouchReservation(tx *gorm.DB, sessionID string, expiresAt time.Time) error {
	res := s.dbOr(tx).Model(&BillingReservation{}).
		Where("session_id = ? AND status = ?", sessionID, reservationStatusOpen).
		Update("expires_at", expiresAt)
	if res.Error != nil {
		return fmt.Errorf("billing store: touch reservation: %w", res.Error)
	}
	return nil
}

// ExpireReservations 把沉默超时的 open 预冻结标记为 expired 并返回它们
// （UPDATE ... WHERE status = 'open' AND expires_at < ? RETURNING *）。
//
// 回收只释放冻结，不取消已经上报的用量：用量是事实，事件照旧会被 worker 结算，
// 回收之后余额可能被扣成负数，那由透支策略去兜（§16.3）。
func (s *BillingStore) ExpireReservations(tx *gorm.DB, now time.Time) ([]BillingReservation, error) {
	var expired []BillingReservation
	err := s.dbOr(tx).Model(&expired).
		Clauses(clause.Returning{}).
		Where("status = ? AND expires_at < ?", reservationStatusOpen, now).
		Update("status", reservationStatusExpired).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: expire reservations: %w", err)
	}
	return expired, nil
}
