package xiaozhi

import (
	"testing"
)

func TestValidateManagerURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "http", url: "http://127.0.0.1:9090", want: true},
		{name: "https", url: "https://manager.example.com", want: true},
		{name: "missing", url: "", want: false},
		{name: "relative", url: "/internal", want: false},
		{name: "unsupported scheme", url: "ftp://manager.example.com", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManagerURL(tt.url)
			if (err == nil) != tt.want {
				t.Fatalf("ValidateManagerURL(%q) error = %v, want valid = %v", tt.url, err, tt.want)
			}
		})
	}
}

// TestAuthEnabledDefaultsOff：auth 段不写、enabled 缺省、显式 false 都是关闭；
// 只有显式 true 才开启（D6：默认关闭是兼容策略的一部分）。
func TestAuthEnabledDefaultsOff(t *testing.T) {
	tests := []struct {
		name string
		cfg  AuthConfig
		want bool
	}{
		{name: "missing", cfg: AuthConfig{}, want: false},
		{name: "explicit false", cfg: AuthConfig{Enabled: boolPtr(false)}, want: false},
		{name: "explicit true", cfg: AuthConfig{Enabled: boolPtr(true)}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.AuthEnabled(); got != tt.want {
				t.Fatalf("AuthEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestValidateAuthConfigRequiresToken：开启鉴权但没有内部 token 是启动期错误——
// 否则每条连接都会以 auth_unavailable 被拒，配错留到线上才发现。
func TestValidateAuthConfigRequiresToken(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		token   string
		wantErr bool
	}{
		{name: "disabled without token", enabled: false, token: "", wantErr: false},
		{name: "enabled without token", enabled: true, token: "", wantErr: true},
		{name: "enabled with token", enabled: true, token: "secret", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Auth.Enabled = boolPtr(tt.enabled)
			cfg.Manager.Token = tt.token
			if err := ValidateAuthConfig(cfg); (err != nil) != tt.wantErr {
				t.Fatalf("ValidateAuthConfig() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }
