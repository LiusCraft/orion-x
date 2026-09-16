package handler

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/language"
	ttsprovider "github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/store"
)

var (
	errVoiceCloneForbidden            = errors.New("forbidden")
	errVoiceCloneUnsupportedModel     = errors.New("model does not support voice cloning")
	errVoiceCloneProviderUnconfigured = errors.New("voice cloning provider is not configured")
	errVoiceCloneProviderUnavailable  = errors.New("provider does not implement voice cloning")
	errVoiceCloneEmptyVoiceID         = errors.New("voice cloning provider returned an empty voice ID")
)

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
	Prefix         string                  `json:"prefix,omitempty"`
	SourceAudioURL string                  `json:"source_audio_url"`
	Format         ttsprovider.AudioFormat `json:"format"`
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
	models      voiceCloneModelStore
	voices      voiceCloneVoiceStore
	newProvider func(ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error)
}

func newProviderVoiceCloneService(
	models voiceCloneModelStore,
	voices voiceCloneVoiceStore,
	newProvider func(ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error),
) *providerVoiceCloneService {
	return &providerVoiceCloneService{models: models, voices: voices, newProvider: newProvider}
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

	result, err := cloner.CloneVoice(ctx, ttsprovider.VoiceCloneRequest{
		TargetModel:    model.ModelID,
		Prefix:         req.Prefix,
		Format:         req.Format,
		SourceAudioURL: req.SourceAudioURL,
		LanguageHints:  req.Langs,
		Extra:          req.Extra,
	})
	if err != nil {
		return nil, &voiceCloneProviderError{err: err}
	}
	if result == nil || strings.TrimSpace(result.VoiceID) == "" {
		return nil, errVoiceCloneEmptyVoiceID
	}

	voice, err := s.voices.CreateCloned(store.CloneVoiceParams{
		ModelID:        model.ID,
		VoiceID:        strings.TrimSpace(result.VoiceID),
		Name:           req.Name,
		SourceAudioURL: req.SourceAudioURL,
		Langs:          req.Langs,
		Creator:        userID,
	})
	if err != nil {
		return nil, err
	}
	return voice, nil
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
	if category, providerType, hasCategory := strings.Cut(slug, "/"); hasCategory {
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

	req.SourceAudioURL = strings.TrimSpace(req.SourceAudioURL)
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
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
	case errors.Is(err, errVoiceCloneForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	case errors.Is(err, errVoiceCloneUnsupportedModel),
		errors.Is(err, errVoiceCloneProviderUnconfigured),
		errors.Is(err, errVoiceCloneProviderUnavailable):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case errors.Is(err, ttsprovider.ErrBadRequest):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ttsprovider.ErrTransient):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
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
