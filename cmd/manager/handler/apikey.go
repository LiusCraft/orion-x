package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// APIKeyHandler 管的是用户自己的访问密钥：只能看见、只能删自己的。
// 密钥本身不能调用这里的接口（middleware.Auth 的表里没有 /api/apikeys），
// 所以“用旧密钥签发新密钥 / 读回别人的密钥”这种提权路径不存在。
type APIKeyHandler struct {
	keys  *store.APIKeyStore
	users *store.UserStore
}

func NewAPIKeyHandler(keys *store.APIKeyStore, users *store.UserStore) *APIKeyHandler {
	return &APIKeyHandler{keys: keys, users: users}
}

// apiKeyResponse 是密钥的对外形态。明文只在创建响应里出现一次（key 字段），
// 其余接口只能给脱敏串：库里存的是摘要，明文取不回来。
type apiKeyResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Key        string     `json:"key,omitempty"`
	MaskedKey  string     `json:"masked_key"`
	Scopes     []string   `json:"scopes"`
	CallCount  int64      `json:"call_count"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func newAPIKeyResponse(k *store.APIKey) apiKeyResponse {
	return apiKeyResponse{
		ID:         k.ID,
		Name:       k.Name,
		MaskedKey:  apikey.Masked(k.KeyPrefix, k.KeyLast4),
		Scopes:     apikey.Strings(apikey.FromStrings(k.Scopes)),
		CallCount:  k.CallCount,
		LastUsedAt: k.LastUsedAt,
		CreatedAt:  k.CreatedAt,
	}
}

// GET /api/apikeys
func (h *APIKeyHandler) List(c *gin.Context) {
	list, err := h.keys.ListByOwner(middleware.UserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]apiKeyResponse, 0, len(list))
	for i := range list {
		out = append(out, newAPIKeyResponse(&list[i]))
	}
	c.JSON(http.StatusOK, out)
}

type createAPIKeyRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

// POST /api/apikeys
func (h *APIKeyHandler) Create(c *gin.Context) {
	var req createAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > maxAPIKeyNameLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must be 1-64 characters"})
		return
	}
	scopes, err := apikey.ParseScopes(req.Scopes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scopes must be a non-empty list of known permissions"})
		return
	}

	userID := middleware.UserID(c)
	key, plain, err := h.keys.Create(name, userID, scopes, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := newAPIKeyResponse(key)
	resp.Key = plain
	c.JSON(http.StatusCreated, resp)
}

// POST /api/apikeys/:id/reveal
type revealAPIKeyRequest struct {
	Password string `json:"password"`
}

// Reveal 验证账号密码后把明文交回给控制台（前端直接复制，不再展示）。
// 这是整套接口里唯一能取回明文的路径，所以三道门：会话（路由本身不在 API Key 白名单里）、
// 属主（Reveal 按 owner_id 过滤）、账号密码。
func (h *APIKeyHandler) Reveal(c *gin.Context) {
	var req revealAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is required"})
		return
	}

	userID := middleware.UserID(c)
	user, err := h.users.GetByID(userID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user.PasswordHash == "" {
		// OAuth 注册的账号可能还没设密码：没法验身份，先把话说清楚。
		c.JSON(http.StatusConflict, gin.H{"error": "account has no password; set one in the console first"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		// 刻意用 403：前端把 401 当“会话失效”处理，会顺手登出，而这里只是密码敲错了。
		c.JSON(http.StatusForbidden, gin.H{"error": "password is incorrect"})
		return
	}

	plain, err := h.keys.Reveal(userID, c.Param("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
		return
	case errors.Is(err, apikey.ErrSecretUnavailable):
		c.JSON(http.StatusConflict, gin.H{"error": "this key was issued without a stored secret; delete and recreate it"})
		return
	case errors.Is(err, apikey.ErrDecrypt):
		// 换过 jwt.secret：旧密文解不开了，密钥本身还能照常认证。
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot decrypt the stored key; the manager secret has changed"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logging.Infof("apikey: revealed key %s (owner %s)", c.Param("id"), userID)
	c.JSON(http.StatusOK, gin.H{"key": plain})
}

// DELETE /api/apikeys/:id
func (h *APIKeyHandler) Delete(c *gin.Context) {
	if err := h.keys.Delete(middleware.UserID(c), c.Param("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// maxAPIKeyNameLen 与 api_keys.name 的列宽一致。
const maxAPIKeyNameLen = 64

// APIKeyAuthorizeHandler 是数据面（wsserver）握手时的接入校验接口
// （/internal/apikey/authorize）。它只拿到“设备 + 明文密钥”两个事实，密钥属主、
// 设备属主、权限范围都由这里查库后交给 apikey.Decide 判定。
type APIKeyAuthorizeHandler struct {
	keys      *store.APIKeyStore
	devices   *store.DeviceStore
	voicebots *store.VoicebotStore
}

func NewAPIKeyAuthorizeHandler(keys *store.APIKeyStore, devices *store.DeviceStore, voicebots *store.VoicebotStore) *APIKeyAuthorizeHandler {
	return &APIKeyAuthorizeHandler{keys: keys, devices: devices, voicebots: voicebots}
}

// POST /internal/apikey/authorize
func (h *APIKeyAuthorizeHandler) Authorize(c *gin.Context) {
	var req apikey.AuthorizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	req.Key = strings.TrimSpace(req.Key)
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	if req.Key == "" || req.DeviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key and device_id are required"})
		return
	}

	var (
		facts   apikey.Facts
		keyID   string
		keyName string
	)
	key, err := h.keys.Authenticate(req.Key)
	switch {
	case err == nil:
		// Authenticate 同时记一次调用统计：控制台里能看到这把钥被接入过几次。
		facts.KeyFound = true
		facts.KeyOwnerID = key.OwnerID
		facts.KeyScopes = apikey.FromStrings(key.Scopes)
		keyID, keyName = key.ID, key.Name
	case errors.Is(err, store.ErrNotFound):
		// 未知密钥是正常结果，走 allowed=false；不再区分“格式不对”。
	default:
		// 查库失败是控制面自己的问题：回 500，让数据面按“验证不了就不放行”处理。
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	deviceOwner, deviceErr := h.deviceOwner(req.DeviceID)
	if deviceErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": deviceErr.Error()})
		return
	}
	if deviceOwner != "" {
		facts.DeviceFound = true
		facts.DeviceOwnerID = deviceOwner
	}

	reason := apikey.Decide(facts)
	if reason != apikey.RejectNone {
		logging.Warnf("apikey: rejected voice session for device %q: %s", req.DeviceID, reason)
		c.JSON(http.StatusOK, apikey.AuthorizeResponse{Allowed: false, RejectReason: reason})
		return
	}
	c.JSON(http.StatusOK, apikey.AuthorizeResponse{
		Allowed: true,
		OwnerID: facts.KeyOwnerID,
		KeyID:   keyID,
		KeyName: keyName,
	})
}

// deviceOwner 返回设备归属的智能体属主。设备不存在（或它绑的智能体已删）时
// 返回空串：对调用方而言这两种都是“不认识这个设备”。
func (h *APIKeyAuthorizeHandler) deviceOwner(deviceID string) (string, error) {
	device, err := h.devices.GetByID(deviceID)
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	voicebot, err := h.voicebots.GetByID(device.VoicebotID)
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		logging.Warnf("apikey: device %q points at missing voicebot %q", deviceID, device.VoicebotID)
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return voicebot.OwnerID, nil
}
