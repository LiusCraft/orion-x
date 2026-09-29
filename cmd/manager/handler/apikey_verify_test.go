package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/store"
)

// fakeDeviceOwner 是 DeviceOwner 的内联替身（仓库约定：不引 mock 生成器）。
type fakeDeviceOwner struct {
	owners map[string]string
	err    error
}

func (f fakeDeviceOwner) OwnerID(_ context.Context, deviceID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.owners[deviceID], nil
}

// verifyRequest 直接驱动 handler：请求体是 JSON，路径与中间件无关（这里只测判定）。
func verifyRequest(body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, apikey.PathVerify, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func decodeVerifyResponse(t *testing.T, w *httptest.ResponseRecorder) apikey.VerifyResponse {
	t.Helper()
	var resp apikey.VerifyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	return resp
}

// TestAPIKeyVerifyDecisionMatrix 钉住 §3.3 的判定顺序与拒绝矩阵：
// Authenticate → 限流 → scope → 设备归属；设备不存在与不归属统一 device_not_owned。
func TestAPIKeyVerifyDecisionMatrix(t *testing.T) {
	expiredAt := time.Now().Add(-time.Hour)
	revokedAt := time.Now().Add(-time.Minute)

	cases := []struct {
		name      string
		scopes    []string
		owners    map[string]string
		deviceID  string
		rawKey    string
		prepare   func(st *fakeAPIKeyStore, id string)
		wantAllow bool
		wantCause apikey.RejectReason
		wantReq   []string
		wantGrant []string
	}{
		{
			name:      "allowed",
			scopes:    []string{apikey.ScopeDeviceConnect, apikey.ScopeAgentRead},
			owners:    map[string]string{"dev-1": "user-1"},
			deviceID:  "dev-1",
			wantAllow: true,
		},
		{
			name:      "invalid key",
			scopes:    []string{apikey.ScopeDeviceConnect},
			owners:    map[string]string{"dev-1": "user-1"},
			deviceID:  "dev-1",
			rawKey:    "ox_sk_not-a-real-key",
			wantCause: apikey.RejectInvalidKey,
		},
		{
			name:     "revoked key",
			scopes:   []string{apikey.ScopeDeviceConnect},
			owners:   map[string]string{"dev-1": "user-1"},
			deviceID: "dev-1",
			prepare: func(st *fakeAPIKeyStore, id string) {
				st.rows[id].RevokedAt = &revokedAt
			},
			wantCause: apikey.RejectKeyRevoked,
		},
		{
			name:     "expired key",
			scopes:   []string{apikey.ScopeDeviceConnect},
			owners:   map[string]string{"dev-1": "user-1"},
			deviceID: "dev-1",
			prepare: func(st *fakeAPIKeyStore, id string) {
				st.rows[id].ExpiresAt = &expiredAt
			},
			wantCause: apikey.RejectKeyExpired,
		},
		{
			name:      "missing scope",
			scopes:    []string{apikey.ScopeAgentRead},
			owners:    map[string]string{"dev-1": "user-1"},
			deviceID:  "dev-1",
			wantCause: apikey.RejectInsufficientScope,
			wantReq:   []string{apikey.ScopeDeviceConnect},
			wantGrant: []string{apikey.ScopeAgentRead},
		},
		{
			name:      "device owned by someone else",
			scopes:    []string{apikey.ScopeDeviceConnect},
			owners:    map[string]string{"dev-1": "user-2"},
			deviceID:  "dev-1",
			wantCause: apikey.RejectDeviceNotOwned,
		},
		{
			// 设备不存在与不归属同码：不给“这个 device_id 存在吗”的枚举面。
			name:      "unknown device",
			scopes:    []string{apikey.ScopeDeviceConnect},
			owners:    map[string]string{},
			deviceID:  "dev-ghost",
			wantCause: apikey.RejectDeviceNotOwned,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newFakeAPIKeyStore()
			// 限流放得很宽：这条用例关心的是授权矩阵，不是配额。
			svc := apikey.New(st, apikey.Config{RateLimitRPS: 1000, RateLimitBurst: 1000})
			plaintext, record, err := svc.Create(context.Background(), "user-1", "device", tc.scopes, nil)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if tc.prepare != nil {
				tc.prepare(st, record.ID)
			}
			rawKey := tc.rawKey
			if rawKey == "" {
				rawKey = plaintext
			}

			h := NewAPIKeyHandler(svc, false, fakeDeviceOwner{owners: tc.owners})
			c, w := verifyRequest(fmt.Sprintf(`{"key":%q,"device_id":%q}`, rawKey, tc.deviceID))
			h.Verify(c)

			if w.Code != http.StatusOK {
				t.Fatalf("Verify = %d, want 200 (body %s)", w.Code, w.Body.String())
			}
			resp := decodeVerifyResponse(t, w)
			if resp.Allowed != tc.wantAllow {
				t.Fatalf("allowed = %v, want %v (body %s)", resp.Allowed, tc.wantAllow, w.Body.String())
			}
			if resp.RejectReason != tc.wantCause {
				t.Fatalf("reject_reason = %q, want %q (body %s)", resp.RejectReason, tc.wantCause, w.Body.String())
			}
			if tc.wantAllow {
				if resp.UserID != "user-1" || resp.KeyID != record.ID {
					t.Fatalf("identity = %+v, want user-1/%s", resp, record.ID)
				}
				if len(resp.Scopes) != len(tc.scopes) {
					t.Fatalf("scopes = %v, want %v", resp.Scopes, tc.scopes)
				}
				return
			}
			if strings.Join(resp.Required, ",") != strings.Join(tc.wantReq, ",") {
				t.Errorf("required = %v, want %v", resp.Required, tc.wantReq)
			}
			if strings.Join(resp.Granted, ",") != strings.Join(tc.wantGrant, ",") {
				t.Errorf("granted = %v, want %v", resp.Granted, tc.wantGrant)
			}
			if strings.Contains(w.Body.String(), rawKey) && tc.rawKey == "" {
				t.Error("rejection body leaks the plaintext key")
			}
		})
	}
}

// TestAPIKeyVerifyRateLimited 限流与 HTTP 面共用同一个桶：同一个 key 的
// 第二次校验被拒（FR-8 的反面）。
func TestAPIKeyVerifyRateLimited(t *testing.T) {
	st := newFakeAPIKeyStore()
	svc := apikey.New(st, apikey.Config{RateLimitRPS: 0.001, RateLimitBurst: 1})
	plaintext, _, err := svc.Create(context.Background(), "user-1", "device", []string{apikey.ScopeDeviceConnect}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	h := NewAPIKeyHandler(svc, false, fakeDeviceOwner{owners: map[string]string{"dev-1": "user-1"}})
	body := fmt.Sprintf(`{"key":%q,"device_id":"dev-1"}`, plaintext)

	c, w := verifyRequest(body)
	h.Verify(c)
	if resp := decodeVerifyResponse(t, w); !resp.Allowed {
		t.Fatalf("first verify = %+v, want allowed", resp)
	}

	c, w = verifyRequest(body)
	h.Verify(c)
	resp := decodeVerifyResponse(t, w)
	if resp.Allowed || resp.RejectReason != apikey.RejectRateLimited {
		t.Fatalf("second verify = %+v, want rate_limited", resp)
	}
}

// TestAPIKeyVerifyRecordsUsage WS 连接要与 HTTP 请求共用用量计数：
// verify 放行后 call_count/last_used_at 记一笔（FR-8）。
func TestAPIKeyVerifyRecordsUsage(t *testing.T) {
	st := newFakeAPIKeyStore()
	svc := apikey.New(st, apikey.Config{})
	plaintext, _, err := svc.Create(context.Background(), "user-1", "device", []string{apikey.ScopeDeviceConnect}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	h := NewAPIKeyHandler(svc, false, fakeDeviceOwner{owners: map[string]string{"dev-1": "user-1"}})

	c, w := verifyRequest(fmt.Sprintf(`{"key":%q,"device_id":"dev-1"}`, plaintext))
	h.Verify(c)
	if resp := decodeVerifyResponse(t, w); !resp.Allowed {
		t.Fatalf("verify = %+v, want allowed", resp)
	}

	svc.Flush(context.Background())
	if st.usageCalls != 1 {
		t.Fatalf("usage calls = %d, want 1 (WS 连接计入 call_count/last_used_at)", st.usageCalls)
	}
}

// TestAPIKeyVerifyRequestErrors 请求体与依赖运行期故障的状态码：400 / 500。
func TestAPIKeyVerifyRequestErrors(t *testing.T) {
	svc := apikey.New(newFakeAPIKeyStore(), apikey.Config{})

	t.Run("invalid json", func(t *testing.T) {
		h := NewAPIKeyHandler(svc, false, fakeDeviceOwner{})
		c, w := verifyRequest(`{`)
		h.Verify(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("owner resolver failure", func(t *testing.T) {
		st := newFakeAPIKeyStore()
		s := apikey.New(st, apikey.Config{})
		plaintext, _, err := s.Create(context.Background(), "user-1", "device", []string{apikey.ScopeDeviceConnect}, nil)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		h := NewAPIKeyHandler(s, false, fakeDeviceOwner{err: errors.New("db down")})
		c, w := verifyRequest(fmt.Sprintf(`{"key":%q,"device_id":"dev-1"}`, plaintext))
		h.Verify(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

// TestNewAPIKeyHandlerRequiresOwner 装配期约束：verify 的归属判定没有 owner 就没法
// 工作，构造时直接 panic，不把装配错误拖到第一个握手。
func TestNewAPIKeyHandlerRequiresOwner(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewAPIKeyHandler(svc, false, nil) must panic")
		}
	}()
	NewAPIKeyHandler(apikey.New(newFakeAPIKeyStore(), apikey.Config{}), false, nil)
}

// TestNewStoreDeviceOwnerRequiresStores 同理：store 实现缺任何一个都装配不出来。
func TestNewStoreDeviceOwnerRequiresStores(t *testing.T) {
	for _, tc := range []struct {
		name      string
		devices   *store.DeviceStore
		voicebots *store.VoicebotStore
	}{
		{name: "missing devices", voicebots: &store.VoicebotStore{}},
		{name: "missing voicebots", devices: &store.DeviceStore{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("NewStoreDeviceOwner must panic on nil store")
				}
			}()
			NewStoreDeviceOwner(tc.devices, tc.voicebots)
		})
	}
}
