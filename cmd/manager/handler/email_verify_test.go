package handler

import (
	"strings"
	"testing"

	"github.com/liuscraft/orion-x/internal/mailer"
)

func TestVerifyLink(t *testing.T) {
	h, err := NewAuthHandler(nil, nil, nil, AuthConfig{VerifyURLBase: "https://console.example.com/"}, nil, nil)
	if err != nil {
		t.Fatalf("NewAuthHandler() error = %v", err)
	}
	if got, want := h.verifyLink("abc123"), "https://console.example.com/login?verify_token=abc123"; got != want {
		t.Errorf("verifyLink() = %q, want %q", got, want)
	}
}

func TestNewAuthHandlerConfig(t *testing.T) {
	mail := &mailer.Mailer{}
	verify := NewVerifyStore(nil)
	cases := []struct {
		name    string
		cfg     AuthConfig
		mail    *mailer.Mailer
		verify  *VerifyStore
		wantErr string
	}{
		{name: "disabled ignores missing dependencies", cfg: AuthConfig{}},
		{
			name:    "enabled requires mailer",
			cfg:     AuthConfig{EmailVerify: true, VerifyURLBase: "https://console.example.com"},
			verify:  verify,
			wantErr: "smtp.host",
		},
		{
			name:    "enabled requires redis",
			cfg:     AuthConfig{EmailVerify: true, VerifyURLBase: "https://console.example.com"},
			mail:    mail,
			wantErr: "redis.addr",
		},
		{
			name:    "enabled requires verify url",
			cfg:     AuthConfig{EmailVerify: true},
			mail:    mail,
			verify:  verify,
			wantErr: "verify_url_base",
		},
		{
			name:    "relative verify url is rejected",
			cfg:     AuthConfig{EmailVerify: true, VerifyURLBase: "console.example.com/login"},
			mail:    mail,
			verify:  verify,
			wantErr: "verify_url_base",
		},
		{
			name:   "enabled with complete config passes",
			cfg:    AuthConfig{EmailVerify: true, VerifyURLBase: "https://console.example.com"},
			mail:   mail,
			verify: verify,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, err := NewAuthHandler(nil, nil, nil, tc.cfg, tc.mail, tc.verify)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("NewAuthHandler() error = %v, want nil", err)
				}
				if h == nil {
					t.Fatal("NewAuthHandler() handler = nil, want non-nil")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("NewAuthHandler() error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}
