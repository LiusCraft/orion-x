package apikey

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/store"
)

// fakeStore 是 *store.APIKeyStore 的内联替身（仓库约定：不引 mock 生成器）。
type fakeStore struct {
	rows map[string]*store.APIKey

	failCreates int // > 0 时前 N 次 Create 返回 createErr
	createErr   error
	createCalls int
	getErr      error
	listErr     error
	revokeErr   error

	usage []usageCall
}

type usageCall struct {
	ID       string
	Calls    int64
	LastUsed time.Time
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: make(map[string]*store.APIKey)}
}

func (s *fakeStore) Create(_ context.Context, k *store.APIKey) error {
	s.createCalls++
	if s.failCreates > 0 {
		s.failCreates--
		if s.createErr == nil {
			return store.ErrDuplicate
		}
		return s.createErr
	}
	if _, exists := s.rows[k.ID]; exists {
		return store.ErrDuplicate
	}
	row := *k
	row.CreatedAt = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	s.rows[k.ID] = &row
	return nil
}

func (s *fakeStore) GetByLookup(_ context.Context, lookup string) (*store.APIKey, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	for _, row := range s.rows {
		if row.Lookup == lookup {
			cp := *row
			return &cp, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *fakeStore) ListByUser(_ context.Context, userID string) ([]store.APIKey, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	var out []store.APIKey
	for _, row := range s.rows {
		if row.UserID == userID {
			out = append(out, *row)
		}
	}
	return out, nil
}

func (s *fakeStore) CountByUser(_ context.Context, userID string) (int64, error) {
	var n int64
	for _, row := range s.rows {
		if row.UserID == userID {
			n++
		}
	}
	return n, nil
}

func (s *fakeStore) Revoke(_ context.Context, id, userID string, at time.Time) error {
	if s.revokeErr != nil {
		return s.revokeErr
	}
	row, ok := s.rows[id]
	if !ok || row.UserID != userID {
		return store.ErrNotFound
	}
	if row.RevokedAt == nil {
		row.RevokedAt = &at
	}
	return nil
}

func (s *fakeStore) AddUsage(_ context.Context, id string, calls int64, lastUsed time.Time) error {
	s.usage = append(s.usage, usageCall{ID: id, Calls: calls, LastUsed: lastUsed})
	return nil
}

// fakeLimiter 只记录调用，判定结果固定。
type fakeLimiter struct {
	allowed   bool
	forgotten []string
}

func (l *fakeLimiter) Allow(string) LimitDecision { return LimitDecision{Allowed: l.allowed, Limit: 1} }
func (l *fakeLimiter) Forget(keyID string)        { l.forgotten = append(l.forgotten, keyID) }

var fixedNow = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

func newTestService(t *testing.T, st Store) (*Service, *fakeLimiter) {
	t.Helper()
	limiter := &fakeLimiter{allowed: true}
	svc := New(st, Config{Limiter: limiter, Now: func() time.Time { return fixedNow }})
	return svc, limiter
}

func TestServiceCreateValidates(t *testing.T) {
	longName := strings.Repeat("名", 65)
	past := fixedNow.Add(-time.Minute)
	now := fixedNow
	future := fixedNow.Add(time.Hour)

	cases := []struct {
		name    string
		keyName string
		scopes  []string
		expires *time.Time
		wantErr error
	}{
		{name: "blank name", keyName: "   ", scopes: []string{ScopeAgentRead}, wantErr: ErrNameRequired},
		{name: "name too long", keyName: longName, scopes: []string{ScopeAgentRead}, wantErr: ErrNameTooLong},
		{name: "no scopes", keyName: "ci", wantErr: ErrNoScopes},
		{name: "unknown scope", keyName: "ci", scopes: []string{"nope"}, wantErr: ErrUnknownScope},
		{name: "expired already", keyName: "ci", scopes: []string{ScopeAgentRead}, expires: &past, wantErr: ErrExpiresAtPast},
		{name: "expires now", keyName: "ci", scopes: []string{ScopeAgentRead}, expires: &now, wantErr: ErrExpiresAtPast},
		{name: "valid with expiry", keyName: "ci", scopes: []string{ScopeAgentRead}, expires: &future},
		{name: "valid without expiry", keyName: "ci", scopes: []string{ScopeAgentRead}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newFakeStore()
			svc, _ := newTestService(t, st)
			_, _, err := svc.Create(context.Background(), "user-1", tc.keyName, tc.scopes, tc.expires)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Create() error = %v, want %v", err, tc.wantErr)
				}
				if st.createCalls != 0 {
					t.Fatalf("validation failure must not reach the store (%d calls)", st.createCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		})
	}
}

func TestServiceCreateStoresOnlyDigest(t *testing.T) {
	st := newFakeStore()
	svc, _ := newTestService(t, st)

	plaintext, record, err := svc.Create(context.Background(), "user-1",
		" 生产环境 ", []string{ScopeDataRead, ScopeAgentRead, ScopeDataRead}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	lookup, _, ok := Parse(plaintext)
	if !ok {
		t.Fatalf("Create() returned an unparseable key %q", plaintext)
	}
	if record.Name != "生产环境" {
		t.Errorf("record.Name = %q, want the trimmed name", record.Name)
	}
	if want := strings.Join([]string{ScopeAgentRead, ScopeDataRead}, ","); strings.Join(record.Scopes, ",") != want {
		t.Errorf("record.Scopes = %v, want normalized %q", record.Scopes, want)
	}
	if record.Lookup != lookup {
		t.Errorf("record.Lookup = %q, want %q", record.Lookup, lookup)
	}
	if record.Hash != Digest(plaintext) {
		t.Error("record.Hash is not sha256 of the plaintext")
	}

	row := st.rows[record.ID]
	if row == nil {
		t.Fatal("record was not persisted")
	}
	if row.Hash == plaintext || row.Lookup == plaintext || strings.Contains(fmt.Sprintf("%+v", row), plaintext) {
		t.Fatal("the plaintext ended up in the persisted row")
	}
	if row.UserID != "user-1" || row.Creator != "user-1" {
		t.Fatalf("row owner = (%q, %q), want user-1 twice", row.UserID, row.Creator)
	}
}

func TestServiceCreateKeyLimit(t *testing.T) {
	st := newFakeStore()
	svc := New(st, Config{MaxKeysPerAccount: 1, Now: func() time.Time { return fixedNow }})

	if _, _, err := svc.Create(context.Background(), "user-1", "a", []string{ScopeAgentRead}, nil); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	if _, _, err := svc.Create(context.Background(), "user-1", "b", []string{ScopeAgentRead}, nil); !errors.Is(err, ErrKeyLimitReached) {
		t.Fatalf("second Create() error = %v, want ErrKeyLimitReached", err)
	}
	// 上限按账号算，不按全平台算。
	if _, _, err := svc.Create(context.Background(), "user-2", "c", []string{ScopeAgentRead}, nil); err != nil {
		t.Fatalf("Create() for another account error = %v", err)
	}
}

func TestServiceCreateRetriesOnCollision(t *testing.T) {
	st := newFakeStore()
	st.failCreates = 2 // 前两次撞唯一索引
	svc, _ := newTestService(t, st)

	if _, _, err := svc.Create(context.Background(), "user-1", "ci", []string{ScopeAgentRead}, nil); err != nil {
		t.Fatalf("Create() error = %v, want success after retries", err)
	}
	if st.createCalls != 3 {
		t.Fatalf("createCalls = %d, want 3", st.createCalls)
	}

	st.failCreates = 100 // 一直撞：放弃，把错误交给调用方
	if _, _, err := svc.Create(context.Background(), "user-1", "ci", []string{ScopeAgentRead}, nil); !errors.Is(err, store.ErrDuplicate) {
		t.Fatalf("Create() error = %v, want store.ErrDuplicate", err)
	}
	if want := 3 + createAttempts; st.createCalls != want {
		t.Fatalf("createCalls = %d, want %d", st.createCalls, want)
	}
}

func TestServiceAuthenticate(t *testing.T) {
	st := newFakeStore()
	svc, _ := newTestService(t, st)

	plaintext, record, err := svc.Create(context.Background(), "user-1", "ci", []string{ScopeAgentRead, ScopeDataRead}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	t.Run("valid", func(t *testing.T) {
		id, err := svc.Authenticate(context.Background(), plaintext)
		if err != nil {
			t.Fatalf("Authenticate() error = %v", err)
		}
		if id.KeyID != record.ID || id.UserID != "user-1" {
			t.Fatalf("Identity = %+v, want key %s of user-1", id, record.ID)
		}
		if strings.Join(id.Scopes, ",") != (ScopeAgentRead + "," + ScopeDataRead) {
			t.Fatalf("Identity.Scopes = %v", id.Scopes)
		}
		// Identity 视为不可变：调用方改它不该动到库里那行。
		id.Scopes[0] = "mutated"
		if st.rows[record.ID].Scopes[0] == "mutated" {
			t.Fatal("Identity aliases the stored row's scopes")
		}
	})

	t.Run("malformed is rejected without touching the store", func(t *testing.T) {
		if _, err := svc.Authenticate(context.Background(), "ox_sk_not-a-key"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Authenticate(malformed) error = %v, want ErrInvalid", err)
		}
	})

	t.Run("unknown lookup", func(t *testing.T) {
		other, _, err := Generate("user-2", "ci", []string{ScopeAgentRead}, nil)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if _, err := svc.Authenticate(context.Background(), other); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Authenticate(unknown) error = %v, want ErrInvalid", err)
		}
	})

	t.Run("same lookup, different secret", func(t *testing.T) {
		// 形状合法、lookup 存在、但秘密段不是当初那把：必须走常量时间比对后被拒。
		forgedSecret := strings.Repeat("z", secretLength)
		forged := Prefix + record.Lookup + "_" + forgedSecret + "_" + crcSuffix(record.Lookup, forgedSecret)
		if _, _, ok := Parse(forged); !ok {
			t.Fatal("fixture is not shape-valid")
		}
		if _, err := svc.Authenticate(context.Background(), forged); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Authenticate(forged) error = %v, want ErrInvalid", err)
		}
	})

	t.Run("revoked", func(t *testing.T) {
		revokedAt := fixedNow
		st.rows[record.ID].RevokedAt = &revokedAt
		if _, err := svc.Authenticate(context.Background(), plaintext); !errors.Is(err, ErrRevoked) {
			t.Fatalf("Authenticate(revoked) error = %v, want ErrRevoked", err)
		}
		st.rows[record.ID].RevokedAt = nil
	})

	t.Run("expired at the boundary", func(t *testing.T) {
		expiresAt := fixedNow
		st.rows[record.ID].ExpiresAt = &expiresAt
		if _, err := svc.Authenticate(context.Background(), plaintext); !errors.Is(err, ErrExpired) {
			t.Fatalf("Authenticate(expired) error = %v, want ErrExpired", err)
		}
		st.rows[record.ID].ExpiresAt = nil
	})

	t.Run("store failure is not a 401", func(t *testing.T) {
		st.getErr = errors.New("db down")
		defer func() { st.getErr = nil }()
		_, err := svc.Authenticate(context.Background(), plaintext)
		if err == nil || errors.Is(err, ErrInvalid) {
			t.Fatalf("Authenticate() error = %v, want a wrapped store error", err)
		}
	})
}

func TestServiceRevoke(t *testing.T) {
	st := newFakeStore()
	svc, limiter := newTestService(t, st)

	plaintext, record, err := svc.Create(context.Background(), "user-1", "ci", []string{ScopeAgentRead}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := svc.Revoke(context.Background(), "user-2", record.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Revoke(other user) error = %v, want ErrNotFound", err)
	}
	if err := svc.Revoke(context.Background(), "user-1", record.ID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	// 幂等：第二次也成功（FR-3 锁死"可重复调用"）。
	if err := svc.Revoke(context.Background(), "user-1", record.ID); err != nil {
		t.Fatalf("second Revoke() error = %v, want idempotent success", err)
	}
	// 撤销后第一个请求即失效：没有缓存可以变旧（G3）。
	if _, err := svc.Authenticate(context.Background(), plaintext); !errors.Is(err, ErrRevoked) {
		t.Fatalf("Authenticate() after revoke error = %v, want ErrRevoked", err)
	}
	// 内存里的桶与待刷计数跟着一起清掉（§5.2 路径 B）。
	if strings.Join(limiter.forgotten, ",") != record.ID+","+record.ID {
		t.Fatalf("limiter.forgotten = %v, want the key twice", limiter.forgotten)
	}
	if len(svc.counter.pending) != 0 {
		t.Fatalf("counter still holds %d entries for a revoked key", len(svc.counter.pending))
	}
}

func TestServiceListFiltersByUser(t *testing.T) {
	st := newFakeStore()
	svc, _ := newTestService(t, st)

	if _, _, err := svc.Create(context.Background(), "user-1", "mine", []string{ScopeAgentRead}, nil); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, _, err := svc.Create(context.Background(), "user-2", "theirs", []string{ScopeAgentRead}, nil); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	list, err := svc.List(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].Name != "mine" {
		t.Fatalf("List(user-1) = %+v, want only its own key", list)
	}
}

func TestServiceUsageAggregationAndFlush(t *testing.T) {
	st := newFakeStore()
	svc := New(st, Config{CounterFlush: time.Millisecond, Now: func() time.Time { return fixedNow }})

	_, record, err := svc.Create(context.Background(), "user-1", "ci", []string{ScopeAgentRead}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc.RecordUsage(record.ID)
	svc.RecordUsage(record.ID)
	svc.RecordUsage(record.ID)
	if len(st.usage) != 0 {
		t.Fatal("counting must stay in memory until the flush window closes")
	}

	svc.Flush(context.Background())
	if len(st.usage) != 1 {
		t.Fatalf("flush wrote %d rows, want 1", len(st.usage))
	}
	if st.usage[0].ID != record.ID || st.usage[0].Calls != 3 {
		t.Fatalf("usage = %+v, want 3 calls for %s", st.usage[0], record.ID)
	}
	if !st.usage[0].LastUsed.Equal(fixedNow) {
		t.Fatalf("LastUsed = %v, want %v", st.usage[0].LastUsed, fixedNow)
	}

	// 刷完就清空：没有新的调用就没有新的写。
	svc.Flush(context.Background())
	if len(st.usage) != 1 {
		t.Fatalf("second flush wrote again: %+v", st.usage)
	}
}
