// Package oauth 提供第三方平台 OAuth 登录/绑定能力：
// Provider 注册表 + 授权 state 存储。具体平台（github 等）通过 Register 注册。
package oauth

import (
	"context"

	"golang.org/x/oauth2"
)

// UserInfo 第三方平台返回的用户信息。
type UserInfo struct {
	ProviderUID string // 平台侧用户唯一 ID
	Email       string
	Username    string
	AvatarURL   string
}

// Provider 第三方 OAuth 平台抽象。实现并注册后即可通过
// /api/auth/oauth/:provider/* 参与登录与账号绑定流程。
type Provider interface {
	// Name 平台标识，如 "github"。
	Name() string
	// DisplayName 面向用户的平台显示名，如 "GitHub"。
	DisplayName() string
	// AuthURL 生成授权页 URL（携带 state 参数防 CSRF）。
	AuthURL(state string) string
	// Exchange 用授权码换取 access token。
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	// FetchUserInfo 拉取平台侧用户信息（平台 UID、邮箱等）。
	FetchUserInfo(ctx context.Context, token *oauth2.Token) (*UserInfo, error)
}

var registry = map[string]Provider{}

// Register 注册 provider。同名重复注册会 panic，避免静默覆盖。
func Register(p Provider) {
	if _, ok := registry[p.Name()]; ok {
		panic("oauth: provider already registered: " + p.Name())
	}
	registry[p.Name()] = p
}

// Get 按名称获取已注册的 provider。
func Get(name string) (Provider, bool) {
	p, ok := registry[name]
	return p, ok
}

// Names 返回全部已注册平台名。
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
