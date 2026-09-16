package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	ttsprovider "github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/store"
)

const (
	testVoiceCloneProviderType = "manager-voice-clone-test"
	testVoiceCloneTargetModel  = "manager-voice-clone-model"
)

type fakeVoiceCloneModelStore struct {
	list     []store.AIModel
	byID     map[string]*store.AIModel
	listErr  error
	getErr   error
	listUser string
	listType store.ModelType
	listLang string
}

func (s *fakeVoiceCloneModelStore) List(userID string, modelType store.ModelType, lang string) ([]store.AIModel, error) {
	s.listUser = userID
	s.listType = modelType
	s.listLang = lang
	return s.list, s.listErr
}

func (s *fakeVoiceCloneModelStore) GetByID(id string) (*store.AIModel, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	model, ok := s.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return model, nil
}

type fakeVoiceCloneVoiceStore struct {
	params store.CloneVoiceParams
	result *store.ModelVoice
	err    error
}

func (s *fakeVoiceCloneVoiceStore) CreateCloned(params store.CloneVoiceParams) (*store.ModelVoice, error) {
	s.params = params
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &store.ModelVoice{ID: "stored-voice", VoiceID: params.VoiceID}, nil
}

type fakeVoiceCloner struct {
	request ttsprovider.VoiceCloneRequest
	result  *ttsprovider.VoiceCloneResult
	err     error
}

func (p *fakeVoiceCloner) Synthesize(context.Context, ttsprovider.SynthesizeRequest) (*ttsprovider.SynthesizeResult, error) {
	return nil, errors.New("not implemented")
}

func (p *fakeVoiceCloner) CloneVoice(_ context.Context, req ttsprovider.VoiceCloneRequest) (*ttsprovider.VoiceCloneResult, error) {
	p.request = req
	return p.result, p.err
}

func registerVoiceCloneTestProvider() {
	ttsprovider.Register(testVoiceCloneProviderType, func(ttsprovider.Config) (ttsprovider.Synthesizer, error) {
		return &fakeVoiceCloner{}, nil
	}, ttsprovider.ProviderMeta{
		Name: "Manager voice clone test provider",
		Models: map[string]ttsprovider.ModelInfo{
			testVoiceCloneTargetModel: {},
		},
		Features: []ttsprovider.Feature{ttsprovider.FeatureVoiceCloning},
	})
}

func testVoiceCloneModel(id, name, owner, apiKey string) store.AIModel {
	return store.AIModel{
		ID:         id,
		ProviderID: "provider-1",
		Name:       name,
		Type:       store.ModelTypeSpeech,
		BaseURL:    "wss://model.example.test/api-ws/v1/inference",
		ModelID:    testVoiceCloneTargetModel,
		Provider: &store.Provider{
			ID:        "provider-1",
			Name:      "Test TTS",
			Slug:      "tts/" + testVoiceCloneProviderType,
			BaseURL:   "wss://provider.example.test/api-ws/v1/inference",
			APIKeyEnc: apiKey,
			Extra: map[string]any{
				"workspace": "provider-workspace",
			},
		},
		Langs: pq.StringArray{"zh", "en"},
		Extra: map[string]any{
			"workspace":            "model-workspace",
			"voice_clone_endpoint": "https://clone.example.test/v1",
		},
		BaseModel: store.BaseModel{Creator: owner},
	}
}

func TestProviderVoiceCloneServiceListModels(t *testing.T) {
	registerVoiceCloneTestProvider()

	configured := testVoiceCloneModel("model-configured", "Configured", "user-1", "api-key")
	unconfigured := testVoiceCloneModel("model-unconfigured", "Unconfigured", "user-1", "")
	wrongCategory := testVoiceCloneModel("model-asr", "ASR", "user-1", "api-key")
	wrongCategory.Provider.Slug = "asr/" + testVoiceCloneProviderType
	unknownTarget := testVoiceCloneModel("model-unknown", "Unknown", "user-1", "api-key")
	unknownTarget.ModelID = "unknown-model"

	models := &fakeVoiceCloneModelStore{list: []store.AIModel{wrongCategory, unknownTarget, unconfigured, configured}}
	service := newProviderVoiceCloneService(models, &fakeVoiceCloneVoiceStore{}, nil)

	got, err := service.ListModels("user-1")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if models.listUser != "user-1" || models.listType != store.ModelTypeSpeech || models.listLang != "" {
		t.Fatalf("List() arguments = (%q, %q, %q), want (%q, %q, %q)", models.listUser, models.listType, models.listLang, "user-1", store.ModelTypeSpeech, "")
	}
	if len(got) != 2 {
		t.Fatalf("ListModels() returned %d models, want 2: %#v", len(got), got)
	}
	if got[0].ID != configured.ID || !got[0].Configured {
		t.Fatalf("first model = %#v, want configured cloneable model", got[0])
	}
	if got[1].ID != unconfigured.ID || got[1].Configured {
		t.Fatalf("second model = %#v, want unconfigured cloneable model", got[1])
	}
	if got[0].ProviderSlug != "tts/"+testVoiceCloneProviderType {
		t.Errorf("provider slug = %q", got[0].ProviderSlug)
	}
}

func TestProviderVoiceCloneServiceClone(t *testing.T) {
	registerVoiceCloneTestProvider()

	model := testVoiceCloneModel("model-1", "Cloneable", "user-1", "api-key")
	models := &fakeVoiceCloneModelStore{byID: map[string]*store.AIModel{model.ID: &model}}
	voices := &fakeVoiceCloneVoiceStore{result: &store.ModelVoice{ID: "stored-voice", VoiceID: "provider-voice"}}
	cloner := &fakeVoiceCloner{result: &ttsprovider.VoiceCloneResult{VoiceID: " provider-voice ", TargetModel: testVoiceCloneTargetModel}}
	var providerConfig ttsprovider.ProviderConfig
	service := newProviderVoiceCloneService(models, voices, func(cfg ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error) {
		providerConfig = cfg
		return cloner, nil
	})

	got, err := service.Clone(context.Background(), model.ID, "user-1", cloneVoiceRequest{
		Name:           "  My cloned voice  ",
		Prefix:         "voice123",
		SourceAudioURL: " https://audio.example.test/sample.m4a ",
		Format:         "M4A",
		Langs:          pq.StringArray{"zh-CN", "zh"},
		Extra:          map[string]any{"noise_reduction": true},
	})
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	if got.ID != "stored-voice" {
		t.Fatalf("Clone() voice ID = %q, want stored-voice", got.ID)
	}

	if providerConfig.Type != testVoiceCloneProviderType {
		t.Errorf("provider type = %q, want %q", providerConfig.Type, testVoiceCloneProviderType)
	}
	if providerConfig.Config.APIKey != "api-key" {
		t.Errorf("provider API key = %q", providerConfig.Config.APIKey)
	}
	if providerConfig.Config.Endpoint != model.BaseURL {
		t.Errorf("provider endpoint = %q, want model endpoint %q", providerConfig.Config.Endpoint, model.BaseURL)
	}
	wantConfigExtra := map[string]any{
		"workspace":            "model-workspace",
		"voice_clone_endpoint": "https://clone.example.test/v1",
	}
	if !reflect.DeepEqual(providerConfig.Config.Extra, wantConfigExtra) {
		t.Errorf("provider extra = %#v, want %#v", providerConfig.Config.Extra, wantConfigExtra)
	}

	if cloner.request.TargetModel != testVoiceCloneTargetModel {
		t.Errorf("target model = %q, want %q", cloner.request.TargetModel, testVoiceCloneTargetModel)
	}
	if cloner.request.Prefix != "voice123" {
		t.Errorf("prefix = %q, want voice123", cloner.request.Prefix)
	}
	if cloner.request.Format != ttsprovider.FormatM4A {
		t.Errorf("format = %q, want %q", cloner.request.Format, ttsprovider.FormatM4A)
	}
	if cloner.request.SourceAudioURL != "https://audio.example.test/sample.m4a" {
		t.Errorf("source audio URL = %q", cloner.request.SourceAudioURL)
	}
	if !reflect.DeepEqual(cloner.request.LanguageHints, []string{"zh"}) {
		t.Errorf("language hints = %#v, want [zh]", cloner.request.LanguageHints)
	}
	if !reflect.DeepEqual(cloner.request.Extra, map[string]any{"noise_reduction": true}) {
		t.Errorf("request extra = %#v", cloner.request.Extra)
	}

	if voices.params.ModelID != model.ID || voices.params.VoiceID != "provider-voice" {
		t.Errorf("stored voice params = %#v", voices.params)
	}
	if voices.params.Name != "My cloned voice" || voices.params.Creator != "user-1" {
		t.Errorf("stored metadata = %#v", voices.params)
	}
	if !reflect.DeepEqual(voices.params.Langs, pq.StringArray{"zh"}) {
		t.Errorf("stored languages = %#v, want [zh]", voices.params.Langs)
	}
}

func TestVoiceCloneHandlers(t *testing.T) {
	registerVoiceCloneTestProvider()

	gin.SetMode(gin.TestMode)
	model := testVoiceCloneModel("model-1", "Cloneable", "user-1", "api-key")
	models := &fakeVoiceCloneModelStore{
		list: []store.AIModel{model},
		byID: map[string]*store.AIModel{model.ID: &model},
	}
	voices := &fakeVoiceCloneVoiceStore{result: &store.ModelVoice{ID: "stored-voice", VoiceID: "provider-voice", Name: "My voice"}}
	cloner := &fakeVoiceCloner{result: &ttsprovider.VoiceCloneResult{VoiceID: "provider-voice"}}
	service := newProviderVoiceCloneService(models, voices, func(ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error) {
		return cloner, nil
	})
	handler := &VoiceHandler{cloneService: service}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", "user-1")
		c.Next()
	})
	router.GET("/api/models/voice-cloning", handler.ListCloneableModels)
	router.POST("/api/models/:id/voices/clone", handler.Clone)

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/models/voice-cloning", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	var cloneable []VoiceCloneModel
	if err := json.Unmarshal(listResponse.Body.Bytes(), &cloneable); err != nil {
		t.Fatalf("decode cloneable models: %v", err)
	}
	if len(cloneable) != 1 || cloneable[0].ID != model.ID || !cloneable[0].Configured {
		t.Fatalf("cloneable models = %#v", cloneable)
	}

	body := strings.NewReader(`{"name":"My voice","source_audio_url":"https://audio.example.test/sample.wav","format":"wav","target_model":"must-not-override-path"}`)
	cloneResponse := httptest.NewRecorder()
	router.ServeHTTP(cloneResponse, httptest.NewRequest(http.MethodPost, "/api/models/model-1/voices/clone", body))
	if cloneResponse.Code != http.StatusCreated {
		t.Fatalf("clone status = %d, body = %s", cloneResponse.Code, cloneResponse.Body.String())
	}
	if cloner.request.TargetModel != testVoiceCloneTargetModel {
		t.Errorf("target model = %q, want path model target %q", cloner.request.TargetModel, testVoiceCloneTargetModel)
	}
	if cloner.request.Format != ttsprovider.FormatWAV {
		t.Errorf("format = %q, want wav", cloner.request.Format)
	}

	missingFormat := httptest.NewRecorder()
	missingFormatBody := strings.NewReader(`{"name":"My voice","source_audio_url":"https://audio.example.test/sample.wav"}`)
	router.ServeHTTP(missingFormat, httptest.NewRequest(http.MethodPost, "/api/models/model-1/voices/clone", missingFormatBody))
	if missingFormat.Code != http.StatusBadRequest {
		t.Errorf("missing format status = %d, body = %s", missingFormat.Code, missingFormat.Body.String())
	}
}

func TestProviderVoiceCloneServiceRejectsForeignModel(t *testing.T) {
	registerVoiceCloneTestProvider()

	model := testVoiceCloneModel("model-1", "Private", "another-user", "api-key")
	models := &fakeVoiceCloneModelStore{byID: map[string]*store.AIModel{model.ID: &model}}
	service := newProviderVoiceCloneService(models, &fakeVoiceCloneVoiceStore{}, func(ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error) {
		t.Fatal("provider constructor must not be called")
		return nil, nil
	})

	_, err := service.Clone(context.Background(), model.ID, "user-1", cloneVoiceRequest{})
	if !errors.Is(err, errVoiceCloneForbidden) {
		t.Fatalf("Clone() error = %v, want forbidden", err)
	}
}
