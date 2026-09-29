// Package qrlogin 实现企业微信智能机器人的「扫码开通」：调 /ai/qc/generate 生成
// 二维码，轮询 /ai/qc/query_result 取回 Bot ID 与 Secret。
//
// 这两个端点不是企业微信开发者中心的公开 API，而是其帮助页「扫码关联 OpenClaw」
// 背后的网页接口（调研见 docs/wecom-qr-onboarding-design.md §C）。它们随时可能
// 变更或限流，兜底是控制台的手动填写；协议细节（参数、状态串、轮询参数）只允许
// 出现在这个包里，便于整体替换或下线。
package qrlogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/channels/qrbind"
	"github.com/liuscraft/orion-x/internal/logging"
)

const (
	defaultBaseURL = "https://work.weixin.qq.com"
	defaultSource  = "orion-x"

	generatePath = "/ai/qc/generate"
	queryPath    = "/ai/qc/query_result"

	// 轮询间隔与有效期对齐开源实现（LangBot / PicoClaw 都是 3s 一次、5 分钟窗口）。
	// 企微没有公布二维码有效期，5 分钟是本地判断（见设计文档 R1 待验证假设）。
	defaultPollInterval = 3 * time.Second
	defaultSessionTTL   = 5 * time.Minute
	defaultTimeout      = 15 * time.Second
	// defaultRetention 是终态（含过期）后继续保留会话的时长：控制台轮询间隔是秒级，
	// 留出一段让它把结果取走；到期后会话被清理。
	defaultRetention = 30 * time.Second

	// expiredMessage 是二维码过期时的默认提示。
	expiredMessage = "二维码已过期，请重新生成"
	// missingCredentialMessage 表示平台回了成功但凭证不完整。
	missingCredentialMessage = "企业微信返回的凭证不完整，请重新生成"
)

// Options 组装一个 Sessions；零值可用。
type Options struct {
	// BaseURL 覆盖企微端点根地址（私有代理 / 联调），空 = https://work.weixin.qq.com。
	BaseURL string
	// Source 是扫码会话的接入方标识，空 = orion-x。
	Source string
	// Platform 是请求里的 plat 参数。语义未公开：LangBot 的 Web 流程用 0，
	// PicoClaw 按操作系统给 1/2/3；这里是服务端调用，跟随 LangBot 用 0。
	Platform int
	// HTTPClient 允许调用方复用连接池；nil = 按 Timeout 新建。
	HTTPClient *http.Client
	// PollInterval 是轮询间隔，0 = 3s。
	PollInterval time.Duration
	// SessionTTL 是会话有效期，0 = 5 分钟。
	SessionTTL time.Duration
	// RetentionAfterExpiry 是终态（含过期）后继续保留会话的时长，0 = 30s。
	RetentionAfterExpiry time.Duration
	// Timeout 是单次出网超时，0 = 15s。
	Timeout time.Duration
	// Now 供测试注入时钟；nil = time.Now。
	Now func() time.Time
}

func (o Options) withDefaults() Options {
	if strings.TrimSpace(o.BaseURL) == "" {
		o.BaseURL = defaultBaseURL
	}
	if strings.TrimSpace(o.Source) == "" {
		o.Source = defaultSource
	}
	if o.PollInterval <= 0 {
		o.PollInterval = defaultPollInterval
	}
	if o.SessionTTL <= 0 {
		o.SessionTTL = defaultSessionTTL
	}
	if o.RetentionAfterExpiry <= 0 {
		o.RetentionAfterExpiry = defaultRetention
	}
	if o.Timeout <= 0 {
		o.Timeout = defaultTimeout
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	o.BaseURL = strings.TrimRight(o.BaseURL, "/")
	return o
}

// Sessions 持有本进程的全部扫码会话，并负责后台轮询。
//
// 单设备同一时刻只有一个活跃会话（Start 会替换旧的）；会话在 TTL 后被清理，
// 成功态也保留到 TTL，让控制台来得及取走凭证。
type Sessions struct {
	ctx  context.Context
	opts Options

	mu       sync.Mutex
	byDevice map[string]*session
	closed   bool
}

// New 创建会话管理器；ctx 是全部后台轮询的父上下文（manager 用 worker 上下文）。
func New(ctx context.Context, opts Options) *Sessions {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Sessions{
		ctx:      ctx,
		opts:     opts.withDefaults(),
		byDevice: make(map[string]*session),
	}
}

// session 是一次扫码会话的内部状态；状态字段由 poll goroutine 写、Get 读。
type session struct {
	id        string
	deviceID  string
	scode     string
	qrContent string
	expiresAt time.Time
	cancel    context.CancelFunc

	mu     sync.Mutex
	status qrbind.Status
	errMsg string
	config map[string]string
}

func (s *session) snapshot() qrbind.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := qrbind.Session{
		ID:        s.id,
		Status:    s.status,
		QRContent: s.qrContent,
		ExpiresAt: s.expiresAt,
		Error:     s.errMsg,
	}
	if s.status == qrbind.StatusSuccess {
		out.Config = make(map[string]string, len(s.config))
		for k, v := range s.config {
			out.Config[k] = v
		}
	}
	return out
}

func (s *session) isTerminal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status == qrbind.StatusSuccess || s.status == qrbind.StatusExpired || s.status == qrbind.StatusFailed
}

// finish 把会话推进到终态；已经终态的会话不会被覆盖。
func (s *session) finish(status qrbind.Status, errMsg string, config map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == qrbind.StatusSuccess || s.status == qrbind.StatusExpired || s.status == qrbind.StatusFailed {
		return
	}
	s.status = status
	s.errMsg = errMsg
	s.config = config
}

// markScanned 记录「已扫码」，不覆盖终态。
func (s *session) markScanned() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == qrbind.StatusPending {
		s.status = qrbind.StatusScanned
	}
}

// Start 实现 qrbind.Binder：生成二维码并启动后台轮询。
func (s *Sessions) Start(deviceID string) (qrbind.Session, error) {
	if strings.TrimSpace(deviceID) == "" {
		return qrbind.Session{}, errors.New("qrlogin: device id is required")
	}

	genCtx, cancel := context.WithTimeout(s.ctx, s.opts.Timeout)
	scode, authURL, err := s.generate(genCtx)
	cancel()
	if err != nil {
		return qrbind.Session{}, err
	}

	sessCtx, sessCancel := context.WithCancel(s.ctx)
	sess := &session{
		id:        uuid.NewString(),
		deviceID:  deviceID,
		scode:     scode,
		qrContent: authURL,
		expiresAt: s.opts.Now().Add(s.opts.SessionTTL),
		cancel:    sessCancel,
		status:    qrbind.StatusPending,
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		sessCancel()
		return qrbind.Session{}, errors.New("qrlogin: sessions closed")
	}
	old := s.byDevice[deviceID]
	s.byDevice[deviceID] = sess
	s.mu.Unlock()

	if old != nil {
		logging.Infof("qrlogin: replacing active session for device %s", deviceID)
		old.cancel()
	}
	go s.poll(sessCtx, sess)
	return sess.snapshot(), nil
}

// Get 实现 qrbind.Binder。
func (s *Sessions) Get(deviceID, sessionID string) (qrbind.Session, error) {
	s.mu.Lock()
	sess := s.byDevice[deviceID]
	s.mu.Unlock()
	if sess == nil || sess.id != sessionID {
		return qrbind.Session{}, qrbind.ErrSessionNotFound
	}
	return sess.snapshot(), nil
}

// Cancel 实现 qrbind.Binder。
func (s *Sessions) Cancel(deviceID, sessionID string) {
	s.mu.Lock()
	sess := s.byDevice[deviceID]
	if sess == nil || sess.id != sessionID {
		s.mu.Unlock()
		return
	}
	delete(s.byDevice, deviceID)
	s.mu.Unlock()
	sess.cancel()
}

// Close 取消并清理所有会话；幂等。
func (s *Sessions) Close() {
	s.mu.Lock()
	s.closed = true
	sessions := make([]*session, 0, len(s.byDevice))
	for _, sess := range s.byDevice {
		sessions = append(sessions, sess)
	}
	s.byDevice = make(map[string]*session)
	s.mu.Unlock()
	for _, sess := range sessions {
		sess.cancel()
	}
}

// drop 删除会话，但仅当它还指向自己（Start 替换过就留给新会话）。
func (s *Sessions) drop(sess *session) {
	s.mu.Lock()
	if s.byDevice[sess.deviceID] == sess {
		delete(s.byDevice, sess.deviceID)
	}
	s.mu.Unlock()
}

// poll 每 PollInterval 查询一次结果，直到终态或 TTL；终态（含过期）后再保留
// RetentionAfterExpiry，让控制台取走结果，然后清理会话。
func (s *Sessions) poll(ctx context.Context, sess *session) {
	defer s.drop(sess)

	retention := sess.expiresAt.Add(s.opts.RetentionAfterExpiry)
	ticker := time.NewTicker(s.opts.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		now := s.opts.Now()
		// TTL 到：未完成的标记过期；终态保留到 retention 结束再清理。
		if !now.Before(sess.expiresAt) {
			sess.finish(qrbind.StatusExpired, expiredMessage, nil)
		}
		if !now.Before(retention) {
			return
		}
		if sess.isTerminal() {
			continue
		}

		result, err := s.query(ctx, sess.scode)
		switch {
		case err == nil:
			s.apply(sess, result)
		case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
			// 会话被取消或父上下文结束：交回上层清理。
			if ctx.Err() != nil {
				return
			}
			logging.Warnf("qrlogin: query for device %s timed out, will retry", sess.deviceID)
		default:
			var apiErr *apiError
			if errors.As(err, &apiErr) {
				sess.finish(qrbind.StatusFailed, apiErr.userMessage(), nil)
				logging.Warnf("qrlogin: device %s session failed: %v", sess.deviceID, apiErr)
				continue
			}
			// 网络抖动不改写会话状态，继续轮询到 TTL。
			logging.Warnf("qrlogin: query for device %s failed, will retry: %v", sess.deviceID, err)
		}
	}
}

// apply 把一次查询结果映射到会话状态。
func (s *Sessions) apply(sess *session, result queryResult) {
	switch strings.ToLower(result.Status) {
	case "success":
		if result.BotInfo.BotID == "" || result.BotInfo.Secret == "" {
			sess.finish(qrbind.StatusFailed, missingCredentialMessage, nil)
			return
		}
		sess.finish(qrbind.StatusSuccess, "", map[string]string{
			"bot_id":     result.BotInfo.BotID,
			"bot_secret": result.BotInfo.Secret,
		})
		logging.Infof("qrlogin: device %s bound bot %s", sess.deviceID, result.BotInfo.BotID)
	case "expired":
		sess.finish(qrbind.StatusExpired, expiredMessage, nil)
	case "scaned", "scanned":
		sess.markScanned()
	default:
		// pending / waiting / 未知状态：保持现状，继续轮询。
	}
}

// generate 请求二维码：GET /ai/qc/generate?source=..&plat=..
func (s *Sessions) generate(ctx context.Context) (scode, authURL string, err error) {
	// PicoClaw 还额外带 sourceID（与 source 同值）；LangBot 只带 source + plat，
	// 这里跟随后者——参数面越小，端点变动时越稳。
	query := url.Values{}
	query.Set("source", s.opts.Source)
	query.Set("plat", strconv.Itoa(s.opts.Platform))

	var resp generateResponse
	if err := s.getJSON(ctx, s.opts.BaseURL+generatePath+"?"+query.Encode(), &resp); err != nil {
		return "", "", fmt.Errorf("qrlogin: generate qr code: %w", err)
	}
	if resp.ErrCode != 0 {
		return "", "", fmt.Errorf("qrlogin: generate qr code: %w", &apiError{code: resp.ErrCode, msg: resp.ErrMsg})
	}
	if resp.Data.SCode == "" || resp.Data.AuthURL == "" {
		return "", "", errors.New("qrlogin: generate qr code: response missing scode or auth_url")
	}
	return resp.Data.SCode, resp.Data.AuthURL, nil
}

// query 查询一次扫码结果：GET /ai/qc/query_result?scode=..
func (s *Sessions) query(ctx context.Context, scode string) (queryResult, error) {
	query := url.Values{}
	query.Set("scode", scode)

	reqCtx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()

	var resp queryResponse
	if err := s.getJSON(reqCtx, s.opts.BaseURL+queryPath+"?"+query.Encode(), &resp); err != nil {
		return queryResult{}, fmt.Errorf("qrlogin: query qr result: %w", err)
	}
	if resp.ErrCode != 0 {
		return queryResult{}, fmt.Errorf("qrlogin: query qr result: %w", &apiError{code: resp.ErrCode, msg: resp.ErrMsg})
	}
	return resp.Data, nil
}

func (s *Sessions) getJSON(ctx context.Context, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (s *Sessions) httpClient() *http.Client {
	if s.opts.HTTPClient != nil {
		return s.opts.HTTPClient
	}
	return &http.Client{Timeout: s.opts.Timeout}
}

// apiError 是企微返回非零 errcode 的错误；msg 是可直接给用户看的原因。
type apiError struct {
	code int
	msg  string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("errcode=%d errmsg=%s", e.code, e.msg)
}

func (e *apiError) userMessage() string {
	if strings.TrimSpace(e.msg) != "" {
		return e.msg
	}
	return fmt.Sprintf("企业微信返回错误码 %d，请改用手动配置或稍后重试", e.code)
}

// generateResponse 是 /ai/qc/generate 的响应。
type generateResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
	Data    struct {
		SCode   string `json:"scode"`
		AuthURL string `json:"auth_url"`
	} `json:"data"`
}

// queryResponse 是 /ai/qc/query_result 的响应。
type queryResponse struct {
	ErrCode int         `json:"errcode"`
	ErrMsg  string      `json:"errmsg"`
	Data    queryResult `json:"data"`
}

// queryResult 是查询结果里的业务字段。
type queryResult struct {
	Status  string `json:"status"`
	BotInfo struct {
		BotID  string `json:"botid"`
		Secret string `json:"secret"`
	} `json:"bot_info"`
}
