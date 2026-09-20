// Package apikey 是访问密钥的纯领域层：凭据格式、摘要、权限范围判定。
//
// 与 internal/billing 的领域层同构，零依赖（不碰数据库 / HTTP 框架 / 仓储），
// 由 .golangci.yml 的 apikey-domain 规则守住；落库在 internal/store，
// HTTP 出口在 cmd/manager 的 handler 与 middleware。
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Prefix 是自产凭据的固定前缀，按 AGENTS.md 的命名约定用 `:` 分段。
// 校验时靠它把 Bearer 头里的 JWT 和 API Key 分开。
const Prefix = "ox:sk:"

// secretSize 是明文的随机部分长度（24 字节 → 48 个十六进制字符）。
// 密钥是高熵随机串，用 SHA-256 直接摘要即可，不需要 bcrypt 这类慢哈希——
// 慢哈希是给低熵口令抗爆破用的，而这里每次请求都要校验一次。
const secretSize = 24

// displayPrefixSize 是明文中保留下来供列表页展示的前缀长度（不含 Prefix）。
const displayPrefixSize = 8

// ErrInvalid 表示凭据不符合自产格式。
var ErrInvalid = errors.New("apikey: invalid key format")

// ErrInvalidScope 表示权限范围取值不合法。
var ErrInvalidScope = errors.New("apikey: invalid scope")

// Generate 生成一个新的明文密钥。明文只在创建响应里出现一次，不落库。
func Generate() (string, error) {
	buf := make([]byte, secretSize)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("apikey: generate: %w", err)
	}
	return Prefix + hex.EncodeToString(buf), nil
}

// Hash 返回明文摘要（十六进制 SHA-256），库里只存它。
func Hash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// LooksLike 判断凭据是否是我们签发的 API Key。
func LooksLike(raw string) bool {
	return strings.HasPrefix(raw, Prefix)
}

// Display 从明文里拆出展示用的前缀与尾四位（两者都是明文片段，可以落库）。
func Display(plain string) (prefix, last4 string, err error) {
	if !LooksLike(plain) {
		return "", "", ErrInvalid
	}
	secret := strings.TrimPrefix(plain, Prefix)
	if len(secret) < displayPrefixSize+4 {
		return "", "", ErrInvalid
	}
	return Prefix + secret[:displayPrefixSize], secret[len(secret)-4:], nil
}

// Masked 把库里的展示片段拼成脱敏串，形如 ox:sk:8f2a1c3d...e9b4。
// 明文不可逆，所以列表接口只能给出这个形态。
func Masked(prefix, last4 string) string {
	if prefix == "" || last4 == "" {
		return ""
	}
	return prefix + "..." + last4
}

// Scope 是 API Key 的权限范围，取值同样按 `:` 分段：<域>:scope:<能力>。
//
// API Key 是程序化调用凭据，可达范围就是下面这几项；控制面能力（供应商密钥、
// 模型与音色、计费、密钥管理）不在其中——那些只有控制台会话能碰。
type Scope string

const (
	// ScopeAll 放行 API Key 可达的全部命名空间（区别于控制台管理员的“全部权限”）。
	ScopeAll Scope = "apikey:scope:all"
	// ScopeRead 只放行安全方法（GET/HEAD/OPTIONS），可达范围与 ScopeAll 相同。
	ScopeRead Scope = "apikey:scope:read"
	// ScopeAgent 智能体、设备与智能体模板。
	ScopeAgent Scope = "apikey:scope:agent"
	// ScopeMCP MCP 市场、私有 MCP 服务与工具调用。
	ScopeMCP Scope = "apikey:scope:mcp"
	// ScopeData 记忆、知识库与资源。
	ScopeData Scope = "apikey:scope:data"
	// ScopeVoice 接入 wsserver（小智 WebSocket）的会话凭据。它不对应任何 REST
	// 命名空间：调用方是设备/客户端，面向的是语音会话，判定在数据面的握手里
	// （走 /internal/apikey/authorize 问控制面）。
	ScopeVoice Scope = "apikey:scope:voice"
)

// allScopes 是全部合法取值，顺序即控制台里的展示顺序。
var allScopes = []Scope{ScopeAll, ScopeRead, ScopeAgent, ScopeMCP, ScopeData, ScopeVoice}

// AllScopes 返回全部合法取值（复制一份，调用方改不坏包内的表）。
func AllScopes() []Scope {
	return append([]Scope(nil), allScopes...)
}

// Valid 判断取值是否为已知权限范围。
func (s Scope) Valid() bool {
	for _, known := range allScopes {
		if s == known {
			return true
		}
	}
	return false
}

// ParseScopes 校验并规范化入参：未知取值报 ErrInvalidScope，重复项去掉。
// 空列表同样报错——不给“默认全权”的余地。
func ParseScopes(raw []string) ([]Scope, error) {
	if len(raw) == 0 {
		return nil, ErrInvalidScope
	}
	out := make([]Scope, 0, len(raw))
	seen := make(map[Scope]struct{}, len(raw))
	for _, item := range raw {
		s := Scope(strings.TrimSpace(item))
		if !s.Valid() {
			return nil, fmt.Errorf("%w: %q", ErrInvalidScope, item)
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out, nil
}

// Strings 把权限范围转成落库用的字符串切片。
func Strings(scopes []Scope) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, string(s))
	}
	return out
}

// FromStrings 还原落库的权限范围。不做校验：库里可能留着历史取值，
// 校验发生在写入侧（ParseScopes）。
func FromStrings(raw []string) []Scope {
	out := make([]Scope, 0, len(raw))
	for _, s := range raw {
		out = append(out, Scope(s))
	}
	return out
}

// Allows 判定一组权限范围能否访问“需要 required、方法为 method”的接口。
// 安全方法（GET/HEAD/OPTIONS）是 ScopeRead 的放行范围。
func Allows(scopes []Scope, required Scope, method string) bool {
	if AllowsScope(scopes, required) {
		return true
	}
	for _, s := range scopes {
		if s == ScopeRead && isSafeMethod(method) {
			return true
		}
	}
	return false
}

// AllowsScope 判定是否持有某个命名空间的能力，不看 HTTP 方法。
// 语音接入（ScopeVoice）这类非 REST 场景用它。
func AllowsScope(scopes []Scope, required Scope) bool {
	for _, s := range scopes {
		if s == ScopeAll || s == required {
			return true
		}
	}
	return false
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
