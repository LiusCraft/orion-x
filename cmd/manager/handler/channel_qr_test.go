package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/channels/platform"
	"github.com/liuscraft/orion-x/internal/channels/qrbind"
	"github.com/liuscraft/orion-x/internal/store"
)

// fakeQRBinder 是 qrbind.Binder 的内联替身（仓库约定：不引 mock 生成器）。
type fakeQRBinder struct {
	sessions map[string]qrbind.Session
	started  int
	canceled []string
	startErr error
}

func newFakeQRBinder() *fakeQRBinder {
	return &fakeQRBinder{sessions: map[string]qrbind.Session{}}
}

func (b *fakeQRBinder) Start(string) (qrbind.Session, error) {
	b.started++
	if b.startErr != nil {
		return qrbind.Session{}, b.startErr
	}
	sess := qrbind.Session{
		ID:        "sess-1",
		Status:    qrbind.StatusPending,
		QRContent: "https://work.weixin.qq.com/ai/qc/auth?scode=abc",
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	b.sessions[sess.ID] = sess
	return sess, nil
}

func (b *fakeQRBinder) Get(_, sessionID string) (qrbind.Session, error) {
	sess, ok := b.sessions[sessionID]
	if !ok {
		return qrbind.Session{}, qrbind.ErrSessionNotFound
	}
	return sess, nil
}

func (b *fakeQRBinder) Cancel(_, sessionID string) { b.canceled = append(b.canceled, sessionID) }

// fakeChannelStore 记录通道配置的写入，避免单测连库。
type fakeChannelStore struct {
	upserts map[string]map[string]string
	deletes []string
}

func newFakeChannelStore() *fakeChannelStore {
	return &fakeChannelStore{upserts: map[string]map[string]string{}}
}

func (s *fakeChannelStore) Upsert(deviceID, platformName string, config map[string]string) error {
	s.upserts[deviceID+":"+platformName] = config
	return nil
}

func (s *fakeChannelStore) Delete(deviceID, platformName string) error {
	s.deletes = append(s.deletes, deviceID+":"+platformName)
	return nil
}

// qrHandlerRequest 组装一个只带路径参数的 gin 上下文；这些用例不走中间件与库。
func qrHandlerRequest(params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = params
	return c, w
}

// TestListPlatformsGatesQRBinding：扫码入口由「注册表声明 + 运行时装配」共同决定。
func TestListPlatformsGatesQRBinding(t *testing.T) {
	cases := []struct {
		name      string
		binders   map[string]qrbind.Binder
		wantWeCom bool
	}{
		{name: "no binder hides qr", binders: nil, wantWeCom: false},
		{name: "binder enables qr", binders: map[string]qrbind.Binder{platform.WeCom: newFakeQRBinder()}, wantWeCom: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewChannelHandler(nil, nil, nil, tc.binders)
			c, w := qrHandlerRequest(nil)
			h.ListPlatforms(c)
			if w.Code != http.StatusOK {
				t.Fatalf("ListPlatforms = %d, want 200", w.Code)
			}

			var found bool
			for _, desc := range decodePlatforms(t, w) {
				switch desc.Name {
				case platform.WeCom:
					found = true
					if desc.QRBinding != tc.wantWeCom {
						t.Errorf("wecom qr_binding = %v, want %v", desc.QRBinding, tc.wantWeCom)
					}
				case platform.Telegram:
					if desc.QRBinding {
						t.Error("telegram must not advertise qr_binding")
					}
				}
			}
			if !found {
				t.Fatal("wecom platform missing from GET /api/channels payload")
			}
		})
	}
}

func decodePlatforms(t *testing.T, w *httptest.ResponseRecorder) []platform.Descriptor {
	t.Helper()
	var descs []platform.Descriptor
	if err := json.Unmarshal(w.Body.Bytes(), &descs); err != nil {
		t.Fatalf("decode platforms: %v (body %s)", err, w.Body.String())
	}
	return descs
}

// TestChannelQRBinderGate：未知平台 404；不支持扫码或未装配 Binder 的平台 400。
func TestChannelQRBinderGate(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		binders  map[string]qrbind.Binder
		wantCode int
		wantOK   bool
	}{
		{name: "unknown platform", platform: "nope", binders: nil, wantCode: http.StatusNotFound},
		{name: "platform without qr", platform: platform.Telegram, binders: nil, wantCode: http.StatusBadRequest},
		{name: "qr disabled", platform: platform.WeCom, binders: nil, wantCode: http.StatusBadRequest},
		{
			name:     "wecom ready",
			platform: platform.WeCom,
			binders:  map[string]qrbind.Binder{platform.WeCom: newFakeQRBinder()},
			wantOK:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewChannelHandler(nil, nil, nil, tc.binders)
			c, w := qrHandlerRequest(gin.Params{{Key: "platform", Value: tc.platform}})
			_, _, ok := h.qrBinder(c)
			if ok != tc.wantOK {
				t.Fatalf("qrBinder ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK && w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.wantCode, w.Body.String())
			}
		})
	}
}

// TestBindQRSuccessMasksCredentials：写库用明文，回显只用掩码；Schema 之外的字段不写库。
func TestBindQRSuccessMasksCredentials(t *testing.T) {
	const secret = "s3cret-value-1234"
	sess := qrbind.Session{
		ID:     "sess-1",
		Status: qrbind.StatusSuccess,
		Config: map[string]string{"bot_id": "AIBOT12345678", "bot_secret": secret},
	}

	channels := newFakeChannelStore()
	h := NewChannelHandler(nil, nil, channels, nil)
	desc, _ := platform.Get(platform.WeCom)
	channel, err := h.bindQRSuccess(&store.Device{ID: "device-1"}, desc, sess)
	if err != nil {
		t.Fatalf("bindQRSuccess() error = %v", err)
	}
	if channel == nil || !channel.Enabled || channel.Platform != platform.WeCom {
		t.Fatalf("channel status = %+v, want enabled wecom", channel)
	}
	if got := channel.Config["bot_secret"]; got == secret || !strings.Contains(got, "...") {
		t.Fatalf("response secret = %q, want masked", got)
	}
	if got := channels.upserts["device-1:"+platform.WeCom]["bot_secret"]; got != secret {
		t.Fatalf("stored secret = %q, want plaintext %q", got, secret)
	}

	// Binder 返回 Schema 之外的键：拒绝写库，宁可让用户重新扫码。
	bad := qrbind.Session{Status: qrbind.StatusSuccess, Config: map[string]string{"bot_id": "AIBOT12345678", "extra": "x"}}
	if _, err := h.bindQRSuccess(&store.Device{ID: "device-1"}, desc, bad); err == nil {
		t.Fatal("bindQRSuccess() with an unknown field = nil error, want rejection")
	}
	if len(channels.upserts) != 1 {
		t.Fatalf("upserts = %d, want only the valid one", len(channels.upserts))
	}
}

// TestNewChannelQRSessionHidesConfig：会话响应带二维码与状态，但凭证永远不进响应。
func TestNewChannelQRSessionHidesConfig(t *testing.T) {
	desc, _ := platform.Get(platform.WeCom)
	resp := newChannelQRSession(desc, qrbind.Session{
		ID:        "sess-1",
		Status:    qrbind.StatusSuccess,
		ExpiresAt: time.Now(),
		Config:    map[string]string{"bot_secret": "s3cret-value-1234"},
	})
	if resp.SessionID != "sess-1" || resp.Status != string(qrbind.StatusSuccess) {
		t.Fatalf("response = %+v, want success session", resp)
	}
	if resp.Channel != nil {
		t.Fatalf("channel = %+v, want nil until the handler writes it", resp.Channel)
	}
	if resp.QRContent != "" {
		t.Fatalf("qr_content = %q, want empty for a finished session", resp.QRContent)
	}
}
