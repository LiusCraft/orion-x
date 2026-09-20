package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/assets"
	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/billing/service"
	"github.com/liuscraft/orion-x/internal/language"
	"github.com/liuscraft/orion-x/internal/logging"
	ttsprovider "github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/store"
)

var (
	errVoiceCloneForbidden            = errors.New("forbidden")
	errVoiceCloneUnsupportedModel     = errors.New("model does not support voice cloning")
	errVoiceCloneProviderUnconfigured = errors.New("voice cloning provider is not configured")
	errVoiceCloneProviderUnavailable  = errors.New("provider does not implement voice cloning")
	errVoiceCloneStorageUnconfigured  = errors.New("object storage is not configured for voice samples")
	errVoiceCloneEmptyVoiceID         = errors.New("voice cloning provider returned an empty voice ID")
)

// voiceSamplePresignTTL 是交给复刻厂商拉取参考音频的预签名 URL 有效期。
// 厂商可能在任务排队后才下载音频，故比资源库默认的 presign_ttl 留更长窗口。
const voiceSamplePresignTTL = 2 * time.Hour

// VoiceCloneModel is a TTS model that can create cloned voices.
type VoiceCloneModel struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	ModelID      string         `json:"model_id"`
	ProviderID   string         `json:"provider_id"`
	ProviderName string         `json:"provider_name"`
	ProviderSlug string         `json:"provider_slug"`
	Langs        pq.StringArray `json:"langs,omitempty"`
	Configured   bool           `json:"configured"`
}

type cloneVoiceRequest struct {
	Name           string                  `json:"name"`
	Description    string                  `json:"description,omitempty"`
	Prefix         string                  `json:"prefix,omitempty"`
	SourceAssetID  string                  `json:"source_asset_id,omitempty"`
	SourceAudioURL string                  `json:"source_audio_url,omitempty"`
	Format         ttsprovider.AudioFormat `json:"format,omitempty"`
	Langs          pq.StringArray          `json:"langs,omitempty"`
	Extra          map[string]any          `json:"extra,omitempty"`
}

type voiceCloneService interface {
	ListModels(userID string) ([]VoiceCloneModel, error)
	Clone(ctx context.Context, modelID, userID string, req cloneVoiceRequest) (*store.ModelVoice, error)
}

type voiceCloneModelStore interface {
	List(userID string, modelType store.ModelType, lang string) ([]store.AIModel, error)
	GetByID(id string) (*store.AIModel, error)
}

type voiceCloneVoiceStore interface {
	CreateCloned(store.CloneVoiceParams) (*store.ModelVoice, error)
}

type providerVoiceCloneService struct {
	models voiceCloneModelStore
	voices voiceCloneVoiceStore
	assets voiceAssetStore
	// meter 是计费在复刻链路上的窄接口（§6.4 的 billing.Meter）。nil = 计费关闭
	// （billing.enabled: false）或调用方没接计费，此时既不预冻结也不扣费。
	meter       billing.Meter
	newProvider func(ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error)
}

func newProviderVoiceCloneService(
	models voiceCloneModelStore,
	voices voiceCloneVoiceStore,
	assetSvc voiceAssetStore,
	newProvider func(ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error),
	meter billing.Meter,
) *providerVoiceCloneService {
	return &providerVoiceCloneService{models: models, voices: voices, assets: assetSvc, newProvider: newProvider, meter: meter}
}

// GET /api/models/voice-cloning
func (h *VoiceHandler) ListCloneableModels(c *gin.Context) {
	if h.cloneService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "voice cloning service is not configured"})
		return
	}

	models, err := h.cloneService.ListModels(middleware.UserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, models)
}

// POST /api/models/:id/voices/clone
func (h *VoiceHandler) Clone(c *gin.Context) {
	if h.cloneService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "voice cloning service is not configured"})
		return
	}

	var req cloneVoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	voice, err := h.cloneService.Clone(c.Request.Context(), c.Param("id"), middleware.UserID(c), req)
	if err != nil {
		writeVoiceCloneError(c, err)
		return
	}
	c.JSON(http.StatusCreated, voice)
}

func (s *providerVoiceCloneService) ListModels(userID string) ([]VoiceCloneModel, error) {
	models, err := s.models.List(userID, store.ModelTypeSpeech, "")
	if err != nil {
		return nil, err
	}

	registered := ttsprovider.ListRegistered()
	out := make([]VoiceCloneModel, 0, len(models))
	for _, model := range models {
		if _, ok := cloneableTTSProvider(model, registered); !ok {
			continue
		}

		out = append(out, VoiceCloneModel{
			ID:           model.ID,
			Name:         model.Name,
			ModelID:      model.ModelID,
			ProviderID:   model.ProviderID,
			ProviderName: model.Provider.Name,
			ProviderSlug: model.Provider.Slug,
			Langs:        model.Langs,
			Configured:   strings.TrimSpace(model.Provider.APIKeyEnc) != "",
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ProviderName != out[j].ProviderName {
			return out[i].ProviderName < out[j].ProviderName
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *providerVoiceCloneService) Clone(ctx context.Context, modelID, userID string, req cloneVoiceRequest) (*store.ModelVoice, error) {
	model, err := s.models.GetByID(modelID)
	if err != nil {
		return nil, err
	}
	if !model.IsSystem && model.Creator != userID {
		return nil, errVoiceCloneForbidden
	}

	registered := ttsprovider.ListRegistered()
	providerType, ok := cloneableTTSProvider(*model, registered)
	if !ok {
		return nil, errVoiceCloneUnsupportedModel
	}

	req, err = normalizeVoiceCloneRequest(req)
	if err != nil {
		return nil, err
	}

	source, err := s.resolveSourceAudio(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(model.Provider.APIKeyEnc) == "" {
		return nil, errVoiceCloneProviderUnconfigured
	}
	if s.newProvider == nil {
		return nil, errVoiceCloneProviderUnavailable
	}

	provider, err := s.newProvider(ttsprovider.ProviderConfig{
		Type:   providerType,
		Config: voiceCloneProviderConfig(*model),
	})
	if err != nil {
		return nil, &voiceCloneProviderError{err: err}
	}
	cloner, ok := provider.(ttsprovider.VoiceCloner)
	if !ok {
		return nil, errVoiceCloneProviderUnavailable
	}

	// 一过性费用（§6.3）：先预冻结再调厂商，否则余额不足的用户会把平台那把厂商
	// key 白刷一遍。ref_id 由请求内容推出，在调厂商之前就定下来，Reserve / Charge /
	// Release 三段用的是同一个 (ref_type, ref_id)。
	refID := voiceCloneRefID(userID, model.ID, req)
	charge, err := s.beginVoiceCloneCharge(ctx, *model, userID, refID)
	if err != nil {
		return nil, err
	}

	result, err := cloner.CloneVoice(ctx, ttsprovider.VoiceCloneRequest{
		TargetModel:    model.ModelID,
		Prefix:         req.Prefix,
		Format:         source.format,
		SourceAudioURL: source.url,
		LanguageHints:  req.Langs,
		Extra:          req.Extra,
	})
	if err != nil {
		// 厂商失败：释放预冻结，不计费。
		charge.abandon(ctx, "provider")
		return nil, &voiceCloneProviderError{err: err}
	}
	if result == nil || strings.TrimSpace(result.VoiceID) == "" {
		charge.abandon(ctx, "empty_voice_id")
		return nil, errVoiceCloneEmptyVoiceID
	}

	voice, err := s.voices.CreateCloned(store.CloneVoiceParams{
		ModelID:        model.ID,
		VoiceID:        strings.TrimSpace(result.VoiceID),
		Name:           req.Name,
		Description:    req.Description,
		SourceAssetID:  source.assetID,
		SourceAudioURL: source.legacyURL,
		Langs:          req.Langs,
		Creator:        userID,
	})
	if err != nil {
		// 厂商那边复刻已经成了，但用户拿不到这个音色。宁可平台自己吃下这笔厂商
		// 成本（少收），也不要给用户开一笔“没拿到东西”的账。
		charge.abandon(ctx, "persist")
		return nil, err
	}

	// 业务动作完成才扣费：Charge 内部先落库事件再结算，并自动释放冻结。
	charge.capture(ctx, voice)
	return voice, nil
}

// ---------------------------------------------------------------- 复刻的计费（§6.3）

// voiceCloneRefID 由「用户 + 模型 + 规范化后的请求内容」拼出这次复刻的外部引用。
//
// 它是确定性的，所以客户端没收到响应就重发同一个请求时，Reserve / Charge 会命中
// 同一条预冻结和同一条扣费，不会重复冻结、重复扣钱（§6.4：幂等键由外部引用拼，
// 不由调用方随手生成）。不把厂商用的 prefix 算进来：prefix 缺省是每次随机生成的，
// 算进来等于每次重试都是新操作，幂等就没了。
func voiceCloneRefID(userID, modelID string, req cloneVoiceRequest) string {
	parts := []string{
		userID,
		modelID,
		req.Name,
		req.Description,
		req.SourceAssetID,
		req.SourceAudioURL,
		string(req.Format),
		strings.Join(req.Langs, ","),
		voiceCloneExtraFingerprint(req.Extra),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "vc_" + hex.EncodeToString(sum[:12])
}

// voiceCloneExtraFingerprint 把 extra 拼成稳定的字符串：json.Marshal 对 map 的键
// 是排序输出的，所以同一个 map 每次结果一致。
func voiceCloneExtraFingerprint(extra map[string]any) string {
	if len(extra) == 0 {
		return ""
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		return ""
	}
	return string(raw)
}

// voiceCloneCharge 是一次复刻的报账上下文。指针为 nil 表示这次动作不计费：计费关
// 闭（没有 meter）或 BYOK。
type voiceCloneCharge struct {
	meter      billing.Meter
	userID     string
	modelID    string
	providerID string
	refID      string
	reserved   billing.Reservation
}

// beginVoiceCloneCharge 调厂商之前的预冻结（§6.3）。计费关闭或 BYOK 时返回 nil, nil，
// 后续的 abandon / capture 都是 no-op。
//
// BYOK（Provider.IsSystem == false）：用户已经直接付给厂商那把 key 的钱，再收
// 一次是错账，所以整个链路不碰账户（§7）。
func (s *providerVoiceCloneService) beginVoiceCloneCharge(ctx context.Context, model store.AIModel, userID, refID string) (*voiceCloneCharge, error) {
	if s.meter == nil {
		return nil, nil
	}
	if !platformPaysForVoiceClone(model) {
		logging.Infof("billing: voice clone not billed (byok) user=%s model=%s", userID, model.ID)
		return nil, nil
	}

	reservation, err := s.meter.Reserve(ctx, billing.ReserveRequest{
		SubjectType: billing.SubjectTypeUser,
		SubjectID:   userID,
		ItemCode:    billing.ItemVoiceClone,
		Quantity:    1,
		RefType:     billing.RefVoiceClone,
		RefID:       refID,
		Dims:        voiceCloneChargeDims(model, ""),
	})
	if err != nil {
		// ErrInsufficientBalance → 402、价格没配（ErrItemNotFound）→ 503，
		// 都由 writeVoiceCloneError 映射；后者不按 0 元放行（§14.3）。
		return nil, fmt.Errorf("voice clone reserve: %w", err)
	}
	return &voiceCloneCharge{
		meter:      s.meter,
		userID:     userID,
		modelID:    model.ID,
		providerID: model.ProviderID,
		refID:      refID,
		reserved:   reservation,
	}, nil
}

// abandon 业务动作失败：释放预冻结，不计费。
func (ch *voiceCloneCharge) abandon(ctx context.Context, reason string) {
	if ch == nil {
		return
	}
	if err := ch.meter.Release(ctx, ch.reserved.ID); err != nil {
		logging.Errorf("billing: release voice clone reservation ref=%s reason=%s: %v", ch.refID, reason, err)
		return
	}
	logging.Infof("billing: voice clone reservation released ref=%s reason=%s", ch.refID, reason)
}

// capture 业务动作成功：扣费，顺带把预冻结捕获掉（Charge 内部按同一个 ref 释放）。
//
// Charge 内部是先落库事件、再结算（service/charge.go）：所以这里失败时事件可能已经
// 落在 pending 上、由 worker 补扣；真的一件没落下去时，下面这条 Errorf 是唯一线索，
// 运维可以按同一个幂等键（charge:voice:clone:<ref_id>）重放。
func (ch *voiceCloneCharge) capture(ctx context.Context, voice *store.ModelVoice) {
	if ch == nil {
		return
	}
	result, err := ch.meter.Charge(ctx, billing.ChargeRequest{
		SubjectType: billing.SubjectTypeUser,
		SubjectID:   ch.userID,
		ItemCode:    billing.ItemVoiceClone,
		Quantity:    1,
		RefType:     billing.RefVoiceClone,
		RefID:       ch.refID,
		Dims:        ch.dims(voice.ID),
	})
	if err != nil {
		// 复刻已经做成了，不能因为记账失败就把结果丢掉：事件落库成功但结算失败时
		// worker 会补扣，真正落库失败时得靠这条日志重放。
		logging.Errorf("billing: voice clone charge failed ref=%s voice=%s model=%s user=%s: %v",
			ch.refID, voice.ID, ch.modelID, ch.userID, err)
		return
	}
	logging.Infof("billing: voice clone charged ref=%s voice=%s amount_micro=%d price=%s skipped=%t",
		ch.refID, voice.ID, result.AmountMicro, result.PriceID, result.Skipped)
}

// platformPaysForVoiceClone 判断这次复刻算不算平台代付。真正付给厂商的是
// provider 那把 key，所以“平台代付还是 BYOK”由 Provider.IsSystem 决定（§7）。
func platformPaysForVoiceClone(model store.AIModel) bool {
	return model.Provider != nil && model.Provider.IsSystem
}

// voiceCloneChargeDims 是报账时带上的资源维度：价格按 model / provider 级匹配，
// 流水排查靠 voice_id。
func voiceCloneChargeDims(model store.AIModel, voiceID string) map[string]any {
	dims := map[string]any{"model_id": model.ID}
	if model.ProviderID != "" {
		dims["provider_id"] = model.ProviderID
	}
	if voiceID != "" {
		dims["voice_id"] = voiceID
	}
	return dims
}

// dims 是这次报账的资源维度。
func (ch *voiceCloneCharge) dims(voiceID string) map[string]any {
	return voiceCloneChargeDims(store.AIModel{ID: ch.modelID, ProviderID: ch.providerID}, voiceID)
}

// voiceCloneSource 是一次复刻的参考音频来源：要么是资源库里的 voice:sample 资源，
// 要么是调用方直传的外部 URL（legacy）。
type voiceCloneSource struct {
	url       string // 传给厂商的可访问 URL
	assetID   string // 落库：资源 ID
	legacyURL string // 落库：外部 URL，资源来源时留空（预签名 URL 会过期，不入库）
	format    ttsprovider.AudioFormat
}

func (s *providerVoiceCloneService) resolveSourceAudio(ctx context.Context, userID string, req cloneVoiceRequest) (voiceCloneSource, error) {
	if req.SourceAssetID == "" {
		return voiceCloneSource{url: req.SourceAudioURL, legacyURL: req.SourceAudioURL, format: req.Format}, nil
	}
	if s.assets == nil {
		return voiceCloneSource{}, errVoiceCloneStorageUnconfigured
	}

	asset, err := s.assets.Get(ctx, userID, req.SourceAssetID)
	if err != nil {
		return voiceCloneSource{}, err
	}
	if assets.Purpose(asset.Purpose) != assets.PurposeVoiceSample {
		return voiceCloneSource{}, fmt.Errorf("%w: source asset purpose must be %q, got %q",
			ttsprovider.ErrBadRequest, assets.PurposeVoiceSample, asset.Purpose)
	}
	format, err := voiceCloneFormatForAsset(asset.Name)
	if err != nil {
		return voiceCloneSource{}, err
	}

	url, err := s.assets.PresignURLWithTTL(ctx, asset, voiceSamplePresignTTL, false)
	if err != nil {
		return voiceCloneSource{}, err
	}
	return voiceCloneSource{url: url, assetID: asset.ID, format: format}, nil
}

func cloneableTTSProvider(model store.AIModel, registered map[string]ttsprovider.ProviderMeta) (string, bool) {
	if model.Type != store.ModelTypeSpeech || model.Provider == nil {
		return "", false
	}

	providerType, ok := ttsProviderTypeForSlug(model.Provider.Slug, registered)
	if !ok {
		return "", false
	}
	meta := registered[providerType]
	if !hasTTSFeature(meta.Features, ttsprovider.FeatureVoiceCloning) {
		return "", false
	}
	if len(meta.Models) > 0 {
		if _, ok := meta.Models[model.ModelID]; !ok {
			return "", false
		}
	}
	return providerType, true
}

func ttsProviderTypeForSlug(slug string, registered map[string]ttsprovider.ProviderMeta) (string, bool) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if category, providerType, hasCategory := strings.Cut(slug, ":"); hasCategory {
		if category != "tts" {
			return "", false
		}
		slug = providerType
	}
	_, ok := registered[slug]
	return slug, ok
}

func hasTTSFeature(features []ttsprovider.Feature, want ttsprovider.Feature) bool {
	return slices.Contains(features, want)
}

func normalizeVoiceCloneRequest(req cloneVoiceRequest) (cloneVoiceRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return req, fmt.Errorf("%w: name is required", ttsprovider.ErrBadRequest)
	}
	if utf8.RuneCountInString(req.Name) > 128 {
		return req, fmt.Errorf("%w: name must be at most 128 characters", ttsprovider.ErrBadRequest)
	}
	req.Description = strings.TrimSpace(req.Description)

	req.SourceAssetID = strings.TrimSpace(req.SourceAssetID)
	req.SourceAudioURL = strings.TrimSpace(req.SourceAudioURL)
	switch {
	case req.SourceAssetID != "" && req.SourceAudioURL != "":
		return req, fmt.Errorf("%w: source_asset_id and source_audio_url are mutually exclusive", ttsprovider.ErrBadRequest)
	case req.SourceAssetID != "":
		if len(req.SourceAssetID) > 36 {
			return req, fmt.Errorf("%w: source_asset_id must be at most 36 characters", ttsprovider.ErrBadRequest)
		}
		// 音频格式由资源文件扩展名推导，忽略调用方传入的 format。
		req.Format = ""
	default:
		if req.SourceAudioURL == "" {
			return req, fmt.Errorf("%w: source_asset_id or source_audio_url is required", ttsprovider.ErrBadRequest)
		}
		if len(req.SourceAudioURL) > 512 {
			return req, fmt.Errorf("%w: source audio URL must be at most 512 characters", ttsprovider.ErrBadRequest)
		}
		if err := validateSourceAudioURL(req.SourceAudioURL); err != nil {
			return req, fmt.Errorf("%w: %v", ttsprovider.ErrBadRequest, err)
		}
		format, err := normalizeVoiceCloneFormat(req.Format)
		if err != nil {
			return req, err
		}
		req.Format = format
	}

	langs, err := normalizeVoiceCloneLangs(req.Langs)
	if err != nil {
		return req, err
	}
	req.Langs = langs

	req.Prefix = strings.TrimSpace(req.Prefix)
	if req.Prefix == "" {
		req.Prefix = generatedVoiceClonePrefix()
	}
	return req, nil
}

// voiceCloneFormatForAsset 从参考音频文件名推导厂商侧音频格式。
// 可接受的扩展名由 assets 的 voice:sample 白名单保证。
func voiceCloneFormatForAsset(fileName string) (ttsprovider.AudioFormat, error) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
	format, err := normalizeVoiceCloneFormat(ttsprovider.AudioFormat(ext))
	if err != nil {
		return "", fmt.Errorf("%w: unsupported source audio file %q", ttsprovider.ErrBadRequest, fileName)
	}
	return format, nil
}

func validateSourceAudioURL(raw string) error {
	if raw == "" {
		return errors.New("source audio URL is required")
	}

	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("source audio URL must be an absolute HTTP or HTTPS URL")
	}
	return nil
}

func normalizeVoiceCloneFormat(format ttsprovider.AudioFormat) (ttsprovider.AudioFormat, error) {
	format = ttsprovider.AudioFormat(strings.ToLower(strings.TrimSpace(string(format))))
	switch format {
	case ttsprovider.FormatPCM, ttsprovider.FormatMP3, ttsprovider.FormatM4A, ttsprovider.FormatWAV, ttsprovider.FormatOpus, ttsprovider.FormatFlac:
		return format, nil
	default:
		return "", fmt.Errorf("%w: unsupported source audio format %q", ttsprovider.ErrBadRequest, format)
	}
}

func normalizeVoiceCloneLangs(langs pq.StringArray) (pq.StringArray, error) {
	if len(langs) == 0 {
		return langs, nil
	}

	out := make(pq.StringArray, 0, len(langs))
	seen := make(map[string]struct{}, len(langs))
	for _, raw := range langs {
		code := language.Normalize(raw)
		if code == "" || !language.Exists(code) {
			return nil, fmt.Errorf("%w: unknown language %q", ttsprovider.ErrBadRequest, raw)
		}
		if _, ok := seen[string(code)]; ok {
			continue
		}
		seen[string(code)] = struct{}{}
		out = append(out, string(code))
	}
	return out, nil
}

func generatedVoiceClonePrefix() string {
	return "vc" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
}

func voiceCloneProviderConfig(model store.AIModel) ttsprovider.Config {
	cfg := ttsprovider.Config{Model: model.ModelID}
	if model.Provider == nil {
		return cfg
	}

	cfg.APIKey = model.Provider.APIKeyEnc
	cfg.Endpoint = model.BaseURL
	if strings.TrimSpace(cfg.Endpoint) == "" {
		cfg.Endpoint = model.Provider.BaseURL
	}
	cfg.Extra = mergeVoiceCloneConfigExtra(model.Provider.Extra, model.Extra)
	return cfg
}

func mergeVoiceCloneConfigExtra(providerExtra, modelExtra map[string]any) map[string]any {
	if len(providerExtra) == 0 && len(modelExtra) == 0 {
		return nil
	}

	out := make(map[string]any, len(providerExtra)+len(modelExtra))
	maps.Copy(out, providerExtra)
	maps.Copy(out, modelExtra)
	return out
}

type voiceCloneProviderError struct {
	err error
}

func (e *voiceCloneProviderError) Error() string {
	return e.err.Error()
}

func (e *voiceCloneProviderError) Unwrap() error {
	return e.err
}

func writeVoiceCloneError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, store.ErrNotFound), errors.Is(err, assets.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "model or source audio not found"})
	case errors.Is(err, service.ErrInsufficientBalance):
		// 预冻结时就发现钱不够。这里用 402（这条路径的业务就是“先付钱再复刻”），
		// 与管理端那套通用映射（余额不足 = 409 冲突）不冲突：两边的调用者不同。
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "余额不足，无法复刻音色"})
	case errors.Is(err, service.ErrItemNotFound):
		// 定价表里没配 voice:clone：不按 0 元放行（§14.3）。这不是用户能修的问题。
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "音色复刻未配置价格（voice:clone）"})
	case errors.Is(err, errVoiceCloneForbidden), errors.Is(err, assets.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	case errors.Is(err, errVoiceCloneUnsupportedModel),
		errors.Is(err, errVoiceCloneProviderUnconfigured),
		errors.Is(err, errVoiceCloneProviderUnavailable):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case errors.Is(err, errVoiceCloneStorageUnconfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, ttsprovider.ErrBadRequest):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ttsprovider.ErrTransient):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, assets.ErrStorage):
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	case errors.Is(err, ttsprovider.ErrAuth), errors.Is(err, errVoiceCloneEmptyVoiceID):
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	default:
		if _, ok := errors.AsType[*voiceCloneProviderError](err); ok {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
