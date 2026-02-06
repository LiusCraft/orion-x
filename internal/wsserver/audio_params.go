package wsserver

import (
	"fmt"
	"strings"
)

type AudioParams struct {
	Format               string `json:"format"`
	SampleRate           int    `json:"sample_rate"`
	Channels             int    `json:"channels"`
	FrameDuration        int    `json:"frame_duration"`
	BitsPerSample        int    `json:"bits_per_sample"`
	PlayBufferDurationMs int    `json:"play_buffer_duration"`
}

func NormalizeAudioParams(input *AudioParams, defaults AudioParams) AudioParams {
	if input == nil {
		return defaults
	}
	out := defaults

	format := strings.ToLower(strings.TrimSpace(input.Format))
	if format == "opus" || format == "pcm" {
		out.Format = format
	}
	if input.SampleRate == 16000 {
		out.SampleRate = input.SampleRate
	}
	if input.Channels == 1 || input.Channels == 2 {
		out.Channels = input.Channels
	}
	switch input.FrameDuration {
	case 20, 40, 60, 100:
		out.FrameDuration = input.FrameDuration
	}
	switch input.BitsPerSample {
	case 16, 24, 32:
		out.BitsPerSample = input.BitsPerSample
	}
	if out.Format == "pcm" && out.BitsPerSample != 16 {
		out.BitsPerSample = defaults.BitsPerSample
	}
	if input.PlayBufferDurationMs >= 100 {
		out.PlayBufferDurationMs = input.PlayBufferDurationMs
	}

	return out
}

func ValidateAudioParams(params AudioParams) error {
	if params.Format != "opus" && params.Format != "pcm" {
		return fmt.Errorf("audio_params.format must be opus or pcm")
	}
	if params.SampleRate != 16000 {
		return fmt.Errorf("audio_params.sample_rate must be 16000")
	}
	if params.Channels != 1 && params.Channels != 2 {
		return fmt.Errorf("audio_params.channels must be 1 or 2")
	}
	switch params.FrameDuration {
	case 20, 40, 60, 100:
	default:
		return fmt.Errorf("audio_params.frame_duration must be 20, 40, 60, or 100")
	}
	switch params.BitsPerSample {
	case 16, 24, 32:
	default:
		return fmt.Errorf("audio_params.bits_per_sample must be 16, 24, or 32")
	}
	if params.Format == "pcm" && params.BitsPerSample != 16 {
		return fmt.Errorf("audio_params.bits_per_sample must be 16 when format is pcm")
	}
	if params.PlayBufferDurationMs < 100 {
		return fmt.Errorf("audio_params.play_buffer_duration must be >= 100")
	}
	return nil
}

func FrameSize(params AudioParams) int {
	if params.SampleRate <= 0 || params.FrameDuration <= 0 {
		return 0
	}
	return params.SampleRate * params.FrameDuration / 1000
}
