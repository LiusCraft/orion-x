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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"github.com/liuscraft/orion-x/internal/assets"
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

type fakeVoiceCloneAssets struct {
	asset      *store.Asset
	getErr     error
	presignURL string
	presignErr error
	gotOwner   string
	gotAssetID string
	gotTTL     time.Duration
}

func (a *fakeVoiceCloneAssets) Get(_ context.Context, ownerID, assetID string) (*store.Asset, error) {
	a.gotOwner, a.gotAssetID = ownerID, assetID
	if a.getErr != nil {
		return nil, a.getErr
	}
	return a.asset, nil
}

func (a *fakeVoiceCloneAssets) PresignURLWithTTL(_ context.Context, _ *store.Asset, ttl time.Duration, _ bool) (string, error) {
	a.gotTTL = ttl
	if a.presignErr != nil {
		return "", a.presignErr
	}
	return a.presignURL, nil
}

func voiceSampleAsset(id, name string) *store.Asset {
	return &store.Asset{ID: id, OwnerID: "user-1", Purpose: string(assets.PurposeVoiceSample), Name: name}
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
			Slug:      "tts:" + testVoiceCloneProviderType,
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
	wrongCategory.Provider.Slug = "asr:" + testVoiceCloneProviderType
	unknownTarget := testVoiceCloneModel("model-unknown", "Unknown", "user-1", "api-key")
	unknownTarget.ModelID = "unknown-model"

	models := &fakeVoiceCloneModelStore{list: []store.AIModel{wrongCategory, unknownTarget, unconfigured, configured}}
	service := newProviderVoiceCloneService(models, &fakeVoiceCloneVoiceStore{}, nil, nil)

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
	if got[0].ProviderSlug != "tts:"+testVoiceCloneProviderType {
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
	service := newProviderVoiceCloneService(models, voices, nil, func(cfg ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error) {
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
	if voices.params.SourceAssetID != "" {
		t.Errorf("legacy URL clone must not store an asset ID: %#v", voices.params)
	}
	if voices.params.SourceAudioURL != "https://audio.example.test/sample.m4a" {
		t.Errorf("stored source URL = %q", voices.params.SourceAudioURL)
	}
	if !reflect.DeepEqual(voices.params.Langs, pq.StringArray{"zh"}) {
		t.Errorf("stored languages = %#v, want [zh]", voices.params.Langs)
	}
}

// TestProviderVoiceCloneServiceCloneFromAsset 参考音频来自资源库：由后台现算预签名
// URL 交给厂商，库里只存 asset_id（URL 会过期，不入库）。
func TestProviderVoiceCloneServiceCloneFromAsset(t *testing.T) {
	registerVoiceCloneTestProvider()

	model := testVoiceCloneModel("model-1", "Cloneable", "user-1", "api-key")
	models := &fakeVoiceCloneModelStore{byID: map[string]*store.AIModel{model.ID: &model}}
	voices := &fakeVoiceCloneVoiceStore{}
	assetSvc := &fakeVoiceCloneAssets{
		asset:      voiceSampleAsset("asset-1", "我的录音.M4A"),
		presignURL: "https://storage.test/sample?signature=stub",
	}
	cloner := &fakeVoiceCloner{result: &ttsprovider.VoiceCloneResult{VoiceID: "provider-voice"}}
	service := newProviderVoiceCloneService(models, voices, assetSvc, func(ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error) {
		return cloner, nil
	})

	if _, err := service.Clone(context.Background(), model.ID, "user-1", cloneVoiceRequest{
		Name:          "My voice",
		SourceAssetID: " asset-1 ",
	}); err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	if assetSvc.gotOwner != "user-1" || assetSvc.gotAssetID != "asset-1" {
		t.Errorf("asset lookup = (%q, %q), want (user-1, asset-1)", assetSvc.gotOwner, assetSvc.gotAssetID)
	}
	if assetSvc.gotTTL != voiceSamplePresignTTL {
		t.Errorf("presign ttl = %v, want %v", assetSvc.gotTTL, voiceSamplePresignTTL)
	}
	if cloner.request.SourceAudioURL != assetSvc.presignURL {
		t.Errorf("provider URL = %q, want presigned URL", cloner.request.SourceAudioURL)
	}
	if cloner.request.Format != ttsprovider.FormatM4A {
		t.Errorf("format = %q, want m4a derived from file name", cloner.request.Format)
	}
	if voices.params.SourceAssetID != "asset-1" {
		t.Errorf("stored asset ID = %q, want asset-1", voices.params.SourceAssetID)
	}
	if voices.params.SourceAudioURL != "" {
		t.Errorf("presigned URL must not be persisted, got %q", voices.params.SourceAudioURL)
	}
}

func TestProviderVoiceCloneServiceRejectsInvalidSourceAsset(t *testing.T) {
	registerVoiceCloneTestProvider()

	cases := []struct {
		name    string
		assets  voiceAssetStore
		wantErr error
	}{
		{
			name:    "storage not configured",
			assets:  nil,
			wantErr: errVoiceCloneStorageUnconfigured,
		},
		{
			name:    "asset not found",
			assets:  &fakeVoiceCloneAssets{getErr: assets.ErrNotFound},
			wantErr: assets.ErrNotFound,
		},
		{
			name:    "foreign asset",
			assets:  &fakeVoiceCloneAssets{getErr: assets.ErrForbidden},
			wantErr: assets.ErrForbidden,
		},
		{
			name:    "wrong purpose",
			assets:  &fakeVoiceCloneAssets{asset: &store.Asset{ID: "a1", OwnerID: "user-1", Purpose: string(assets.PurposeKBDocument), Name: "doc.md"}},
			wantErr: ttsprovider.ErrBadRequest,
		},
		{
			name:    "presign failure",
			assets:  &fakeVoiceCloneAssets{asset: voiceSampleAsset("a1", "sample.wav"), presignErr: assets.ErrStorage},
			wantErr: assets.ErrStorage,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := testVoiceCloneModel("model-1", "Cloneable", "user-1", "api-key")
			models := &fakeVoiceCloneModelStore{byID: map[string]*store.AIModel{model.ID: &model}}
			service := newProviderVoiceCloneService(models, &fakeVoiceCloneVoiceStore{}, tc.assets, func(ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error) {
				t.Fatal("provider constructor must not be called")
				return nil, nil
			})

			_, err := service.Clone(context.Background(), model.ID, "user-1", cloneVoiceRequest{Name: "My voice", SourceAssetID: "asset-1"})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Clone() error = %v, want %v", err, tc.wantErr)
			}
		})
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
	assetSvc := &fakeVoiceCloneAssets{asset: voiceSampleAsset("asset-1", "sample.wav"), presignURL: "https://storage.test/sample.wav?signature=stub"}
	service := newProviderVoiceCloneService(models, voices, assetSvc, func(ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error) {
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

	// 资源库参考音频：format 由资源文件名推导，无需调用方传入。
	fromAsset := httptest.NewRecorder()
	fromAssetBody := strings.NewReader(`{"name":"From asset","source_asset_id":"asset-1","format":"mp3","langs":["zh"]}`)
	router.ServeHTTP(fromAsset, httptest.NewRequest(http.MethodPost, "/api/models/model-1/voices/clone", fromAssetBody))
	if fromAsset.Code != http.StatusCreated {
		t.Fatalf("clone from asset status = %d, body = %s", fromAsset.Code, fromAsset.Body.String())
	}
	if cloner.request.SourceAudioURL != assetSvc.presignURL {
		t.Errorf("provider URL = %q, want presigned URL", cloner.request.SourceAudioURL)
	}
	if cloner.request.Format != ttsprovider.FormatWAV {
		t.Errorf("format = %q, want wav derived from asset name", cloner.request.Format)
	}
	if voices.params.SourceAssetID != "asset-1" {
		t.Errorf("stored asset ID = %q, want asset-1", voices.params.SourceAssetID)
	}

	for name, body := range map[string]string{
		"both sources": `{"name":"x","source_asset_id":"asset-1","source_audio_url":"https://audio.example.test/a.wav","format":"wav"}`,
		"no source":    `{"name":"x"}`,
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/models/model-1/voices/clone", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400, body = %s", name, recorder.Code, recorder.Body.String())
		}
	}

	missingAsset := httptest.NewRecorder()
	assetSvc.getErr = assets.ErrNotFound
	router.ServeHTTP(missingAsset, httptest.NewRequest(http.MethodPost, "/api/models/model-1/voices/clone", strings.NewReader(`{"name":"x","source_asset_id":"gone"}`)))
	if missingAsset.Code != http.StatusNotFound {
		t.Errorf("missing asset status = %d, want 404, body = %s", missingAsset.Code, missingAsset.Body.String())
	}
}

func TestProviderVoiceCloneServiceRejectsForeignModel(t *testing.T) {
	registerVoiceCloneTestProvider()

	model := testVoiceCloneModel("model-1", "Private", "another-user", "api-key")
	models := &fakeVoiceCloneModelStore{byID: map[string]*store.AIModel{model.ID: &model}}
	service := newProviderVoiceCloneService(models, &fakeVoiceCloneVoiceStore{}, nil, func(ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error) {
		t.Fatal("provider constructor must not be called")
		return nil, nil
	})

	_, err := service.Clone(context.Background(), model.ID, "user-1", cloneVoiceRequest{})
	if !errors.Is(err, errVoiceCloneForbidden) {
		t.Fatalf("Clone() error = %v, want forbidden", err)
	}
}
