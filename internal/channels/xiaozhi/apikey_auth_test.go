package xiaozhi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/channels"
	"github.com/liuscraft/orion-x/internal/channels/xiaozhi/wsproto"
)

// fakeAPIKeyAuthorizer 是 channels.APIKeyAuthorizer 的内联 mock（仓库约定：
// 手写 mock struct，不引 mock 生成器）。
type fakeAPIKeyAuthorizer struct {
	response apikey.AuthorizeResponse
	err      error
	calls    int
	lastKey  string
	lastDev  string
}

func (f *fakeAPIKeyAuthorizer) Authorize(_ context.Context, key, deviceID string) (apikey.AuthorizeResponse, error) {
	f.calls++
	f.lastKey = key
	f.lastDev = deviceID
	return f.response, f.err
}

func testKey(t *testing.T) string {
	t.Helper()
	key, err := apikey.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	return key
}

func newAuthTestChannel(deps *channels.Dependencies, requireKey bool) *XiaozhiWSChannel {
	cfg := DefaultConfig()
	cfg.Auth.RequireAPIKey = requireKey
	return NewXiaozhiWSChannel(cfg, deps, nil, nil)
}

func TestAuthorizeConnectionWithoutAKey(t *testing.T) {
	t.Run("optional by default", func(t *testing.T) {
		verifier := &fakeAPIKeyAuthorizer{}
		ch := newAuthTestChannel(&channels.Dependencies{APIKeyAuth: verifier}, false)
		if rejected := ch.authorizeConnection("device-1", ""); rejected != nil {
			t.Fatalf("rejected = %+v, want nil", rejected)
		}
		if verifier.calls != 0 {
			t.Errorf("verifier calls = %d, want 0", verifier.calls)
		}
	})

	t.Run("required rejects", func(t *testing.T) {
		verifier := &fakeAPIKeyAuthorizer{}
		ch := newAuthTestChannel(&channels.Dependencies{APIKeyAuth: verifier}, true)
		rejected := ch.authorizeConnection("device-1", "")
		if rejected == nil {
			t.Fatal("rejected = nil, want a rejection")
		}
		if rejected.Reason != string(apikey.RejectKeyMissing) {
			t.Errorf("reason = %q, want %q", rejected.Reason, apikey.RejectKeyMissing)
		}
		if verifier.calls != 0 {
			t.Errorf("verifier calls = %d, want 0", verifier.calls)
		}
	})

	t.Run("foreign credentials are ignored unless required", func(t *testing.T) {
		// 固件自带的随机 token 不是我们的格式：默认照旧放行（历史行为），
		// 开了 require_api_key 就一视同仁地拒。
		verifier := &fakeAPIKeyAuthorizer{}
		optional := newAuthTestChannel(&channels.Dependencies{APIKeyAuth: verifier}, false)
		if rejected := optional.authorizeConnection("device-1", "some-firmware-token"); rejected != nil {
			t.Fatalf("optional: rejected = %+v, want nil", rejected)
		}
		required := newAuthTestChannel(&channels.Dependencies{APIKeyAuth: verifier}, true)
		if rejected := required.authorizeConnection("device-1", "some-firmware-token"); rejected == nil {
			t.Fatal("required: rejected = nil, want a rejection")
		}
		if verifier.calls != 0 {
			t.Errorf("verifier calls = %d, want 0", verifier.calls)
		}
	})
}

// TestHandshakeRejectionOverWebSocket 走真 WebSocket：验证 handleWS 拿到的凭据确实
// 传到了握手鉴权，且拒绝时的收尾是“先回 hello 再 Close(1008, reason)”。
// 不需要真设备配置——鉴权发生在建 pipeline 之前。
func TestHandshakeRejectionOverWebSocket(t *testing.T) {
	verifier := &fakeAPIKeyAuthorizer{response: apikey.AuthorizeResponse{
		Allowed: false, RejectReason: apikey.RejectOwnerMismatch,
	}}
	ch := newAuthTestChannel(&channels.Dependencies{APIKeyAuth: verifier}, true)

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		ch.handleConnection(conn, r.Header.Get("Authorization"))
	}))
	defer server.Close()

	header := http.Header{"Authorization": []string{"Bearer " + testKey(t)}}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.WriteJSON(wsproto.HelloMessage{Type: wsproto.TypeHello, DeviceID: "device-1"}); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var hello wsproto.HelloMessage
	if err := conn.ReadJSON(&hello); err != nil {
		t.Fatalf("read hello response: %v", err)
	}
	if hello.Type != wsproto.TypeHello {
		t.Errorf("first frame = %q, want a hello response", hello.Type)
	}

	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		t.Fatalf("second frame error = %v, want a close frame", err)
	}
	if closeErr.Code != websocket.ClosePolicyViolation {
		t.Errorf("close code = %d, want %d", closeErr.Code, websocket.ClosePolicyViolation)
	}
	if closeErr.Text != string(apikey.RejectOwnerMismatch) {
		t.Errorf("close reason = %q, want %q", closeErr.Text, apikey.RejectOwnerMismatch)
	}
	if verifier.calls != 1 || verifier.lastDev != "device-1" {
		t.Errorf("verifier calls=%d device=%q, want the device from the hello frame", verifier.calls, verifier.lastDev)
	}
}

func TestAuthorizeConnectionWithAKey(t *testing.T) {
	key := testKey(t)

	t.Run("allowed", func(t *testing.T) {
		verifier := &fakeAPIKeyAuthorizer{response: apikey.AuthorizeResponse{Allowed: true, OwnerID: "user-1"}}
		ch := newAuthTestChannel(&channels.Dependencies{APIKeyAuth: verifier}, false)
		if rejected := ch.authorizeConnection("device-1", key); rejected != nil {
			t.Fatalf("rejected = %+v, want nil", rejected)
		}
		if verifier.calls != 1 || verifier.lastKey != key || verifier.lastDev != "device-1" {
			t.Errorf("verifier calls=%d key=%q device=%q", verifier.calls, verifier.lastKey, verifier.lastDev)
		}
	})

	t.Run("rejected by the control plane", func(t *testing.T) {
		verifier := &fakeAPIKeyAuthorizer{response: apikey.AuthorizeResponse{Allowed: false, RejectReason: apikey.RejectOwnerMismatch}}
		ch := newAuthTestChannel(&channels.Dependencies{APIKeyAuth: verifier}, false)
		rejected := ch.authorizeConnection("device-1", key)
		if rejected == nil {
			t.Fatal("rejected = nil, want a rejection")
		}
		if rejected.Reason != string(apikey.RejectOwnerMismatch) {
			t.Errorf("reason = %q, want %q", rejected.Reason, apikey.RejectOwnerMismatch)
		}
	})

	t.Run("unreachable control plane fails closed", func(t *testing.T) {
		verifier := &fakeAPIKeyAuthorizer{err: errors.New("connection refused")}
		ch := newAuthTestChannel(&channels.Dependencies{APIKeyAuth: verifier}, false)
		rejected := ch.authorizeConnection("device-1", key)
		if rejected == nil {
			t.Fatal("rejected = nil, want a rejection")
		}
		if rejected.Reason != string(apikey.RejectKeyUnavailable) {
			t.Errorf("reason = %q, want %q", rejected.Reason, apikey.RejectKeyUnavailable)
		}
	})

	t.Run("no verifier injected fails closed", func(t *testing.T) {
		cases := []struct {
			name string
			deps *channels.Dependencies
		}{
			{name: "nil deps", deps: nil},
			{name: "deps without a verifier", deps: &channels.Dependencies{}},
		}
		for _, tc := range cases {
			ch := newAuthTestChannel(tc.deps, false)
			rejected := ch.authorizeConnection("device-1", key)
			if rejected == nil {
				t.Fatalf("%s: rejected = nil, want a rejection", tc.name)
			}
			if rejected.Reason != string(apikey.RejectKeyUnavailable) {
				t.Errorf("%s: reason = %q, want %q", tc.name, rejected.Reason, apikey.RejectKeyUnavailable)
			}
		}
	})

	t.Run("key is trimmed before it is sent", func(t *testing.T) {
		verifier := &fakeAPIKeyAuthorizer{response: apikey.AuthorizeResponse{Allowed: true}}
		ch := newAuthTestChannel(&channels.Dependencies{APIKeyAuth: verifier}, false)
		if rejected := ch.authorizeConnection("device-1", "  "+key+"  "); rejected != nil {
			t.Fatalf("rejected = %+v, want nil", rejected)
		}
		if verifier.lastKey != key {
			t.Errorf("verifier key = %q, want the trimmed key", verifier.lastKey)
		}
	})

	t.Run("both credential entry points work", func(t *testing.T) {
		// Authorization 头带 "Bearer " 前缀，?access_token= 是裸 token：两种入口
		// 必须交到控制面同一把钥，否则头形式会静默地变成“没带凭据”。
		for _, credential := range []string{key, "Bearer " + key, "Bearer   " + key} {
			verifier := &fakeAPIKeyAuthorizer{response: apikey.AuthorizeResponse{Allowed: true}}
			ch := newAuthTestChannel(&channels.Dependencies{APIKeyAuth: verifier}, true)
			if rejected := ch.authorizeConnection("device-1", credential); rejected != nil {
				t.Fatalf("credential %q: rejected = %+v, want nil", credential, rejected)
			}
			if verifier.lastKey != key {
				t.Errorf("credential %q: verifier key = %q, want %q", credential, verifier.lastKey, key)
			}
		}
	})
}
