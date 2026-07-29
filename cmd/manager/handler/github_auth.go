package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

const (
	githubAuthURL       = "https://github.com/login/oauth/authorize"
	githubTokenURL      = "https://github.com/login/oauth/access_token"
	githubUserURL       = "https://api.github.com/user"
	githubUserEmailsURL = "https://api.github.com/user/emails"
	githubStateCookie   = "github_oauth_state"
	githubStateTTL      = 10 * time.Minute
)

type githubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

type GithubAuthHandler struct {
	users     *store.UserStore
	signToken func(userID string, isAdmin bool) (string, error)
	oauthCfg  *oauth2.Config
}

func NewGithubAuthHandler(users *store.UserStore, signToken func(userID string, isAdmin bool) (string, error), clientID, clientSecret, redirectURL string) *GithubAuthHandler {
	return &GithubAuthHandler{
		users:     users,
		signToken: signToken,
		oauthCfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read:user", "user:email"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  githubAuthURL,
				TokenURL: githubTokenURL,
			},
		},
	}
}

// stateData OAuth state 参数，携带 CSRF token 和回调后跳转地址
type stateData struct {
	CSRFToken  string `json:"t"`
	RedirectTo string `json:"r,omitempty"` // 前端地址，GitHub 回调后跳回去
}

func marshalState(s stateData) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func unmarshalState(raw string) (stateData, error) {
	var d stateData
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return stateData{}, err
	}
	return d, nil
}

// Login 跳转到 GitHub 授权页
// GET /api/auth/github/login?from=http://localhost:5173
func (h *GithubAuthHandler) Login(c *gin.Context) {
	if h.oauthCfg.ClientID == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "GitHub OAuth 未配置"})
		return
	}

	csrfToken := newCSRFToken()

	frontend := c.Query("from")
	if frontend == "" {
		frontend = "/"
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     githubStateCookie,
		Value:    csrfToken,
		Path:     "/",
		MaxAge:   int(githubStateTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// state 里同时编码 CSRF token 和前端地址
	state := marshalState(stateData{
		CSRFToken:  csrfToken,
		RedirectTo: frontend,
	})

	authURL := h.oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
	c.Redirect(http.StatusFound, authURL)
}

// Callback 处理 GitHub OAuth 回调
// GET /api/auth/github/callback?code=xxx&state=yyy
func (h *GithubAuthHandler) Callback(c *gin.Context) {
	if h.oauthCfg.ClientID == "" || h.oauthCfg.ClientSecret == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "GitHub OAuth 未配置"})
		return
	}

	// 解析 state
	stateRaw := c.Query("state")
	if stateRaw == "" {
		h.redirectError("/", c, "缺少 state 参数")
		return
	}

	// state 可能是 JSON 编码的 stateData，也可能是纯字符串的 CSRF token（兼容旧版）
	sd, err := unmarshalState(stateRaw)
	var csrfToken string
	var redirectTo string
	if err == nil {
		csrfToken = sd.CSRFToken
		redirectTo = sd.RedirectTo
	} else {
		// 回退：state 就是 CSRF token 本身
		csrfToken = stateRaw
		redirectTo = "/"
	}

	if redirectTo == "" {
		redirectTo = "/"
	}

	// 验证 CSRF token
	csrfCookie, err := c.Cookie(githubStateCookie)
	if err != nil || csrfCookie == "" || csrfCookie != csrfToken {
		h.redirectError(redirectTo, c, "安全验证失败，请重新授权")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     githubStateCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	code := c.Query("code")
	if code == "" {
		h.redirectError(redirectTo, c, "缺少 code 参数")
		return
	}

	// 交换 token
	ctx := c.Request.Context()
	oauthToken, err := h.oauthCfg.Exchange(ctx, code)
	if err != nil {
		logging.Errorf("github oauth: exchange code: %v", err)
		h.redirectError(redirectTo, c, "GitHub 授权失败")
		return
	}

	// 获取用户信息
	ghUser, err := h.fetchUser(ctx, oauthToken.AccessToken)
	if err != nil {
		logging.Errorf("github oauth: fetch user: %v", err)
		h.redirectError(redirectTo, c, "获取 GitHub 用户信息失败")
		return
	}

	email := ghUser.Email
	if email == "" {
		email, err = h.fetchPrimaryEmail(ctx, oauthToken.AccessToken)
		if err != nil {
			logging.Errorf("github oauth: fetch emails: %v", err)
			h.redirectError(redirectTo, c, "无法获取 GitHub 邮箱")
			return
		}
	}

	email = strings.TrimSpace(strings.ToLower(email))
	githubID := fmt.Sprintf("%d", ghUser.ID)

	u, err := h.users.GetByEmail(email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		logging.Errorf("github oauth: lookup by email: %v", err)
		h.redirectError(redirectTo, c, "内部错误")
		return
	}

	if u == nil {
		u, err = h.users.CreateWithGithub(email, githubID, "github")
		if err != nil {
			logging.Errorf("github oauth: create user: %v", err)
			h.redirectError(redirectTo, c, "创建用户失败")
			return
		}
		logging.Infof("github oauth: created user %q (email=%s, github_id=%s)", u.Username, email, githubID)
	} else if u.GithubID == "" {
		if err := h.users.UpdateGithubID(u.ID, githubID); err != nil {
			logging.Errorf("github oauth: link github id: %v", err)
			h.redirectError(redirectTo, c, "GitHub 绑定失败")
			return
		}
		logging.Infof("github oauth: linked github_id=%s to user %q (email=%s)", githubID, u.Username, email)
	}

	jwtToken, err := h.signToken(u.ID, u.IsAdmin)
	if err != nil {
		h.redirectError(redirectTo, c, "token 生成失败")
		return
	}

	// 跳回前端
	sep := "?"
	if strings.Contains(redirectTo, "?") {
		sep = "&"
	}
	c.Redirect(http.StatusFound, fmt.Sprintf("%s%stoken=%s&user_id=%s&username=%s",
		redirectTo, sep,
		url.QueryEscape(jwtToken),
		url.QueryEscape(u.ID),
		url.QueryEscape(u.Username),
	))
}

func (h *GithubAuthHandler) redirectError(redirectTo string, c *gin.Context, msg string) {
	sep := "?"
	if strings.Contains(redirectTo, "?") {
		sep = "&"
	}
	c.Redirect(http.StatusFound, fmt.Sprintf("%s%serror=%s", redirectTo, sep, url.QueryEscape(msg)))
}

func (h *GithubAuthHandler) fetchUser(ctx context.Context, accessToken string) (*githubUser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", githubUserURL, nil)
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

	var ghUser githubUser
	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("parse user response: %w", err)
	}
	return &ghUser, nil
}

func (h *GithubAuthHandler) fetchPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", githubUserEmailsURL, nil)
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
	return "", errors.New("no verified email found")
}

func newCSRFToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(buf)
}
