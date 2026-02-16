package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManager_MergesDefaultsAndEnv(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "manager.json")
	data := `{
		"server": {"address": "127.0.0.1:9000", "health_path": "/health"},
		"database": {"dsn": "host=localhost user=u password=p dbname=db port=5432 sslmode=disable"}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("MANAGER_SERVER_ADDRESS", "127.0.0.1:9100")
	t.Setenv("MANAGER_DB_DSN", "host=127.0.0.1 user=postgres password=postgres dbname=override port=5432 sslmode=disable")
	t.Setenv("MANAGER_DB_MAX_OPEN_CONNS", "25")
	t.Setenv("MANAGER_ACCESS_KEY_CIPHER_SECRET", "cipher-secret")
	t.Setenv("MANAGER_AUTH_JWT_SECRET", "test-secret")
	t.Setenv("MANAGER_AUTH_ACCESS_TOKEN_TTL_SECONDS", "120")

	cfg, err := LoadManager(path)
	if err != nil {
		t.Fatalf("LoadManager() error = %v", err)
	}

	if cfg.Logging.Level != "debug" {
		t.Fatalf("expected LOG_LEVEL override, got %q", cfg.Logging.Level)
	}
	if cfg.Server.Address != "127.0.0.1:9100" {
		t.Fatalf("expected MANAGER_SERVER_ADDRESS override, got %q", cfg.Server.Address)
	}
	if cfg.Database.DSN != "host=127.0.0.1 user=postgres password=postgres dbname=override port=5432 sslmode=disable" {
		t.Fatalf("expected MANAGER_DB_DSN override, got %q", cfg.Database.DSN)
	}
	if cfg.Database.MaxOpenConns != 25 {
		t.Fatalf("expected MANAGER_DB_MAX_OPEN_CONNS override, got %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Security.AccessKeyCipherSecret != "cipher-secret" {
		t.Fatalf("expected MANAGER_ACCESS_KEY_CIPHER_SECRET override, got %q", cfg.Security.AccessKeyCipherSecret)
	}
	if cfg.Server.HealthPath != "/health" {
		t.Fatalf("expected health path from file, got %q", cfg.Server.HealthPath)
	}
	if cfg.Auth.JWTSecret != "test-secret" {
		t.Fatalf("expected MANAGER_AUTH_JWT_SECRET override, got %q", cfg.Auth.JWTSecret)
	}
	if cfg.Auth.AccessTokenTTLSeconds != 120 {
		t.Fatalf("expected MANAGER_AUTH_ACCESS_TOKEN_TTL_SECONDS override, got %d", cfg.Auth.AccessTokenTTLSeconds)
	}
}

func TestManagerConfigValidate_Address(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.Server.Address = "not-valid"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid address error")
	}
}

func TestManagerConfigValidate_DSNRequired(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.Database.DSN = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected missing dsn error")
	}
}

func TestLoadManager_DefaultFallback(t *testing.T) {
	cfg, err := LoadManager(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadManager() error = %v", err)
	}
	if cfg.Server.Address == "" {
		t.Fatalf("expected default server address")
	}
	if cfg.Database.DSN == "" {
		t.Fatalf("expected default database dsn")
	}
	if cfg.Security.AccessKeyCipherSecret == "" {
		t.Fatalf("expected default access key cipher secret")
	}
	if cfg.Auth.JWTSecret == "" {
		t.Fatalf("expected default auth jwt secret")
	}
}

func TestManagerConfigValidate_AuthSecretRequired(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.Auth.JWTSecret = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected missing auth.jwt_secret error")
	}
}

func TestManagerConfigValidate_AccessKeyCipherSecretRequired(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.Security.AccessKeyCipherSecret = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected missing security.access_key_cipher_secret error")
	}
}
