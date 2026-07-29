package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

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
}

func NewAvailableHandler(providers *store.ProviderStore, models *store.AIModelStore, voices *store.ModelVoiceStore) *AvailableHandler {
	return &AvailableHandler{providers: providers, models: models, voices: voices}
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

// extractCategory extracts the explicit category from a provider slug.
// "asr/aliyun" → ("asr", "aliyun"),  "llm/openai" → ("llm", "openai"),
// "aliyun" → ("", "aliyun").
func extractCategory(slug string) (category, key string) {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) == 2 {
		switch parts[0] {
		case "asr":
			return "asr", parts[1]
		case "tts":
			return "tts", parts[1]
		case "llm":
			return "llm", parts[1]
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
			// Filter by language (prefix match on main language)
			if lang != "" && len(v.Langs) > 0 {
				codes := make([]language.Code, len(v.Langs))
				for i, l := range v.Langs {
					codes[i] = language.Code(l)
				}
				if !language.Match(lang, codes) {
					continue
				}
			}
			r := VoiceResource{
				ID:             v.ID,
				Name:           v.Name,
				Description:    v.Description,
				Gender:         string(v.Gender),
				AvatarURL:      v.AvatarURL,
				PreviewURL:     v.PreviewURL,
				Langs:          v.Langs,
				Emotions:       v.Emotions,
				IsSystem:       v.IsSystem,
				IsCloned:       v.IsCloned,
				SourceAudioURL: v.SourceAudioURL,
			}
			if v.Tags != nil {
				r.Tags = v.Tags
			}
			resp.Voices = append(resp.Voices, r)
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
