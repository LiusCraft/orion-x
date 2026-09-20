package xiaozhi

import (
	"context"
	"strings"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/channels"
	"github.com/liuscraft/orion-x/internal/logging"
)

// authorizeConnection 在握手阶段决定这次连接能不能继续：返回 nil 表示放行。
//
// 凭据来自 HTTP 层（Authorization: Bearer ox:sk:... 或 ?access_token=），数据面不
// 自己解析密钥——它把“设备 + 明文”交给控制面（/internal/apikey/authorize），由
// 控制面确认密钥存在、带 apikey:scope:voice、且设备属于这把钥的属主。
//
// 三条本地规则：
//   - 不是我们格式的凭据（比如固件自带的随机 token）一律忽略，除非开了 require_api_key。
//   - 带了密钥但验不了（控制面不可达 / 没配内部 token / 没注入校验器）→ 拒绝。
//     取不到结论时放行等于没鉴权。
//   - 没带密钥时由 auth.require_api_key 决定：默认照旧按 device_id 接入。
func (s *XiaozhiWSChannel) authorizeConnection(deviceID, credential string) *sessionRejectedError {
	requireKey := s.cfg != nil && s.cfg.Auth.RequireAPIKey
	key := bearerToken(credential)

	if !apikey.LooksLike(key) {
		if requireKey {
			logging.Warnf("xiaozhi-channel: rejecting device %q: no api key presented", deviceID)
			return &sessionRejectedError{DeviceID: deviceID, Reason: string(apikey.RejectKeyMissing)}
		}
		return nil
	}

	authorizer := s.apiKeyAuthorizer()
	if authorizer == nil {
		logging.Errorf("xiaozhi-channel: rejecting device %q: no api key verifier configured", deviceID)
		return &sessionRejectedError{DeviceID: deviceID, Reason: string(apikey.RejectKeyUnavailable)}
	}

	ctx := s.rootCtx
	if ctx == nil {
		// Start 之前 rootCtx 还是 nil（单测里直接建连接就是这个情形）。
		ctx = context.Background()
	}
	resp, err := authorizer.Authorize(ctx, key, deviceID)
	if err != nil {
		logging.Errorf("xiaozhi-channel: api key check for device %q failed: %v", deviceID, err)
		return &sessionRejectedError{DeviceID: deviceID, Reason: string(apikey.RejectKeyUnavailable)}
	}
	if !resp.Allowed {
		logging.Warnf("xiaozhi-channel: rejecting device %q: %s (key=%s)", deviceID, resp.RejectReason, resp.KeyName)
		return &sessionRejectedError{DeviceID: deviceID, Reason: string(resp.RejectReason)}
	}
	logging.Infof("xiaozhi-channel: device %q authenticated with api key %q (owner=%s)", deviceID, resp.KeyName, resp.OwnerID)
	return nil
}

func (s *XiaozhiWSChannel) apiKeyAuthorizer() channels.APIKeyAuthorizer {
	if s.deps == nil {
		return nil
	}
	return s.deps.APIKeyAuth
}

// bearerToken 把凭据归一化成裸 token：HTTP 头形如 `Authorization: Bearer ox:sk:...`，
// 而 `?access_token=ox:sk:...` 本身就是裸 token。两种入口都指向同一把钥。
func bearerToken(raw string) string {
	token := strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(token, "Bearer "); ok {
		return strings.TrimSpace(rest)
	}
	return token
}
