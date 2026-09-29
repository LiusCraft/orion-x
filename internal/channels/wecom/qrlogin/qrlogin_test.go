package qrlogin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/channels/platform"
	"github.com/liuscraft/orion-x/internal/channels/qrbind"
)

// fakeQC 是企微 /ai/qc/* 两个端点的替身：generate 固定发一个 scode，query_result
// 按脚本逐个应答（脚本用完后重复最后一个）。
type fakeQC struct {
	mu sync.Mutex

	generateStatus int
	generateBody   string
	queryBodies    []string // 依次返回；空 = 一直 pending
	queryStatus    int      // 非 200 时按 HTTP 状态返回（模拟网络/网关抖动）

	generateCalls int
	queryCalls    int
	requests      []url.Values
}

func (f *fakeQC) start(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r.URL.Query())
		switch r.URL.Path {
		case generatePath:
			f.generateCalls++
			status := f.generateStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(f.generateBody))
		case queryPath:
			body := `{"errcode":0,"data":{"status":"pending"}}`
			if len(f.queryBodies) > 0 {
				idx := f.queryCalls
				if idx >= len(f.queryBodies) {
					idx = len(f.queryBodies) - 1
				}
				body = f.queryBodies[idx]
			}
			f.queryCalls++
			if f.queryStatus != 0 && f.queryStatus != http.StatusOK {
				w.WriteHeader(f.queryStatus)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(body))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func newTestSessions(t *testing.T, base string) *Sessions {
	t.Helper()
	s := New(context.Background(), Options{
		BaseURL:              base,
		PollInterval:         5 * time.Millisecond,
		SessionTTL:           time.Second,
		RetentionAfterExpiry: time.Second,
		Timeout:              time.Second,
	})
	t.Cleanup(s.Close)
	return s
}

func waitForStatus(t *testing.T, s *Sessions, deviceID, sessionID string, want qrbind.Status) qrbind.Session {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sess, err := s.Get(deviceID, sessionID)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if sess.Status == want {
			return sess
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("session did not reach %s within 2s", want)
	return qrbind.Session{}
}

func TestStartPollsUntilSuccess(t *testing.T) {
	f := &fakeQC{
		generateBody: `{"errcode":0,"data":{"scode":"sc-1","auth_url":"https://work.weixin.qq.com/ai/qc/auth?scode=sc-1"}}`,
		queryBodies: []string{
			`{"errcode":0,"data":{"status":"scaned"}}`,
			`{"errcode":0,"data":{"status":"success","bot_info":{"botid":"AIBOT123","secret":"s3cret"}}}`,
		},
	}
	s := newTestSessions(t, f.start(t))

	sess, err := s.Start("device-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if sess.Status != qrbind.StatusPending || sess.ID == "" {
		t.Fatalf("session = %+v, want a fresh pending session", sess)
	}
	if sess.QRContent == "" {
		t.Fatal("pending session must carry the qr content")
	}
	if sess.ExpiresAt.Before(time.Now()) {
		t.Fatalf("expires_at = %v, want in the future", sess.ExpiresAt)
	}

	sess = waitForStatus(t, s, "device-1", sess.ID, qrbind.StatusSuccess)
	if sess.Error != "" {
		t.Fatalf("success session carries error %q", sess.Error)
	}
	if got := sess.Config["bot_id"]; got != "AIBOT123" {
		t.Fatalf("config bot_id = %q, want AIBOT123", got)
	}
	if got := sess.Config["bot_secret"]; got != "s3cret" {
		t.Fatalf("config bot_secret = %q, want s3cret", got)
	}

	// 扫码拿到的字段必须正好是平台 Schema 要求的键（两端漂移会在这里红）。
	desc, ok := platform.Get(platform.WeCom)
	if !ok {
		t.Fatal("wecom descriptor missing")
	}
	if err := desc.Validate(sess.Config); err != nil {
		t.Fatalf("schema rejects the scanned credentials: %v", err)
	}

	// 第一次请求是 generate（source/plat），之后都是带 scode 的查询。
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.generateCalls != 1 {
		t.Fatalf("generate calls = %d, want 1", f.generateCalls)
	}
	first := f.requests[0]
	if first.Get("source") != defaultSource || first.Get("plat") != "0" {
		t.Fatalf("generate query = %v, want source=%s plat=0", first, defaultSource)
	}
	for _, q := range f.requests[1:] {
		if q.Get("scode") != "sc-1" {
			t.Fatalf("query request = %v, want scode=sc-1", q)
		}
	}
}

func TestStartGenerateErrors(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantContains string
	}{
		{
			name:         "errcode from wecom",
			body:         `{"errcode":40001,"errmsg":"invalid source"}`,
			wantContains: "invalid source",
		},
		{
			name:         "missing auth url",
			body:         `{"errcode":0,"data":{"scode":"sc-1"}}`,
			wantContains: "missing scode or auth_url",
		},
		{
			name:         "gateway error",
			status:       http.StatusBadGateway,
			body:         "bad gateway",
			wantContains: "unexpected status 502",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeQC{generateStatus: tc.status, generateBody: tc.body}
			s := newTestSessions(t, f.start(t))

			if _, err := s.Start("device-1"); err == nil || !strings.Contains(err.Error(), tc.wantContains) {
				t.Fatalf("Start() error = %v, want it to contain %q", err, tc.wantContains)
			}
			// 生成失败不留下会话。
			if _, err := s.Get("device-1", "whatever"); !errors.Is(err, qrbind.ErrSessionNotFound) {
				t.Fatalf("Get() after a failed start = %v, want ErrSessionNotFound", err)
			}
		})
	}
}

func TestQueryErrorsMapToSessionStates(t *testing.T) {
	cases := []struct {
		name        string
		queryBody   string
		queryStatus int
		want        qrbind.Status
		wantErr     string
	}{
		{
			name:      "errcode fails the session",
			queryBody: `{"errcode":50001,"errmsg":"no permission"}`,
			want:      qrbind.StatusFailed,
			wantErr:   "no permission",
		},
		{
			name:      "expired",
			queryBody: `{"errcode":0,"data":{"status":"expired"}}`,
			want:      qrbind.StatusExpired,
			wantErr:   expiredMessage,
		},
		{
			name:      "success without credentials",
			queryBody: `{"errcode":0,"data":{"status":"success","bot_info":{}}}`,
			want:      qrbind.StatusFailed,
			wantErr:   missingCredentialMessage,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeQC{
				generateBody: `{"errcode":0,"data":{"scode":"sc-1","auth_url":"https://work.weixin.qq.com/ai/qc/auth?scode=sc-1"}}`,
				queryBodies:  []string{tc.queryBody},
			}
			s := newTestSessions(t, f.start(t))

			sess, err := s.Start("device-1")
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			sess = waitForStatus(t, s, "device-1", sess.ID, tc.want)
			if sess.Error != tc.wantErr {
				t.Fatalf("session error = %q, want %q", sess.Error, tc.wantErr)
			}
			if tc.want == qrbind.StatusFailed && sess.Config != nil {
				t.Fatalf("failed session carries config %v", sess.Config)
			}
		})
	}
}

// 网络抖动（HTTP 5xx）不改写会话状态：继续轮询，直到成功。
func TestQueryTransientFailureKeepsPolling(t *testing.T) {
	f := &fakeQC{
		generateBody: `{"errcode":0,"data":{"scode":"sc-1","auth_url":"https://work.weixin.qq.com/ai/qc/auth?scode=sc-1"}}`,
		queryBodies: []string{
			`{"errcode":0,"data":{"status":"success","bot_info":{"botid":"AIBOT1","secret":"s3cret"}}}`,
		},
		queryStatus: http.StatusInternalServerError,
	}
	s := newTestSessions(t, f.start(t))

	sess, err := s.Start("device-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	// 先确认抖动期间会话没有被打成 failed。
	time.Sleep(30 * time.Millisecond)
	current, err := s.Get("device-1", sess.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if current.Status != qrbind.StatusPending && current.Status != qrbind.StatusScanned {
		t.Fatalf("status during transient failure = %s, want pending", current.Status)
	}

	// 网关恢复后应当完成。
	f.mu.Lock()
	f.queryStatus = http.StatusOK
	f.mu.Unlock()
	sess = waitForStatus(t, s, "device-1", sess.ID, qrbind.StatusSuccess)
	if sess.Config["bot_id"] != "AIBOT1" {
		t.Fatalf("config = %v, want the bot from the recovered query", sess.Config)
	}
}

// 本地 TTL：没人扫码时到点即 expired，不再查询。
func TestSessionExpiresAtTTL(t *testing.T) {
	f := &fakeQC{
		generateBody: `{"errcode":0,"data":{"scode":"sc-1","auth_url":"https://work.weixin.qq.com/ai/qc/auth?scode=sc-1"}}`,
	}
	s := New(context.Background(), Options{
		BaseURL:              f.start(t),
		PollInterval:         5 * time.Millisecond,
		SessionTTL:           40 * time.Millisecond,
		RetentionAfterExpiry: 20 * time.Millisecond,
		Timeout:              time.Second,
	})
	t.Cleanup(s.Close)

	sess, err := s.Start("device-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	sess = waitForStatus(t, s, "device-1", sess.ID, qrbind.StatusExpired)
	if sess.Error != expiredMessage {
		t.Fatalf("session error = %q, want %q", sess.Error, expiredMessage)
	}

	// 清理之后会话不再存在。
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := s.Get("device-1", sess.ID); errors.Is(err, qrbind.ErrSessionNotFound) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("expired session was not cleaned up")
}

func TestStartReplacesActiveSession(t *testing.T) {
	f := &fakeQC{
		generateBody: `{"errcode":0,"data":{"scode":"sc-1","auth_url":"https://work.weixin.qq.com/ai/qc/auth?scode=sc-1"}}`,
	}
	s := newTestSessions(t, f.start(t))

	first, err := s.Start("device-1")
	if err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	second, err := s.Start("device-1")
	if err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("second Start() reused the session id")
	}
	if _, err := s.Get("device-1", first.ID); !errors.Is(err, qrbind.ErrSessionNotFound) {
		t.Fatalf("Get() on the replaced session = %v, want ErrSessionNotFound", err)
	}
	if _, err := s.Get("device-1", second.ID); err != nil {
		t.Fatalf("Get() on the new session error = %v", err)
	}
}

func TestCancelIsIdempotent(t *testing.T) {
	f := &fakeQC{
		generateBody: `{"errcode":0,"data":{"scode":"sc-1","auth_url":"https://work.weixin.qq.com/ai/qc/auth?scode=sc-1"}}`,
	}
	s := newTestSessions(t, f.start(t))

	sess, err := s.Start("device-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	s.Cancel("device-1", sess.ID)
	s.Cancel("device-1", sess.ID) // 幂等
	s.Cancel("device-1", "unknown")
	if _, err := s.Get("device-1", sess.ID); !errors.Is(err, qrbind.ErrSessionNotFound) {
		t.Fatalf("Get() after cancel = %v, want ErrSessionNotFound", err)
	}
}

func TestStartAfterCloseFails(t *testing.T) {
	f := &fakeQC{
		generateBody: `{"errcode":0,"data":{"scode":"sc-1","auth_url":"https://work.weixin.qq.com/ai/qc/auth?scode=sc-1"}}`,
	}
	s := newTestSessions(t, f.start(t))
	s.Close()
	if _, err := s.Start("device-1"); err == nil {
		t.Fatal("Start() after Close() = nil error, want failure")
	}
}
