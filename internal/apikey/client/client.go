// Package client 是数据面（wsserver）访问控制面握手校验接口的 HTTP 客户端。
//
// 它只做一件事：把 key + device_id 发给 manager 的 /internal/apikey/verify，并把
// 200 的响应原样 decode 成 apikey.VerifyResponse。不做任何判定、不缓存、不重试：
// 校验只发生在控制面（docs/wsserver-apikey-auth-design.md §1 D2）。
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/logging"
)

const (
	// defaultTimeout 是 verify 的端到端超时默认值。它卡在设备等 hello 的关键
	// 路径上，不能按普通请求的 3s 慢慢等（§2 关键约束：唯一新增阻塞点 ≤800ms）。
	defaultTimeout = 800 * time.Millisecond
	// errorBodySnippetBytes 是错误信息里保留的响应体上限：错误日志里不带全量 body。
	errorBodySnippetBytes = 512
)

// ErrUnauthorized 表示控制面拒绝了内部鉴权（HTTP 401）。调用方用 errors.Is 判断：
// 它和“网络不通 / 控制面 5xx / 端点不存在”不是一回事——前者是 manager.token
// 配错或没配，重试没有意义。
var ErrUnauthorized = errors.New("apikey/client: unauthorized")

// Config 是 Client 的配置。
type Config struct {
	// BaseURL 是 manager 的 base URL，如 http://127.0.0.1:9090。
	BaseURL string
	// Token 是内部鉴权 Bearer token；为空时不发 Authorization 头，构造时会告警。
	Token string
	// Timeout 是单请求超时，<=0 时取 800ms。
	Timeout time.Duration
	// HTTPClient 用于测试注入；为 nil 时按 Timeout 自建。
	HTTPClient *http.Client
}

// Client 是控制面握手校验接口的 HTTP 客户端。
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	timeout    time.Duration
}

// New 构造一个 Client。缺 token / 缺 BaseURL 只告警不报错：鉴权本身是 fail closed
// 的，配置问题会在第一次校验时以传输错误暴露，由调用方决定怎么拒绝连接。
func New(cfg Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		logging.Warnf("apikey/client: base url is empty, every handshake verify will fail")
	}
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		logging.Warnf("apikey/client: internal token is empty, control plane will reject every handshake verify")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: httpClient,
		timeout:    timeout,
	}
}

// Verify 调用 POST /internal/apikey/verify。控制面明确拒绝时同样返回 HTTP 200，
// 只是 allowed=false：reject_reason 是业务判断不是传输错误，所以这里原样返回、
// err 为 nil，由调用方决定怎么拒绝连接（§3.1 错误语义）。
func (c *Client) Verify(ctx context.Context, rawKey, deviceID string) (apikey.VerifyResponse, error) {
	var resp apikey.VerifyResponse
	req := apikey.VerifyRequest{Key: rawKey, DeviceID: deviceID}
	if err := c.post(ctx, apikey.PathVerify, req, &resp); err != nil {
		return apikey.VerifyResponse{}, err
	}
	return resp, nil
}

// post 发一个 JSON 请求，并把 2xx 的响应解到 out。超时由 context 控制：单请求
// 超时与调用方 ctx 取更早的那个。
func (c *Client) post(ctx context.Context, path string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("apikey/client: encode %s request: %w", path, err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("apikey/client: build %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("apikey/client: %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= 300 {
		return statusError(path, resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("apikey/client: %s: decode response: %w", path, err)
	}
	return nil
}

// statusError 把非 2xx 变成一个带状态码和响应体片段的错误。401 额外包上
// ErrUnauthorized，调用方据此区分“配错了”和“控制面挂了”。
func statusError(path string, resp *http.Response) error {
	snippet := bodySnippet(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		logging.Warnf("apikey/client: %s: unauthorized, check the internal token: %s", path, snippet)
		return fmt.Errorf("%w: %s: status %d: %s", ErrUnauthorized, path, resp.StatusCode, snippet)
	}
	logging.Warnf("apikey/client: %s: unexpected status %d: %s", path, resp.StatusCode, snippet)
	return fmt.Errorf("apikey/client: %s: status %d: %s", path, resp.StatusCode, snippet)
}

// bodySnippet 读一段有界的响应体，用于错误信息。
func bodySnippet(body io.Reader) string {
	buf, err := io.ReadAll(io.LimitReader(body, errorBodySnippetBytes))
	if err != nil {
		return fmt.Sprintf("<unreadable: %v>", err)
	}
	return strings.TrimSpace(string(buf))
}
