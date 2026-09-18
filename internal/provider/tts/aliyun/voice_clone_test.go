package aliyun

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tts "github.com/liuscraft/orion-x/internal/provider/tts"
)

func TestDashScopeProviderImplementsVoiceCloner(t *testing.T) {
	var _ tts.VoiceCloner = (*DashScopeProvider)(nil)
}

func TestDashScopeProviderCloneVoice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/services/audio/tts/customization" {
			t.Errorf("path = %s, want /api/v1/services/audio/tts/customization", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q, want Bearer test-key", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content type = %q, want application/json", got)
		}

		var body voiceCloneRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if body.Model != voiceEnrollmentModel {
			t.Errorf("model = %q, want %q", body.Model, voiceEnrollmentModel)
		}
		if body.Input.Action != "create_voice" {
			t.Errorf("action = %q, want create_voice", body.Input.Action)
		}
		if body.Input.TargetModel != "cosyvoice-v3-flash" {
			t.Errorf("target model = %q, want cosyvoice-v3-flash", body.Input.TargetModel)
		}
		if body.Input.Prefix != "myvoice" {
			t.Errorf("prefix = %q, want myvoice", body.Input.Prefix)
		}
		if body.Input.SourceAudioURL != "https://example.com/voice.wav" {
			t.Errorf("source audio URL = %q, want source URL", body.Input.SourceAudioURL)
		}
		if len(body.Input.LanguageHints) != 1 || body.Input.LanguageHints[0] != "en" {
			t.Errorf("language hints = %#v, want [en]", body.Input.LanguageHints)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"voice_id":"cosyvoice-v3-flash-myvoice-abc123"},"request_id":"request-1"}`))
	}))
	defer server.Close()

	provider, err := NewDashScopeProvider(tts.Config{
		APIKey:   "test-key",
		Endpoint: server.URL + "/api-ws/v1/inference",
		Model:    "cosyvoice-v3-flash",
	})
	if err != nil {
		t.Fatalf("NewDashScopeProvider() error = %v", err)
	}

	result, err := provider.CloneVoice(context.Background(), tts.VoiceCloneRequest{
		Prefix:         "myvoice",
		SourceAudioURL: "https://example.com/voice.wav",
		LanguageHints:  []string{"en"},
	})
	if err != nil {
		t.Fatalf("CloneVoice() error = %v", err)
	}
	if result.VoiceID != "cosyvoice-v3-flash-myvoice-abc123" {
		t.Errorf("voice ID = %q, want cloned voice ID", result.VoiceID)
	}
	if result.TargetModel != "cosyvoice-v3-flash" {
		t.Errorf("target model = %q, want cosyvoice-v3-flash", result.TargetModel)
	}
	if result.RequestID != "request-1" {
		t.Errorf("request ID = %q, want request-1", result.RequestID)
	}
}

func TestDashScopeProviderCloneVoiceRejectsInvalidRequest(t *testing.T) {
	provider, err := NewDashScopeProvider(tts.Config{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewDashScopeProvider() error = %v", err)
	}

	tests := []struct {
		name string
		req  tts.VoiceCloneRequest
	}{
		{
			name: "missing prefix",
			req:  tts.VoiceCloneRequest{SourceAudioURL: "https://example.com/voice.wav"},
		},
		{
			name: "invalid prefix",
			req: tts.VoiceCloneRequest{
				Prefix:         "my-voice",
				SourceAudioURL: "https://example.com/voice.wav",
			},
		},
		{
			name: "missing source audio",
			req:  tts.VoiceCloneRequest{Prefix: "myvoice"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := provider.CloneVoice(context.Background(), tt.req)
			if !errors.Is(err, tts.ErrBadRequest) {
				t.Fatalf("CloneVoice() error = %v, want ErrBadRequest", err)
			}
		})
	}
}

func TestDashScopeProviderCloneVoiceMapsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"InvalidApiKey","message":"bad key","request_id":"request-2"}`))
	}))
	defer server.Close()

	provider, err := NewDashScopeProvider(tts.Config{
		APIKey: "bad-key",
		Extra:  map[string]any{"voice_clone_endpoint": server.URL},
	})
	if err != nil {
		t.Fatalf("NewDashScopeProvider() error = %v", err)
	}

	_, err = provider.CloneVoice(context.Background(), tts.VoiceCloneRequest{
		Prefix:         "myvoice",
		SourceAudioURL: "https://example.com/voice.wav",
	})
	if !errors.Is(err, tts.ErrAuth) {
		t.Fatalf("CloneVoice() error = %v, want ErrAuth", err)
	}

	var apiErr *tts.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("CloneVoice() error type = %T, want *tts.APIError", err)
	}
	if apiErr.Code != "InvalidApiKey" || apiErr.RequestID != "request-2" {
		t.Errorf("API error = %+v, want code and request ID", apiErr)
	}
}

func TestVoiceCloneEndpointUsesConfiguredOverride(t *testing.T) {
	endpoint := voiceCloneEndpoint(tts.Config{Extra: map[string]any{
		"voice_clone_endpoint": " https://example.com/custom/ ",
	}})
	if endpoint != "https://example.com/custom" {
		t.Fatalf("voiceCloneEndpoint() = %q, want trimmed override", endpoint)
	}

	if strings.TrimSpace(endpoint) != endpoint {
		t.Fatalf("voiceCloneEndpoint() returned surrounding whitespace")
	}
}

func TestVoiceCloneEndpointDerivesRESTFromWebSocketURL(t *testing.T) {
	const restPath = "/api/v1/services/audio/tts/customization"

	tests := []struct {
		name string
		cfg  tts.Config
		want string
	}{
		{
			name: "wss inference endpoint switches to https",
			cfg:  tts.Config{Endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference"},
			want: "https://dashscope.aliyuncs.com" + restPath,
		},
		{
			name: "ws inference endpoint switches to http",
			cfg:  tts.Config{Endpoint: "ws://127.0.0.1:8080/api-ws/v1/inference"},
			want: "http://127.0.0.1:8080" + restPath,
		},
		{
			name: "wss base without inference path switches to https",
			cfg:  tts.Config{Endpoint: "wss://dashscope.aliyuncs.com/"},
			want: "https://dashscope.aliyuncs.com",
		},
		{
			name: "explicit override keeps its scheme but normalises ws",
			cfg:  tts.Config{Extra: map[string]any{"voice_clone_endpoint": "wss://clone.example.com/api/v1/services/audio/tts/customization"}},
			want: "https://clone.example.com" + restPath,
		},
		{
			name: "http endpoint is left as is",
			cfg:  tts.Config{Endpoint: "https://example.com" + restPath},
			want: "https://example.com" + restPath,
		},
		{
			name: "empty endpoint falls back to the default",
			cfg:  tts.Config{},
			want: defaultVoiceCloneEndpoint,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := voiceCloneEndpoint(tc.cfg); got != tc.want {
				t.Fatalf("voiceCloneEndpoint() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDashScopeProviderCloneVoiceFromWebSocketEndpoint 回归：模型里配的是 WS 推理地址
// （wss://…/api-ws/v1/inference）时，复刻请求必须发到同一主机的 HTTPS REST 地址。
func TestDashScopeProviderCloneVoiceFromWebSocketEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/services/audio/tts/customization" {
			t.Errorf("path = %s, want voice-enrollment REST path", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"voice_id":"cloned-voice"}}`))
	}))
	defer server.Close()

	provider, err := NewDashScopeProvider(tts.Config{
		APIKey:   "test-key",
		Model:    "cosyvoice-v3-flash",
		Endpoint: "ws" + strings.TrimPrefix(server.URL, "http") + "/api-ws/v1/inference",
	})
	if err != nil {
		t.Fatalf("NewDashScopeProvider() error = %v", err)
	}

	result, err := provider.CloneVoice(context.Background(), tts.VoiceCloneRequest{
		Prefix:         "myvoice",
		SourceAudioURL: "https://example.com/voice.wav",
	})
	if err != nil {
		t.Fatalf("CloneVoice() error = %v", err)
	}
	if result.VoiceID != "cloned-voice" {
		t.Fatalf("voice ID = %q, want cloned-voice", result.VoiceID)
	}
}
