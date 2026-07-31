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
	"sync"
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
	githubStateTTL      = 10 * time.Minute
)

// ---------------------------------------------------------------------------
// Server-side state store — replaces cookie-based CSRF validation so that
// OAuth callbacks work reliably regardless of hostname / IP changes between
// login (local proxy) and callback (public redirect).
// ---------------------------------------------------------------------------

type oauthStateEntry struct {
	redirectTo string
	expiresAt  time.Time
}

type oauthStateStore struct {
	mu    sync.Mutex
	items map[string]oauthStateEntry
}

var globalStateStore = newOAuthStateStore()

func newOAuthStateStore() *oauthStateStore {
	s := &oauthStateStore{items: make(map[string]oauthStateEntry)}
	go s.cleanupLoop()
	return s
}

func (s *oauthStateStore) put(key, redirectTo string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = oauthStateEntry{redirectTo: redirectTo, expiresAt: time.Now().Add(githubStateTTL)}
}

// take atomically reads and deletes the entry. Returns (redirectTo, true) on success.
func (s *oauthStateStore) take(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(s.items, key)
		return "", false
	}
	delete(s.items, key) // one-time use
	return entry.redirectTo, true
}

func (s *oauthStateStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for k, v := range s.items {
			if now.After(v.expiresAt) {
				delete(s.items, k)
			}
		}
		s.mu.Unlock()
	}
}

// ---------------------------------------------------------------------------
// GitHub user / email response types
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Login — redirect to GitHub authorize page
// GET /api/auth/github/login?from=http://localhost:5173
// ---------------------------------------------------------------------------

func (h *GithubAuthHandler) Login(c *gin.Context) {
	if h.oauthCfg.ClientID == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "GitHub OAuth 未配置"})
		return
	}

	frontend := c.Query("from")
	if frontend == "" {
		frontend = "/"
	}

	stateKey := newCSRFToken()
	globalStateStore.put(stateKey, frontend)

	authURL := h.oauthCfg.AuthCodeURL(stateKey, oauth2.AccessTypeOnline)
	c.Redirect(http.StatusFound, authURL)
}

// ---------------------------------------------------------------------------
// Callback — handle GitHub OAuth callback
// GET /api/auth/github/callback?code=xxx&state=yyy
// ---------------------------------------------------------------------------

func (h *GithubAuthHandler) Callback(c *gin.Context) {
	if h.oauthCfg.ClientID == "" || h.oauthCfg.ClientSecret == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "GitHub OAuth 未配置"})
		return
	}

	stateKey := c.Query("state")
	if stateKey == "" {
		h.redirectError("/", c, "缺少 state 参数")
		return
	}

	redirectTo, ok := globalStateStore.take(stateKey)
	if !ok {
		h.redirectError("/", c, "安全验证失败，请重新授权")
		return
	}
	if redirectTo == "" {
		redirectTo = "/"
	}

	code := c.Query("code")
	if code == "" {
		h.redirectError(redirectTo, c, "缺少 code 参数")
		return
	}

	// Exchange code for access token
	ctx := c.Request.Context()
	oauthToken, err := h.oauthCfg.Exchange(ctx, code)
	if err != nil {
		logging.Errorf("github oauth: exchange code: %v", err)
		h.redirectError(redirectTo, c, "GitHub 授权失败")
		return
	}

	// Fetch user info from GitHub
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

	// Redirect back to frontend with token
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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
