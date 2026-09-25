package apikey

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// Service 对外暴露的错误，HTTP 层按它们映射状态码（§5.3）。
var (
	// ErrRevoked / ErrExpired 刻意与 ErrInvalid 分开：key 的主人就是我们的用户，
	// "被撤了"还是"过期了"直接决定他下一步该干什么（§6 N7）。
	ErrRevoked = errors.New("apikey: key revoked")
	ErrExpired = errors.New("apikey: key expired")
)

// Store 是 Service 需要的持久化能力，由 internal/store 的 APIKeyStore 实现。
// 抽成接口不是为了换数据库，而是为了让单测能内联假实现（仓库约定：不引 mock 生成器）。
type Store interface {
	Create(ctx context.Context, k *store.APIKey) error
	GetByLookup(ctx context.Context, lookup string) (*store.APIKey, error)
	ListByUser(ctx context.Context, userID string) ([]store.APIKey, error)
	CountByUser(ctx context.Context, userID string) (int64, error)
	Revoke(ctx context.Context, id, userID string, at time.Time) error
	AddUsage(ctx context.Context, id string, calls int64, lastUsed time.Time) error
}

// Identity 是校验成功后交给上层的全部事实：返回后视为不可变，随请求上下文传递，
// 不跨请求缓存（§5.3）。
type Identity struct {
	KeyID  string
	UserID string
	Scopes []string
}

// Config 是运行参数。零值可用，两个例外：RateLimitRPS / RateLimitBurst <= 0 表示
// 不限流（默认值在 DefaultConfig，配置层兜底——配置文件里写 0 不会把限流关掉）；
// MaxKeysPerAccount <= 0 表示不限制账号的 key 数。
type Config struct {
	RateLimitRPS   float64
	RateLimitBurst int
	// CounterFlush 是用量计数的刷库窗口（§3.2：默认 30s）。
	CounterFlush      time.Duration
	MaxKeysPerAccount int
	// Limiter nil = 按 RateLimitRPS/Burst 建进程内令牌桶；Limiter 与 Now 便于测试注入。
	Limiter Limiter
	Now     func() time.Time
}

func DefaultConfig() Config {
	return Config{
		RateLimitRPS:      20,
		RateLimitBurst:    40,
		CounterFlush:      30 * time.Second,
		MaxKeysPerAccount: 100,
		Now:               time.Now,
	}
}

// Service 是 API Key 的全部能力：自助管理与校验热路径。
type Service struct {
	store   Store
	limiter Limiter
	counter *usageCounter
	cfg     Config
}

func New(st Store, cfg Config) *Service {
	cfg = normalizeConfig(cfg)
	limiter := cfg.Limiter
	if limiter == nil {
		limiter = NewTokenBucketLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst, cfg.Now)
	}
	return &Service{store: st, limiter: limiter, counter: newUsageCounter(), cfg: cfg}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.RateLimitRPS < 0 {
		cfg.RateLimitRPS = def.RateLimitRPS
	}
	if cfg.RateLimitBurst < 0 {
		cfg.RateLimitBurst = def.RateLimitBurst
	}
	if cfg.CounterFlush <= 0 {
		cfg.CounterFlush = def.CounterFlush
	}
	if cfg.Now == nil {
		cfg.Now = def.Now
	}
	return cfg
}

// createAttempts 是创建时撞唯一索引的重试次数：lookup/hash 的取值空间是 2^95 /
// 2^256 量级，撞车可忽略，唯一索引只是兜底。
const createAttempts = 3

// Create 生成并落库一把新 key，返回明文（唯一一次）与记录。不接受"给谁创建"这个
// 参数：全都按 userID 过滤。
func (s *Service) Create(ctx context.Context, userID, name string, scopes []string, expiresAt *time.Time) (string, *store.APIKey, error) {
	name, err := normalizeName(name)
	if err != nil {
		return "", nil, err
	}
	normalized, err := NormalizeScopes(scopes)
	if err != nil {
		return "", nil, err
	}
	if expiresAt != nil && !expiresAt.After(s.cfg.Now()) {
		return "", nil, ErrExpiresAtPast
	}

	if s.cfg.MaxKeysPerAccount > 0 {
		n, err := s.store.CountByUser(ctx, userID)
		if err != nil {
			return "", nil, err
		}
		if n >= int64(s.cfg.MaxKeysPerAccount) {
			return "", nil, ErrKeyLimitReached
		}
	}

	for attempt := 0; attempt < createAttempts; attempt++ {
		plaintext, record, err := Generate(userID, name, normalized, expiresAt)
		if err != nil {
			return "", nil, err
		}
		if err := s.store.Create(ctx, &record); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				continue
			}
			return "", nil, err
		}
		return plaintext, &record, nil
	}
	return "", nil, fmt.Errorf("apikey: create: %w", store.ErrDuplicate)
}

// List 列出某个账号自己的 key（含已撤销/已过期的：列表要能回答"这把 key 还在不在"）。
func (s *Service) List(ctx context.Context, userID string) ([]store.APIKey, error) {
	return s.store.ListByUser(ctx, userID)
}

// Revoke 撤销一把 key：只能撤自己的，软删，重复调用幂等（§5.4）。撤销后立刻失效：
// 没有缓存可以变旧（§1 D6）。
func (s *Service) Revoke(ctx context.Context, userID, keyID string) error {
	if err := s.store.Revoke(ctx, keyID, userID, s.cfg.Now()); err != nil {
		return err
	}
	// 桶与待刷计数跟着一起清掉（§5.2 路径 B）：撤销后的 key 不会再被判定。
	s.limiter.Forget(keyID)
	s.counter.Forget(keyID)
	return nil
}

// Authenticate 是热路径：形状 → 查库 → 常量时间比对 → 撤销/过期判定。
//
// 唯一判定有效性的地方，别的路径不许自行判断"这把 key 还能不能用"（§5.4 不变量）。
// 不缓存（§1 D6）：撤销延迟因此为 0，多副本时也省掉跨副本失效。
func (s *Service) Authenticate(ctx context.Context, raw string) (Identity, error) {
	lookup, _, ok := Parse(raw)
	if !ok {
		return Identity{}, ErrInvalid
	}

	row, err := s.store.GetByLookup(ctx, lookup)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return Identity{}, ErrInvalid
		}
		return Identity{}, fmt.Errorf("apikey: authenticate: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(row.Hash), []byte(Digest(raw))) != 1 {
		return Identity{}, ErrInvalid
	}
	if row.RevokedAt != nil {
		return Identity{}, ErrRevoked
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(s.cfg.Now()) {
		return Identity{}, ErrExpired
	}

	return Identity{
		KeyID:  row.ID,
		UserID: row.UserID,
		Scopes: append([]string(nil), row.Scopes...),
	}, nil
}

// Allow 对某个 key 做一次限流判定。
func (s *Service) Allow(keyID string) LimitDecision {
	return s.limiter.Allow(keyID)
}

// RecordUsage 记一次成功放行的调用（内存聚合，窗口到了才刷库）。
func (s *Service) RecordUsage(keyID string) {
	s.counter.Touch(keyID, s.cfg.Now())
}

// RunCounter 是进程级单个刷库 goroutine 的循环，生命周期跟随 manager 的 root context。
// 收到取消后尽量刷最后一次，最多丢一个窗口（§3.2）。
func (s *Service) RunCounter(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.CounterFlush)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), s.cfg.CounterFlush)
			s.Flush(flushCtx)
			cancel()
			return
		case <-ticker.C:
			flushCtx, cancel := context.WithTimeout(ctx, s.cfg.CounterFlush)
			s.Flush(flushCtx)
			cancel()
		}
	}
}

// Flush 把当前窗口的计数写回库（增量 + 取最大，多副本安全）。失败只告警不重试
// （§6 R5）：计数本来就是非精确口径，真值得重试就该换成正经的指标管道。
func (s *Service) Flush(ctx context.Context) {
	for keyID, d := range s.counter.Drain() {
		if err := s.store.AddUsage(ctx, keyID, d.Calls, d.LastUsed); err != nil {
			logging.Warnf("apikey: flush usage for %s: %v", keyID, err)
		}
	}
}

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrNameRequired
	}
	if len([]rune(name)) > 64 {
		return "", ErrNameTooLong
	}
	return name, nil
}
