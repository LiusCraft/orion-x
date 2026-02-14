package wsserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/liuscraft/orion-x/internal/config"
)

var ErrManagerResolverNotImplemented = errors.New("manager voicebot resolver not implemented")

type VoicebotResolver interface {
	ResolveVoicebot(ctx context.Context, deviceID string) (config.VoicebotSessionConfig, string, bool, error)
}

type LocalVoicebotResolver struct {
	cfg *config.WSServerAppConfig
}

func NewLocalVoicebotResolver(cfg *config.WSServerAppConfig) *LocalVoicebotResolver {
	return &LocalVoicebotResolver{cfg: cfg}
}

func (r *LocalVoicebotResolver) ResolveVoicebot(_ context.Context, deviceID string) (config.VoicebotSessionConfig, string, bool, error) {
	if r.cfg == nil {
		return config.VoicebotSessionConfig{}, "", false, errors.New("wsserver config is nil")
	}
	if strings.TrimSpace(deviceID) == "" {
		return config.VoicebotSessionConfig{}, "", false, errors.New("device id is required")
	}
	cfg, profileID, ok := r.cfg.ResolveVoicebotForDevice(deviceID)
	if !ok {
		return config.VoicebotSessionConfig{}, "", false, nil
	}
	return cfg, profileID, true, nil
}

type ManagerVoicebotResolver struct{}

func NewManagerVoicebotResolver() *ManagerVoicebotResolver {
	return &ManagerVoicebotResolver{}
}

func (r *ManagerVoicebotResolver) ResolveVoicebot(_ context.Context, _ string) (config.VoicebotSessionConfig, string, bool, error) {
	return config.VoicebotSessionConfig{}, "", false, ErrManagerResolverNotImplemented
}

type ChainVoicebotResolver struct {
	resolvers []VoicebotResolver
}

func NewChainVoicebotResolver(resolvers ...VoicebotResolver) *ChainVoicebotResolver {
	filtered := make([]VoicebotResolver, 0, len(resolvers))
	for _, resolver := range resolvers {
		if resolver != nil {
			filtered = append(filtered, resolver)
		}
	}
	return &ChainVoicebotResolver{resolvers: filtered}
}

func (r *ChainVoicebotResolver) ResolveVoicebot(ctx context.Context, deviceID string) (config.VoicebotSessionConfig, string, bool, error) {
	if len(r.resolvers) == 0 {
		return config.VoicebotSessionConfig{}, "", false, errors.New("no voicebot resolvers configured")
	}

	for _, resolver := range r.resolvers {
		cfg, profileID, ok, err := resolver.ResolveVoicebot(ctx, deviceID)
		if err != nil {
			if errors.Is(err, ErrManagerResolverNotImplemented) {
				continue
			}
			return config.VoicebotSessionConfig{}, "", false, fmt.Errorf("resolve voicebot failed: %w", err)
		}
		if ok {
			return cfg, profileID, true, nil
		}
	}

	return config.VoicebotSessionConfig{}, "", false, nil
}
