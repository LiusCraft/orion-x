package aliyun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	tts "github.com/liuscraft/orion-x/internal/provider/tts"
)

const (
	defaultVoiceCloneEndpoint = "https://dashscope.aliyuncs.com/api/v1/services/audio/tts/customization"
	voiceEnrollmentModel      = "voice-enrollment"
)

var _ tts.VoiceCloner = (*DashScopeProvider)(nil)

func (p *DashScopeProvider) CloneVoice(ctx context.Context, req tts.VoiceCloneRequest) (*tts.VoiceCloneResult, error) {
	if p == nil {
		return nil, fmt.Errorf("aliyun voice cloner is nil")
	}

	targetModel := strings.TrimSpace(req.TargetModel)
	if targetModel == "" {
		targetModel = p.cfg.Model
	}
	prefix := strings.TrimSpace(req.Prefix)
	audioURL := strings.TrimSpace(req.SourceAudioURL)
	if targetModel == "" {
		return nil, fmt.Errorf("%w: target model is required", tts.ErrBadRequest)
	}
	if err := validateVoiceClonePrefix(prefix); err != nil {
		return nil, fmt.Errorf("%w: %v", tts.ErrBadRequest, err)
	}
	if audioURL == "" {
		return nil, fmt.Errorf("%w: source audio URL is required", tts.ErrBadRequest)
	}

	payload := voiceCloneRequestBody{
		Model: voiceEnrollmentModel,
		Input: voiceCloneInput{
			Action:         "create_voice",
			TargetModel:    targetModel,
			Prefix:         prefix,
			SourceAudioURL: audioURL,
			LanguageHints:  req.LanguageHints,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("aliyun voice clone: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, voiceCloneEndpoint(p.cfg), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("aliyun voice clone: create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: aliyun voice clone request: %v", tts.ErrTransient, err)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: aliyun voice clone response: %v", tts.ErrTransient, err)
	}

	var response voiceCloneResponse
	decodeErr := json.Unmarshal(responseBody, &response)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newVoiceCloneAPIError(resp.StatusCode, response, responseBody, decodeErr)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("aliyun voice clone: decode response: %w", decodeErr)
	}
	if strings.TrimSpace(response.Output.VoiceID) == "" {
		return nil, fmt.Errorf("aliyun voice clone: response did not contain voice_id")
	}

	return &tts.VoiceCloneResult{
		VoiceID:     response.Output.VoiceID,
		TargetModel: targetModel,
		RequestID:   response.RequestID,
	}, nil
}

type voiceCloneRequestBody struct {
	Model string          `json:"model"`
	Input voiceCloneInput `json:"input"`
}

type voiceCloneInput struct {
	Action         string   `json:"action"`
	TargetModel    string   `json:"target_model"`
	Prefix         string   `json:"prefix"`
	SourceAudioURL string   `json:"url"`
	LanguageHints  []string `json:"language_hints,omitempty"`
}

type voiceCloneResponse struct {
	Output struct {
		VoiceID string `json:"voice_id"`
	} `json:"output"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	RequestID    string `json:"request_id"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

func voiceCloneEndpoint(cfg tts.Config) string {
	if cfg.Extra != nil {
		if endpoint, ok := cfg.Extra["voice_clone_endpoint"].(string); ok && strings.TrimSpace(endpoint) != "" {
			return strings.TrimRight(strings.TrimSpace(endpoint), "/")
		}
	}

	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	const websocketPath = "/api-ws/v1/inference"
	if base, ok := strings.CutSuffix(endpoint, websocketPath); ok {
		return base + "/api/v1/services/audio/tts/customization"
	}
	if endpoint == "" {
		return defaultVoiceCloneEndpoint
	}
	return endpoint
}

func validateVoiceClonePrefix(prefix string) error {
	if prefix == "" {
		return errors.New("voice prefix is required")
	}
	if len(prefix) > 10 {
		return errors.New("voice prefix must be at most 10 characters")
	}
	for _, r := range prefix {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return errors.New("voice prefix must contain only letters and digits")
		}
	}
	return nil
}

func newVoiceCloneAPIError(status int, response voiceCloneResponse, body []byte, decodeErr error) error {
	code := strings.TrimSpace(response.Code)
	if code == "" {
		code = strings.TrimSpace(response.ErrorCode)
	}
	message := strings.TrimSpace(response.Message)
	if message == "" {
		message = strings.TrimSpace(response.ErrorMessage)
	}
	if message == "" && decodeErr == nil {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = http.StatusText(status)
	}

	cause := tts.ErrBadRequest
	retryable := false
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		cause = tts.ErrAuth
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		cause = tts.ErrTransient
		retryable = true
	}
	return &tts.APIError{
		Provider:   tts.TypeAliyun,
		StatusCode: status,
		Code:       code,
		Message:    message,
		RequestID:  response.RequestID,
		Retryable:  retryable,
		Cause:      cause,
	}
}
