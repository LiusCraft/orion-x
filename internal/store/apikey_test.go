package store

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/liuscraft/orion-x/internal/apikey"
)

// newDryRunAPIKeyStore 造一个只生成 SQL、不连库的 APIKeyStore。
// SkipDefaultTransaction 是必须的：GORM 在 DryRun 下仍然会为写操作 Begin()，那一步会真去连库。
func newDryRunAPIKeyStore(t *testing.T) (*APIKeyStore, *sqlRecorder) {
	t.Helper()

	rec := &sqlRecorder{Interface: logger.Discard}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DriverName: "pgx",
		DSN:        "postgres://apikey:apikey@127.0.0.1:5432/apikey_test?sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 rec,
	})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	boxKey, err := apikey.DeriveSecretKey([]byte("test-master-secret"))
	if err != nil {
		t.Fatalf("derive secret box key: %v", err)
	}
	return NewAPIKeyStore(db, boxKey), rec
}

// TestAPIKeyStoreCreateKeepsOnlyADigest 盯住“明文不落库”这条底线：
// 插入语句里只能出现摘要、密文、前缀、尾四位，不能出现明文本身。
func TestAPIKeyStoreCreateKeepsOnlyADigest(t *testing.T) {
	s, rec := newDryRunAPIKeyStore(t)

	key, plain, err := s.Create("生产环境", "user-1", []apikey.Scope{apikey.ScopeAgent, apikey.ScopeData}, "user-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if key.KeyHash != apikey.Hash(plain) {
		t.Errorf("KeyHash = %q, want the SHA-256 digest of the plaintext", key.KeyHash)
	}
	// 密文必须能解回同一个明文，否则“验证密码后复制”会吐出别的东西。
	if opened, err := apikey.Open(s.boxKey, key.KeySecret); err != nil || opened != plain {
		t.Errorf("Open(KeySecret) = (%q, %v), want the plaintext", opened, err)
	}
	wantPrefix, wantLast4, err := apikey.Display(plain)
	if err != nil {
		t.Fatalf("Display() error = %v", err)
	}
	if key.KeyPrefix != wantPrefix || key.KeyLast4 != wantLast4 {
		t.Errorf("stored display parts = (%q, %q), want (%q, %q)", key.KeyPrefix, key.KeyLast4, wantPrefix, wantLast4)
	}
	if len(key.Scopes) != 2 || key.Scopes[0] != string(apikey.ScopeAgent) || key.Scopes[1] != string(apikey.ScopeData) {
		t.Errorf("stored scopes = %v", key.Scopes)
	}
	if key.OwnerID != "user-1" || key.ID == "" {
		t.Errorf("stored key = %+v", key)
	}

	sql := rec.all()
	if !strings.Contains(sql, `INSERT INTO "api_keys"`) {
		t.Fatalf("create did not insert into api_keys:\n%s", sql)
	}
	secret := strings.TrimPrefix(plain, apikey.Prefix)
	if strings.Contains(sql, secret) {
		t.Errorf("plaintext secret leaked into the insert statement:\n%s", sql)
	}
}

// TestAPIKeyStoreRevealSQL 只验证语句形状：按 id + 属主取行，不按摘要查。
func TestAPIKeyStoreRevealSQL(t *testing.T) {
	s, rec := newDryRunAPIKeyStore(t)

	if _, err := s.Reveal("user-1", "key-1"); err == nil {
		// DryRun 造不出行，这里只关心语句形状。
		t.Log("reveal returned a row under DryRun; assertion below still holds")
	}
	sql := rec.all()
	for _, want := range []string{`FROM "api_keys"`, `id = 'key-1'`, `owner_id = 'user-1'`} {
		if !strings.Contains(sql, want) {
			t.Errorf("reveal statement missing %q:\n%s", want, sql)
		}
	}
}

// TestAPIKeyStoreWithoutABoxKey 确认没配置封装密钥时签发会明确报错，
// 而不是默默存一个空的 key_secret（那样等到复制时才发现就晚了）。
func TestAPIKeyStoreWithoutABoxKey(t *testing.T) {
	s, _ := newDryRunAPIKeyStore(t)
	s.boxKey = nil
	if _, _, err := s.Create("生产环境", "user-1", []apikey.Scope{apikey.ScopeAll}, "user-1"); !errors.Is(err, apikey.ErrSecretUnavailable) {
		t.Fatalf("Create() error = %v, want ErrSecretUnavailable", err)
	}
}

// TestAPIKeyStoreListAndDeleteSQL 只验证语句形状：列表归属过滤 + 倒序、删除带属主条件。
func TestAPIKeyStoreListAndDeleteSQL(t *testing.T) {
	s, rec := newDryRunAPIKeyStore(t)

	if _, err := s.ListByOwner("user-1"); err != nil {
		t.Fatalf("ListByOwner() error = %v", err)
	}
	sql := rec.all()
	for _, want := range []string{`FROM "api_keys"`, `owner_id = 'user-1'`, `ORDER BY created_at DESC`} {
		if !strings.Contains(sql, want) {
			t.Errorf("list statement missing %q:\n%s", want, sql)
		}
	}

	if err := s.Delete("user-1", "key-1"); !errors.Is(err, ErrNotFound) {
		// DryRun 下 RowsAffected 恒为 0，删除只能走到 ErrNotFound；关键是 WHERE 里带了属主。
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
	sql = rec.all()
	if !strings.Contains(sql, `DELETE FROM "api_keys"`) || !strings.Contains(sql, `owner_id = 'user-1'`) {
		t.Errorf("delete statement must filter by owner:\n%s", sql)
	}
}

func TestAPIKeyStoreAuthenticate(t *testing.T) {
	t.Run("foreign formats never reach the database", func(t *testing.T) {
		s, rec := newDryRunAPIKeyStore(t)
		if _, err := s.Authenticate("eyJhbGciOiJIUzI1NiJ9.token"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Authenticate(jwt) error = %v, want ErrNotFound", err)
		}
		if sql := rec.all(); sql != "" {
			t.Errorf("authenticating a non-key hit the database:\n%s", sql)
		}
	})

	t.Run("unknown key is looked up by digest only", func(t *testing.T) {
		s, rec := newDryRunAPIKeyStore(t)
		plain := apikey.Prefix + strings.Repeat("ab", 24)
		// DryRun 下 GORM 不会真的查库，也造不出 ErrRecordNotFound，这里只盯语句形状；
		// “查不到就是 ErrNotFound” 在 handler / middleware 的测试里用假 store 覆盖。
		if _, err := s.Authenticate(plain); err != nil {
			t.Fatalf("Authenticate() error = %v", err)
		}
		sql := rec.all()
		if !strings.Contains(sql, `WHERE key_hash = '`+apikey.Hash(plain)+`'`) {
			t.Errorf("lookup must be by digest:\n%s", sql)
		}
		if strings.Contains(sql, strings.TrimPrefix(plain, apikey.Prefix)) {
			t.Errorf("plaintext leaked into the lookup:\n%s", sql)
		}
	})

	t.Run("touch bumps usage without failing the request", func(t *testing.T) {
		s, rec := newDryRunAPIKeyStore(t)
		key := &APIKey{ID: "key-1"}
		s.touch(key)
		sql := rec.all()
		for _, want := range []string{`UPDATE "api_keys"`, `"call_count"=call_count + 1`, `WHERE id = 'key-1'`} {
			if !strings.Contains(sql, want) {
				t.Errorf("touch statement missing %q:\n%s", want, sql)
			}
		}
		if key.CallCount != 1 {
			t.Errorf("CallCount = %d, want 1", key.CallCount)
		}
		if key.LastUsedAt == nil || time.Since(*key.LastUsedAt) > time.Minute {
			t.Errorf("LastUsedAt = %v, want a fresh timestamp", key.LastUsedAt)
		}
	})
}
