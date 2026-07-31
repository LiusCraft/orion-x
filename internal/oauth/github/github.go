// Package github 提供 GitHub OAuth provider 实现。
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/oauth2"

	"github.com/liuscraft/orion-x/internal/oauth"
)

const (
	authURL       = "https://github.com/login/oauth/authorize"
	tokenURL      = "https://github.com/login/oauth/access_token"
	userURL       = "https://api.github.com/user"
	userEmailsURL = "https://api.github.com/user/emails"
)

type Provider struct {
	cfg *oauth2.Config
}

func New(clientID, clientSecret, redirectURL string) *Provider {
	return &Provider{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read:user", "user:email"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  authURL,
				TokenURL: tokenURL,
			},
		},
	}
}

func (p *Provider) Name() string { return "github" }

func (p *Provider) DisplayName() string { return "GitHub" }

func (p *Provider) AuthURL(state string) string {
	return p.cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (p *Provider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return p.cfg.Exchange(ctx, code)
}

func (p *Provider) FetchUserInfo(ctx context.Context, token *oauth2.Token) (*oauth.UserInfo, error) {
	user, err := p.fetchUser(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}
	email := user.Email
	if email == "" {
		email, err = p.fetchPrimaryEmail(ctx, token.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("github: fetch email: %w", err)
		}
	}
	return &oauth.UserInfo{
		ProviderUID: strconv.Itoa(user.ID),
		Email:       strings.TrimSpace(strings.ToLower(email)),
		Username:    user.Login,
		AvatarURL:   user.AvatarURL,
	}, nil
}

// ---------------------------------------------------------------------------
// GitHub API 响应类型
// ---------------------------------------------------------------------------

type githubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func (p *Provider) fetchUser(ctx context.Context, accessToken string) (*githubUser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", userURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github user API %s: %s", resp.Status, string(body))
	}

	var user githubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("parse user response: %w", err)
	}
	return &user, nil
}

// fetchPrimaryEmail 返回 GitHub 主邮箱（已验证）。用户隐藏公开邮箱时
// /user 的 email 为空，需要此接口兜底。
func (p *Provider) fetchPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", userEmailsURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github emails API %s: %s", resp.Status, string(body))
	}

	var emails []githubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("parse emails response: %w", err)
	}

	firstVerified := ""
	for _, e := range emails {
		if !e.Verified || strings.Contains(e.Email, "users.noreply.github.com") {
			continue
		}
		if e.Primary {
			return e.Email, nil
		}
		if firstVerified == "" {
			firstVerified = e.Email
		}
	}
	if firstVerified != "" {
		return firstVerified, nil
	}
	return "", fmt.Errorf("no verified email found")
}
