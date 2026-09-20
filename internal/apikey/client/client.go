// Package client 是数据面（wsserver）访问控制面接入鉴权接口的 HTTP 客户端。
//
// 与计费一样分层：wire 类型与判定住在零依赖的 internal/apikey，控制面实现住在
// cmd/manager/handler，这里只负责把一次握手变成一次同步的 HTTP 调用。
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
	// defaultTimeout 是单请求超时。它卡在设备等 hello 的关键路径上，不能慢慢等。
	defaultTimeout = time.Second
	// errorBodySnippetBytes 是错误信息里保留的响应体上限。
	errorBodySnippetBytes = 256
)

// ErrUnauthorized 表示控制面拒绝了内部鉴权（HTTP 401）：token 没配或配错。
// 和“控制面不可达”不同，但调用方的处置是一样的——取不到结论就不放行。
var ErrUnauthorized = errors.New("apikey/client: unauthorized")

// Config 是 Client 的配置。
type Config struct {
	// BaseURL 是 manager 的 base URL，如 http://127.0.0.1:9090。
	BaseURL string
	// Token 是内部鉴权 Bearer token，必须与 manager 的 internal.token 一致。
	Token string
	// Timeout 是单请求超时，<=0 时取 1s。
	Timeout time.Duration
	// HTTPClient 用于测试注入；为 nil 时按 Timeout 自建。
	HTTPClient *http.Client
}

// Client 是接入鉴权接口的 HTTP 客户端。
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// New 构造一个 Client。缺 token / 缺 BaseURL 只告警不报错：设备握手时才会失败，
// 那时错误日志里已经带上了原因。
func New(cfg Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		logging.Warnf("apikey/client: base url is empty, every api key check will fail")
	}
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		logging.Warnf("apikey/client: internal token is empty, control plane will reject every api key check")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{baseURL: baseURL, token: token, httpClient: httpClient}
}

// Authorize 校验一次接入。第二个返回值只表示“这次调用没走通”（网络、状态码、
// 解析失败）；密钥被拒是正常结果，放在响应里（allowed=false + reject_reason）。
func (c *Client) Authorize(ctx context.Context, key, deviceID string) (apikey.AuthorizeResponse, error) {
	body, err := json.Marshal(apikey.AuthorizeRequest{Key: key, DeviceID: deviceID})
	if err != nil {
		return apikey.AuthorizeResponse{}, fmt.Errorf("apikey/client: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+apikey.PathAuthorize, bytes.NewReader(body))
	if err != nil {
		return apikey.AuthorizeResponse{}, fmt.Errorf("apikey/client: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return apikey.AuthorizeResponse{}, fmt.Errorf("apikey/client: POST %s: %w", apikey.PathAuthorize, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return apikey.AuthorizeResponse{}, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodySnippetBytes))
		return apikey.AuthorizeResponse{}, fmt.Errorf("apikey/client: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var out apikey.AuthorizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return apikey.AuthorizeResponse{}, fmt.Errorf("apikey/client: decode response: %w", err)
	}
	// 回 200 但没有 allowed 也没有拒绝原因是不合法的响应：宁可拒绝。
	if !out.Allowed && out.RejectReason == apikey.RejectNone {
		return apikey.AuthorizeResponse{}, errors.New("apikey/client: response has no decision")
	}
	return out, nil
}
