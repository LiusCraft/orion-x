package asr

import (
	"context"
	"errors"
)

var (
	ErrAPIKeyRequired = errors.New("DASHSCOPE_API_KEY is required")
)

type Config struct {
	APIKey                     string
	Endpoint                   string
	Model                      string
	Format                     string
	SampleRate                 int
	VocabularyID               string
	SemanticPunctuationEnabled *bool
	MaxSentenceSilence         int
	MultiThresholdModeEnabled  *bool
	Heartbeat                  *bool
	LanguageHints              []string
}

type Result struct {
	Text          string
	IsFinal       bool
	BeginTimeMs   int64
	EndTimeMs     *int64
	UsageDuration *int
}

type Recognizer interface {
	Start(ctx context.Context) error
	SendAudio(ctx context.Context, data []byte) error
	Finish(ctx context.Context) error
	Close() error
	OnResult(handler func(Result))
}
