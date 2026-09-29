package xiaozhi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/internal/apikey"
	apikeyclient "github.com/liuscraft/orion-x/internal/apikey/client"
	"github.com/liuscraft/orion-x/internal/channels"
	"github.com/liuscraft/orion-x/internal/channels/xiaozhi/wsproto"
	"github.com/liuscraft/orion-x/internal/logging"
)

// fakeVerifier 是 channels.KeyVerifier 的内联替身（仓库约定：不引 mock 生成器）。
// handler 在独立 goroutine 里跑，字段读写都要过锁。
type fakeVerifier struct {
	mu       sync.Mutex
	resp     apikey.VerifyResponse
	err      error
	calls    int
	rawKey   string
	deviceID string
}

func (f *fakeVerifier) Verify(_ context.Context, rawKey, deviceID string) (apikey.VerifyResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.rawKey = rawKey
	f.deviceID = deviceID
	return f.resp, f.err
}

func (f *fakeVerifier) snapshot() (calls int, rawKey, deviceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.rawKey, f.deviceID
}

// newAuthTestChannel 组装一个只关心握手路径的通道：verifier 由调用方注入，
// tools/memory 为 nil（拒绝路径永远走不到它们）。
//
// 注册的 cleanup 会等所有连接 goroutine 退出：否则它们的日志会与下一个用例的
// logging.Init 竞争全局 logger（-race 下必红）。
func newAuthTestChannel(t *testing.T, verifier channels.KeyVerifier) *XiaozhiWSChannel {
	t.Helper()
	deps := &channels.Dependencies{KeyVerifier: verifier}
	ch := NewXiaozhiWSChannel(DefaultConfig(), deps, nil, nil)
	t.Cleanup(func() { ch.connWG.Wait() })
	return ch
}

func wsURLOf(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

func dialWS(t *testing.T, srv *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", token)
	}
	conn, resp, err := websocket.DefaultDialer.Dial(wsURLOf(srv), header)
	if err != nil {
		t.Fatalf("dial: %v (status %v)", err, resp)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func writeHello(t *testing.T, conn *websocket.Conn, deviceID string) {
	t.Helper()
	hello := wsproto.HelloMessage{
		Type:     wsproto.TypeHello,
		DeviceID: deviceID,
		AudioParams: wsproto.AudioParams{
			Format:        "opus",
			SampleRate:    16000,
			Channels:      1,
			FrameDuration: 60,
		},
	}
	if err := conn.WriteJSON(hello); err != nil {
		t.Fatalf("write hello: %v", err)
	}
}

// readHelloThenClose 读一条 hello 响应，再读 Close 帧并返回其 code/reason。
func readHelloThenClose(t *testing.T, conn *websocket.Conn) (int, string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	var hello wsproto.HelloMessage
	if err := conn.ReadJSON(&hello); err != nil {
		t.Fatalf("read hello response: %v", err)
	}
	if hello.Type != wsproto.TypeHello {
		t.Fatalf("first frame type = %q, want hello", hello.Type)
	}
	if hello.SessionID == "" {
		t.Fatal("rejection hello must still carry a session_id")
	}

	_, _, err := conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		t.Fatalf("want close error after hello, got %v", err)
	}
	return closeErr.Code, closeErr.Text
}

// testKey 造一把形状合法的 Key：日志脱敏断言需要一个能解析出公开段的真 key。
func testKey(t *testing.T) string {
	t.Helper()
	plaintext, _, err := apikey.Generate("user-1", "device", []string{apikey.ScopeDeviceConnect}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	return plaintext
}

// TestHandleWSMissingKeyRejectedBeforeUpgrade：鉴权开启时，无 Key 的握手在升级前
// 就 401，且不调用校验（FR-1）。
func TestHandleWSMissingKeyRejectedBeforeUpgrade(t *testing.T) {
	verifier := &fakeVerifier{}
	srv := httptest.NewServer(newAuthTestChannel(t, verifier).wsHandler())
	defer srv.Close()

	_, resp, err := websocket.DefaultDialer.Dial(wsURLOf(srv), nil)
	if err == nil {
		t.Fatal("dial must fail without a key when auth is enabled")
	}
	if resp == nil {
		t.Fatal("expected an HTTP response for the rejected upgrade")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"error":"missing api key"}` {
		t.Fatalf("body = %q, want the missing api key error", body)
	}
	if calls, _, _ := verifier.snapshot(); calls != 0 {
		t.Fatalf("verifier calls = %d, want 0 (rejection happens before verify)", calls)
	}
}

// TestHandleConnectionRejectsWithClose1008：key 被业务拒绝或校验链路故障时，
// 先回 hello 再 Close 1008，reason 即 reject_reason（本地故障是 auth_unavailable）。
func TestHandleConnectionRejectsWithClose1008(t *testing.T) {
	cases := []struct {
		name       string
		resp       apikey.VerifyResponse
		err        error
		wantReason string
	}{
		{
			name:       "insufficient scope",
			resp:       apikey.VerifyResponse{Allowed: false, RejectReason: apikey.RejectInsufficientScope},
			wantReason: string(apikey.RejectInsufficientScope),
		},
		{
			// 未知 reason 原样透传：字段只增，旧数据面不需要认识新值。
			name:       "unknown reason passthrough",
			resp:       apikey.VerifyResponse{Allowed: false, RejectReason: "brand_new_reason"},
			wantReason: "brand_new_reason",
		},
		{
			name:       "verifier transport failure is fail closed",
			err:        errors.New("manager unreachable"),
			wantReason: authUnavailableReason,
		},
		{
			name:       "rejection without a reason is treated as a protocol failure",
			resp:       apikey.VerifyResponse{Allowed: false},
			wantReason: authUnavailableReason,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verifier := &fakeVerifier{resp: tc.resp, err: tc.err}
			srv := httptest.NewServer(newAuthTestChannel(t, verifier).wsHandler())
			defer srv.Close()

			conn := dialWS(t, srv, "Bearer "+testKey(t))
			writeHello(t, conn, "dev-1")

			code, reason := readHelloThenClose(t, conn)
			if code != websocket.ClosePolicyViolation {
				t.Fatalf("close code = %d, want %d", code, websocket.ClosePolicyViolation)
			}
			if reason != tc.wantReason {
				t.Fatalf("close reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

// TestHandleConnectionVerifiesHelloDeviceIDAndQueryToken：device_id 以 hello 为准
// （header 里的不算），access_token query 与 header 等价（§1 D3、§3.2）。
func TestHandleConnectionVerifiesHelloDeviceIDAndQueryToken(t *testing.T) {
	verifier := &fakeVerifier{resp: apikey.VerifyResponse{Allowed: false, RejectReason: apikey.RejectDeviceNotOwned}}
	srv := httptest.NewServer(newAuthTestChannel(t, verifier).wsHandler())
	defer srv.Close()

	key := testKey(t)
	header := http.Header{}
	// header 报的 device_id 与 hello 里的不同：校验必须用 hello 的（避免绕过）。
	header.Set("Device-Id", "dev-from-header")
	dialURL := wsURLOf(srv) + "?access_token=" + url.QueryEscape(key)
	conn, resp, err := websocket.DefaultDialer.Dial(dialURL, header)
	if err != nil {
		t.Fatalf("dial: %v (status %v)", err, resp)
	}
	defer func() { _ = conn.Close() }()

	writeHello(t, conn, "dev-from-hello")
	if code, _ := readHelloThenClose(t, conn); code != websocket.ClosePolicyViolation {
		t.Fatalf("close code = %d, want 1008", code)
	}

	calls, rawKey, deviceID := verifier.snapshot()
	if calls != 1 {
		t.Fatalf("verifier calls = %d, want 1", calls)
	}
	if rawKey != key {
		t.Fatal("verifier did not receive the query access_token")
	}
	if deviceID != "dev-from-hello" {
		t.Fatalf("verifier device_id = %q, want the hello value", deviceID)
	}
}

// TestHandleWSAuthDisabledKeepsCurrentBehavior：auth.enabled: false 时无 Key 也能
// 升级，行为与现状一致（FR-6）。
func TestHandleWSAuthDisabledKeepsCurrentBehavior(t *testing.T) {
	srv := httptest.NewServer(newAuthTestChannel(t, nil).wsHandler())
	defer srv.Close()

	conn, resp, err := websocket.DefaultDialer.Dial(wsURLOf(srv), nil)
	if err != nil {
		t.Fatalf("auth disabled must not require a key: %v (status %v)", err, resp)
	}
	defer func() { _ = conn.Close() }()

	// 升级成功即可；后续建 pipeline 会因为没有 provider 配置而失败，那是既有行为。
	writeHello(t, conn, "dev-1")
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// TestHandshakeNeverLogsTheFullKey：R3/FR-5 的日志侧断言——完整 Key 一旦进日志
// 就是事故；失败路径只允许出现公开段（lookup）。
func TestHandshakeNeverLogsTheFullKey(t *testing.T) {
	key := testKey(t)
	lookup := apikey.LookupOf(key)
	if lookup == "" {
		t.Fatal("fixture is not parseable")
	}

	// zap 在 Init 时就把 os.Stderr 锁进它的 core，所以必须先把 stderr 换掉再 Init。
	original := os.Stderr
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = writePipe
	t.Cleanup(func() {
		os.Stderr = original
		if err := logging.Init(logging.Config{Level: "warn", Format: "console"}); err != nil {
			t.Errorf("restore logger: %v", err)
		}
		_ = readPipe.Close()
		_ = writePipe.Close()
	})
	if err := logging.Init(logging.Config{Level: "debug", Format: "console"}); err != nil {
		t.Fatalf("logging.Init: %v", err)
	}

	verifier := &fakeVerifier{err: errors.New("manager unreachable")}
	ch := newAuthTestChannel(t, verifier)
	srv := httptest.NewServer(ch.wsHandler())
	defer srv.Close()

	conn := dialWS(t, srv, "Bearer "+key)
	writeHello(t, conn, "dev-1")
	if code, reason := readHelloThenClose(t, conn); code != websocket.ClosePolicyViolation || reason != authUnavailableReason {
		t.Fatalf("rejection = (%d, %q), want (1008, %q)", code, reason, authUnavailableReason)
	}
	// 先等连接 goroutine 退出，再恢复 logger：它的最后一条日志必须落在管道里，
	// 也不能与下面的 Init 竞争。
	ch.connWG.Wait()

	logging.Sync()
	if err := writePipe.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	captured, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatalf("read log output: %v", err)
	}
	log := string(captured)

	// 先确认真的抓到了相关日志，否则"没泄漏"只是因为什么都没记。
	if !strings.Contains(log, "handshake verify failed") {
		t.Fatalf("expected the failure to be logged, got: %q", log)
	}
	if strings.Contains(log, key) {
		t.Fatalf("log output contains the full key: %q", log)
	}
	if !strings.Contains(log, lookup) {
		t.Fatalf("log output should carry the public segment %q for troubleshooting, got: %q", lookup, log)
	}
}

// TestAccessTokenParsing：header 优先、Bearer 与裸串都认。
func TestAccessTokenParsing(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{raw: "", want: ""},
		{raw: "   ", want: ""},
		{raw: "ox_sk_abc", want: "ox_sk_abc"},
		{raw: "Bearer ox_sk_abc", want: "ox_sk_abc"},
		{raw: "Bearer   ox_sk_abc  ", want: "ox_sk_abc"},
		{raw: "bearer ox_sk_abc", want: "bearer ox_sk_abc"}, // 大小写敏感：不是合法 Bearer 就当裸串
	}
	for _, tc := range cases {
		if got := accessToken(tc.raw); got != tc.want {
			t.Errorf("accessToken(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// TestRedactAuthorization：能解析出公开段就只留公开段，其余一律脱敏。
func TestRedactAuthorization(t *testing.T) {
	key := testKey(t)
	lookup := apikey.LookupOf(key)
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: "<none>"},
		{name: "bearer key", raw: "Bearer " + key, want: "lookup=" + lookup},
		{name: "raw key", raw: key, want: "lookup=" + lookup},
		{name: "other credential", raw: "Bearer some-jwt-token", want: "<redacted>"},
		{name: "garbage", raw: "not-a-key", want: "<redacted>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactAuthorization(tc.raw); got != tc.want {
				t.Fatalf("redactAuthorization() = %q, want %q", got, tc.want)
			}
			if strings.Contains(redactAuthorization(tc.raw), key) {
				t.Fatal("redacted value contains the full key")
			}
		})
	}
}

// TestMemoryUserID：鉴权开启时 user_id 取 Key 的主人（FR-4），关闭时保持现状。
func TestMemoryUserID(t *testing.T) {
	cases := []struct {
		name      string
		sessionID string
		hello     *wsproto.HelloMessage
		auth      *helloAuth
		want      string
	}{
		{
			name: "auth off keeps device_id", sessionID: "sess-1",
			hello: &wsproto.HelloMessage{DeviceID: "dev-1"}, want: "dev-1",
		},
		{
			name: "verified uses the key owner", sessionID: "sess-1",
			hello: &wsproto.HelloMessage{DeviceID: "dev-1"}, auth: &helloAuth{UserID: "user-1"}, want: "user-1",
		},
		{
			name: "verified without a user falls back to device_id", sessionID: "sess-1",
			hello: &wsproto.HelloMessage{DeviceID: "dev-1"}, auth: &helloAuth{}, want: "dev-1",
		},
		{
			name: "no device falls back to the session id", sessionID: "sess-1",
			hello: &wsproto.HelloMessage{}, want: "sess-1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := memoryUserID(tc.sessionID, tc.hello, tc.auth); got != tc.want {
				t.Fatalf("memoryUserID() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestVerifyHandshakeAgainstFakeManager 用真的 apikey/client 打一个假 manager：
// 钉住数据面 → 控制面的 wire 契约（路径、内部 token、请求体）与“允许/拒绝”两种
// 结论到 helloAuth/关帧 reason 的映射（设计 §D 阶段 2 的“假 manager 集成”）。
func TestVerifyHandshakeAgainstFakeManager(t *testing.T) {
	tests := []struct {
		name       string
		resp       apikey.VerifyResponse
		wantAuth   *helloAuth
		wantReason string
	}{
		{
			name:     "allowed",
			resp:     apikey.VerifyResponse{Allowed: true, KeyID: "key-1", UserID: "user-1", Scopes: []string{apikey.ScopeDeviceConnect}},
			wantAuth: &helloAuth{UserID: "user-1", KeyID: "key-1"},
		},
		{
			name:       "rejected",
			resp:       apikey.VerifyResponse{Allowed: false, RejectReason: apikey.RejectDeviceNotOwned},
			wantReason: string(apikey.RejectDeviceNotOwned),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotAuth string
			var gotBody apikey.VerifyRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					t.Errorf("decode verify request: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tc.resp)
			}))
			defer srv.Close()

			verifier := apikeyclient.New(apikeyclient.Config{BaseURL: srv.URL, Token: "internal-token"})
			ch := newAuthTestChannel(t, verifier)
			key := testKey(t)

			auth, rejected := ch.verifyHandshake(key, &wsproto.HelloMessage{DeviceID: "dev-1"})

			if gotPath != apikey.PathVerify {
				t.Fatalf("verify path = %q, want %q", gotPath, apikey.PathVerify)
			}
			if gotAuth != "Bearer internal-token" {
				t.Fatalf("verify authorization = %q, want the internal token", gotAuth)
			}
			if gotBody.Key != key || gotBody.DeviceID != "dev-1" {
				t.Fatalf("verify body = %+v, want key + hello device_id", gotBody)
			}
			if tc.wantAuth != nil {
				if rejected != nil || auth == nil || *auth != *tc.wantAuth {
					t.Fatalf("verifyHandshake() = (%+v, %+v), want auth %+v", auth, rejected, tc.wantAuth)
				}
				return
			}
			if auth != nil || rejected == nil || rejected.Reason != tc.wantReason {
				t.Fatalf("verifyHandshake() = (%+v, %+v), want rejection %q", auth, rejected, tc.wantReason)
			}
		})
	}
}
