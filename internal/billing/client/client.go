// Package client 是数据面（wsserver）访问控制面计费接口的 HTTP 客户端与 Sink 实现。
//
// 它是仓库里唯一 import net/http 的计费包：纯领域层住在 internal/billing，控制面
// 实现住在 internal/billing/service（docs/billing-design.md §19）。数据面拿到的是
// wire DTO 加 port 接口，实现是注入的。
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

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/logging"
)

const (
	// defaultTimeout 是单个请求的超时默认值。
	defaultTimeout = 3 * time.Second
	// defaultAuthorizeTimeout 是 authorize 的超时默认值。它卡在设备等 hello 的
	// 关键路径上，不能按普通请求的 3s 慢慢等（§14.3）。
	defaultAuthorizeTimeout = 800 * time.Millisecond
	// errorBodySnippetBytes 是错误信息里保留的响应体上限：错误日志里不带全量 body。
	errorBodySnippetBytes = 512
)

// ErrUnauthorized 表示控制面拒绝了内部鉴权（HTTP 401）。调用方用 errors.Is 判断：
// 它和“网络不通 / 控制面 5xx”不是一回事——前者是配置错误（token 没配或配错），
// 重试没有意义。
var ErrUnauthorized = errors.New("billing/client: unauthorized")

// Config 是 Client 的配置。
type Config struct {
	// BaseURL 是 manager 的 base URL，如 http://127.0.0.1:9090。
	BaseURL string
	// Token 是内部鉴权 Bearer token；为空时不发 Authorization 头，构造时会告警。
	Token string
	// Timeout 是单请求超时，<=0 时取 3s。
	Timeout time.Duration
	// AuthorizeTimeout 是 authorize 的单请求超时，<=0 时取 800ms。
	AuthorizeTimeout time.Duration
	// HTTPClient 用于测试注入；为 nil 时按 Timeout 自建。
	HTTPClient *http.Client
}

// Client 是控制面计费接口的 HTTP 客户端。
type Client struct {
	baseURL          string
	token            string
	httpClient       *http.Client
	authorizeTimeout time.Duration
}

// New 构造一个 Client。缺 token / 缺 BaseURL 只告警不报错：计费是旁路，配置问题
// 不该把进程按死在这里，失败方向是“少收钱”，是安全的那一侧。
func New(cfg Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		logging.Warnf("billing/client: base url is empty, every billing request will fail")
	}
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		logging.Warnf("billing/client: internal token is empty, control plane will reject every billing request")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	authorizeTimeout := cfg.AuthorizeTimeout
	if authorizeTimeout <= 0 {
		authorizeTimeout = defaultAuthorizeTimeout
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		baseURL:          baseURL,
		token:            token,
		httpClient:       httpClient,
		authorizeTimeout: authorizeTimeout,
	}
}

// Authorize 调用 POST /internal/billing/authorize，准入请求只带事实、不带账户
// （§14.2）。被拒时控制面同样返回 HTTP 200，只是 allowed=false：reject_reason
// 是业务判断不是传输错误，所以这里原样返回、err 为 nil，由调用方按 reject_reason
// 决定怎么拒绝连接（§15.2）。
func (c *Client) Authorize(ctx context.Context, req billing.AuthorizeRequest) (billing.AuthorizeResponse, error) {
	var resp billing.AuthorizeResponse
	if err := c.post(ctx, billing.PathAuthorize, c.authorizeTimeout, req, &resp); err != nil {
		return billing.AuthorizeResponse{}, err
	}
	return resp, nil
}

// ReportUsage 调用 POST /internal/billing/usage-events。控制面落库即返回、不做定价，
// 响应里的 rejected / duplicated 由 Sink 处理（§14.4）。
func (c *Client) ReportUsage(ctx context.Context, req billing.UsageBatchRequest) (billing.UsageBatchResponse, error) {
	var resp billing.UsageBatchResponse
	if err := c.post(ctx, billing.PathUsageEvents, 0, req, &resp); err != nil {
		return billing.UsageBatchResponse{}, err
	}
	return resp, nil
}

// Settle 调用 POST /internal/billing/settle，交出会话的账目（§14.5）。幂等键是
// session_id，重复调用拿回第一次的结果。
func (c *Client) Settle(ctx context.Context, req billing.SettleRequest) (billing.SettleResponse, error) {
	var resp billing.SettleResponse
	if err := c.post(ctx, billing.PathSettle, 0, req, &resp); err != nil {
		return billing.SettleResponse{}, err
	}
	return resp, nil
}

// post 发一个 JSON 请求，并把 2xx 的响应解到 out。timeout <= 0 表示只用调用方 ctx
// 的期限。
func (c *Client) post(ctx context.Context, path string, timeout time.Duration, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("billing/client: encode %s request: %w", path, err)
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("billing/client: build %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("billing/client: %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= 300 {
		return statusError(path, resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("billing/client: %s: decode response: %w", path, err)
	}
	return nil
}

// statusError 把非 2xx 变成一个带状态码和响应体片段的错误。401 额外包上
// ErrUnauthorized，调用方据此区分“配错了”和“控制面挂了”。
func statusError(path string, resp *http.Response) error {
	snippet := bodySnippet(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		logging.Warnf("billing/client: %s: unauthorized, check the internal token: %s", path, snippet)
		return fmt.Errorf("%w: %s: status %d: %s", ErrUnauthorized, path, resp.StatusCode, snippet)
	}
	logging.Warnf("billing/client: %s: unexpected status %d: %s", path, resp.StatusCode, snippet)
	return fmt.Errorf("billing/client: %s: status %d: %s", path, resp.StatusCode, snippet)
}

// bodySnippet 读一段有界的响应体，用于错误信息。
func bodySnippet(body io.Reader) string {
	buf, err := io.ReadAll(io.LimitReader(body, errorBodySnippetBytes))
	if err != nil {
		return fmt.Sprintf("<unreadable: %v>", err)
	}
	return strings.TrimSpace(string(buf))
}
