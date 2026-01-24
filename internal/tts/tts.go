package tts

import (
	"context"
	"errors"
	"io"
)

type Config struct {
	APIKey               string
	Endpoint             string
	Workspace            string
	Model                string
	Voice                string
	Format               string
	SampleRate           int
	Volume               int
	Rate                 float64
	Pitch                float64
	EnableSSML           bool
	TextType             string
	EnableDataInspection *bool
}

type Provider interface {
	Start(ctx context.Context, cfg Config) (Stream, error)
}

type Stream interface {
	WriteTextChunk(ctx context.Context, text string) error
	Close(ctx context.Context) error
	AudioReader() io.ReadCloser
}

var (
	ErrTransient  = errors.New("tts transient error")
	ErrAuth       = errors.New("tts auth error")
	ErrBadRequest = errors.New("tts bad request")
)
