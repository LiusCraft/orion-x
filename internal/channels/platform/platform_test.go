package platform

import (
	"strings"
	"testing"
)

func TestGetKnownPlatforms(t *testing.T) {
	for _, name := range []string{Telegram, WeCom} {
		desc, ok := Get(name)
		if !ok {
			t.Fatalf("Get(%q) = not found, want registered", name)
		}
		if desc.DisplayName == "" || len(desc.Fields) == 0 {
			t.Errorf("descriptor %q is incomplete: %+v", name, desc)
		}
	}
	if _, ok := Get("no-such-platform"); ok {
		t.Error("Get(no-such-platform) = found, want miss")
	}
}

// 平台的配置字段就是配置接口的契约：必填缺失、未知字段、超长都要在写库前拒掉。
func TestValidate(t *testing.T) {
	wecom, ok := Get(WeCom)
	if !ok {
		t.Fatal("wecom platform is not registered")
	}

	tests := []struct {
		name    string
		config  map[string]string
		wantErr string
	}{
		{
			name:   "complete config accepted",
			config: map[string]string{"bot_id": "AIBOT1", "bot_secret": "s3cret"},
		},
		{
			name:    "missing required field",
			config:  map[string]string{"bot_id": "AIBOT1"},
			wantErr: "bot_secret is required",
		},
		{
			name:    "blank required field",
			config:  map[string]string{"bot_id": "AIBOT1", "bot_secret": "   "},
			wantErr: "bot_secret is required",
		},
		{
			name:    "unknown field",
			config:  map[string]string{"bot_id": "AIBOT1", "bot_secret": "s", "extra": "x"},
			wantErr: `unknown field "extra"`,
		},
		{
			name:    "value too long",
			config:  map[string]string{"bot_id": strings.Repeat("a", maxFieldValueBytes+1), "bot_secret": "s"},
			wantErr: "is too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := wecom.Validate(tt.config)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// 公网回显绝不带明文 Secret，非敏感字段保持原样。
func TestMask(t *testing.T) {
	wecom, _ := Get(WeCom)
	masked := wecom.Mask(map[string]string{"bot_id": "AIBOT123456789", "bot_secret": "supersecretvalue"})

	if masked["bot_secret"] == "supersecretvalue" {
		t.Fatal("secret value leaked through Mask")
	}
	if masked["bot_id"] != "AIBOT123456789" {
		t.Errorf("non-secret field = %q, want unchanged", masked["bot_id"])
	}
	if len(masked) != 2 {
		t.Errorf("masked keys = %v, want both fields present", masked)
	}
}

func TestMaskCredential(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"long value keeps edges", "1234567890", "1234...7890"},
		{"short value fully masked", "short", "********"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskCredential(tt.in); got != tt.want {
				t.Errorf("MaskCredential(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
