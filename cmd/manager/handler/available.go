package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/assets"
	"github.com/liuscraft/orion-x/internal/language"
	llmprovider "github.com/liuscraft/orion-x/internal/llm/provider"
	asrprovider "github.com/liuscraft/orion-x/internal/provider/asr"
	ttsprovider "github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/store"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
)

type AvailableHandler struct {
	providers *store.ProviderStore
	models    *store.AIModelStore
	voices    *store.ModelVoiceStore
	assets    *assets.Service
}

// NewAvailableHandler 构造 AvailableHandler；assetSvc 为 nil 表示未配置对象存储，
// 此时复刻音色没有试听地址（参考音频不对外服务）。
func NewAvailableHandler(providers *store.ProviderStore, models *store.AIModelStore, voices *store.ModelVoiceStore, assetSvc *assets.Service) *AvailableHandler {
	return &AvailableHandler{providers: providers, models: models, voices: voices, assets: assetSvc}
}

type ResourceOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type VoiceResource struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	Gender         string         `json:"gender,omitempty"`
	AvatarURL      string         `json:"avatar_url,omitempty"`
	PreviewURL     string         `json:"preview_url,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	Langs          []string       `json:"langs,omitempty"`
	Emotions       map[string]any `json:"emotions,omitempty"`
	IsSystem       bool           `json:"is_system"`
	IsCloned       bool           `json:"is_cloned"`
	SourceAudioURL string         `json:"source_audio_url,omitempty"`
}

type AvailableResourcesResponse struct {
	ASR    []ResourceOption `json:"asr"`
	Voices []VoiceResource  `json:"voices"`
}

// voiceMatchesLang 按语言过滤音色；音色未声明语言时视为通用，不过滤。
func voiceMatchesLang(lang string, langs []string) bool {
	if lang == "" || len(langs) == 0 {
		return true
	}
	codes := make([]language.Code, len(langs))
	for i, l := range langs {
		codes[i] = language.Code(l)
	}
	return language.Match(lang, codes)
}

// newVoiceResource 把存储层音色转成资源列表项；previewURL 是试听地址
// （复刻音色由参考音频现算，其余取库里存的 preview_url）。
func newVoiceResource(v store.ModelVoice, previewURL string) VoiceResource {
	r := VoiceResource{
		ID:             v.ID,
		Name:           v.Name,
		Description:    v.Description,
		Gender:         string(v.Gender),
		AvatarURL:      v.AvatarURL,
		PreviewURL:     previewURL,
		Langs:          v.Langs,
		Emotions:       v.Emotions,
		IsSystem:       v.IsSystem,
		IsCloned:       v.IsCloned,
		SourceAudioURL: v.SourceAudioURL,
	}
	if v.Tags != nil {
		r.Tags = v.Tags
	}
	return r
}

// extractCategory extracts the explicit category from a provider slug.
// "asr:aliyun" → ("asr", "aliyun"),  "llm:openai-completions" → ("llm", "openai-completions"),
// "aliyun" → ("", "aliyun").
func extractCategory(slug string) (category, key string) {
	prefix, rest, hasCategory := strings.Cut(slug, ":")
	if hasCategory {
		switch prefix {
		case "asr", "tts", "llm":
			return prefix, rest
		}
	}
	return "", slug
}

func (h *AvailableHandler) List(c *gin.Context) {
	userID := middleware.UserID(c)
	lang := c.Query("lang")

	// ── slug registries (used when a provider slug has no explicit prefix) ──
	slugCats := map[string][]string{}
	for key := range asrprovider.ListRegistered() {
		slugCats[key] = append(slugCats[key], "asr")
	}
	for key := range ttsprovider.ListRegistered() {
		slugCats[key] = append(slugCats[key], "tts")
	}
	for key := range llmprovider.DefaultRegistry().ListRegistered() {
		slugCats[key] = append(slugCats[key], "llm")
	}

	providers, err := h.providers.List(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	providerByID := map[string]store.Provider{}
	for _, p := range providers {
		providerByID[p.ID] = p
	}

	allModels, err := h.models.List(userID, "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp AvailableResourcesResponse

	// ── ASR models ──
	seenASR := map[string]bool{}
	for _, m := range allModels {
		if m.Type != store.ModelTypeSpeech {
			continue
		}
		// Filter by language if specified (prefix match on main language)
		if lang != "" && len(m.Langs) > 0 {
			codes := make([]language.Code, len(m.Langs))
			for i, l := range m.Langs {
				codes[i] = language.Code(l)
			}
			if !language.Match(lang, codes) {
				continue
			}
		}
		p, ok := providerByID[m.ProviderID]
		if !ok {
			continue
		}
		cat, key := extractCategory(p.Slug)
		cats := []string{}
		if cat != "" {
			cats = append(cats, cat)
		} else if c := slugCats[key]; len(c) > 0 {
			cats = c
		}
		for _, c := range cats {
			if c == "asr" && !seenASR[m.ID] {
				seenASR[m.ID] = true
				resp.ASR = append(resp.ASR, ResourceOption{ID: m.ID, Name: m.Name})
			}
		}
	}

	// ── System voices ──
	systemVoices, err := h.voices.ListAllSystem("")
	if err == nil {
		for _, v := range systemVoices {
			if !voiceMatchesLang(lang, v.Langs) {
				continue
			}
			resp.Voices = append(resp.Voices, newVoiceResource(v, v.PreviewURL))
		}
	}

	// ── 用户自建 / 复刻音色（复刻音色用参考音频当试听） ──
	if ownVoices, err := h.voices.ListByCreator(userID, ""); err == nil {
		var previews map[string]string
		if h.assets != nil {
			previews = voicePreviewURLs(c.Request.Context(), h.assets, userID, ownVoices)
		}
		for _, v := range ownVoices {
			if !voiceMatchesLang(lang, v.Langs) {
				continue
			}
			preview := v.PreviewURL
			if url, ok := previews[v.ID]; ok {
				preview = url
			}
			resp.Voices = append(resp.Voices, newVoiceResource(v, preview))
		}
	}

	if resp.ASR == nil {
		resp.ASR = []ResourceOption{}
	}
	if resp.Voices == nil {
		resp.Voices = []VoiceResource{}
	}

	c.JSON(http.StatusOK, resp)
}
