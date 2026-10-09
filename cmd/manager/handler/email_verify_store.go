package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrTokenNotFound 表示验证令牌无效、过期或已被消费；对外不区分，防枚举。
var ErrTokenNotFound = errors.New("handler: email verify token not found")

// Redis key 前缀：自己定义的标识值统一用 `:` 分段（AGENTS.md）。
const verifyKeyPrefix = "orionx:auth:verify:"

// VerifyStore 用 Redis 保存邮箱验证的短期状态：一次性令牌与重发冷却。
// 需要 Redis 6.2+（消费令牌用 GETDEL 原子完成）。
type VerifyStore struct {
	rdb *redis.Client
}

// NewVerifyStore 基于已连接的 Redis 客户端创建存储。
func NewVerifyStore(rdb *redis.Client) *VerifyStore { return &VerifyStore{rdb: rdb} }

// Ping 探活，启动期用。
func (s *VerifyStore) Ping(ctx context.Context) error {
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

// IssueToken 签发新令牌并让该用户旧令牌立即失效；返回的明文只进邮件。
func (s *VerifyStore) IssueToken(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("handler: generate verify token: %w", err)
	}
	token := hex.EncodeToString(raw)
	hash := hashToken(token)

	// 先失效旧令牌（user 索引仍指向它时），再写 token -> userID 与 userID -> hash。
	oldHash, err := s.rdb.Get(ctx, verifyUserKey(userID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return "", fmt.Errorf("handler: read verify index: %w", err)
	}
	pipe := s.rdb.TxPipeline()
	if oldHash != "" {
		pipe.Del(ctx, verifyTokenKey(oldHash))
	}
	pipe.Set(ctx, verifyTokenKey(hash), userID, ttl)
	pipe.Set(ctx, verifyUserKey(userID), hash, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("handler: issue verify token: %w", err)
	}
	return token, nil
}

// ConsumeToken 原子消费令牌（GETDEL），返回归属用户；无效/过期/已用一律
// ErrTokenNotFound。索引清理是 best-effort：只有仍指向本令牌时才删。
func (s *VerifyStore) ConsumeToken(ctx context.Context, rawToken string) (string, error) {
	hash := hashToken(rawToken)
	userID, err := s.rdb.GetDel(ctx, verifyTokenKey(hash)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrTokenNotFound
	}
	if err != nil {
		return "", fmt.Errorf("handler: consume verify token: %w", err)
	}

	if cur, err := s.rdb.Get(ctx, verifyUserKey(userID)).Result(); err == nil && cur == hash {
		_ = s.rdb.Del(ctx, verifyUserKey(userID)).Err()
	}
	return userID, nil
}

// AllowResend 用 SET NX EX 实现重发冷却：cooldown 内同一用户只放行一次。
func (s *VerifyStore) AllowResend(ctx context.Context, userID string, cooldown time.Duration) (bool, error) {
	ok, err := s.rdb.SetNX(ctx, verifyCooldownKey(userID), "1", cooldown).Result()
	if err != nil {
		return false, fmt.Errorf("handler: resend cooldown: %w", err)
	}
	return ok, nil
}

func verifyTokenKey(hash string) string      { return verifyKeyPrefix + "token:" + hash }
func verifyUserKey(userID string) string     { return verifyKeyPrefix + "user:" + userID }
func verifyCooldownKey(userID string) string { return verifyKeyPrefix + "cooldown:" + userID }

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
