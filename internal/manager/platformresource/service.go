package platformresource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

var (
	resourceKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-_]{2,127}$`)
	providerPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)

	supportedProvidersByCategory = map[contracts.ResourceCategory]map[string]struct{}{
		contracts.ResourceLLM: {
			ProviderDashScope: {},
			ProviderOpenAI:    {},
			ProviderZhipu:     {},
		},
		contracts.ResourceASR: {
			ProviderDashScope: {},
		},
		contracts.ResourceTTS: {
			ProviderDashScope: {},
		},
	}
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, createdBy uuid.UUID, input CreateInput) (Resource, error) {
	if err := s.validateReady(); err != nil {
		return Resource{}, err
	}
	if createdBy == uuid.Nil {
		return Resource{}, fmt.Errorf("%w: created_by is required", ErrInvalidArgument)
	}

	category, err := parseCategory(input.Category)
	if err != nil {
		return Resource{}, err
	}
	provider, err := normalizeProvider(input.Provider)
	if err != nil {
		return Resource{}, err
	}
	if err := validateProviderForCategory(category, provider); err != nil {
		return Resource{}, err
	}

	resourceKey, err := normalizeResourceKey(input.ResourceKey)
	if err != nil {
		return Resource{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Resource{}, fmt.Errorf("%w: name is required", ErrInvalidArgument)
	}
	if input.SchemaVersion <= 0 {
		return Resource{}, fmt.Errorf("%w: schema_version must be > 0", ErrInvalidArgument)
	}

	capabilities, err := normalizeJSONObject(input.Capabilities, "capabilities")
	if err != nil {
		return Resource{}, err
	}
	config, err := normalizeJSONObject(input.Config, "config")
	if err != nil {
		return Resource{}, err
	}

	credentialRef := strings.TrimSpace(input.CredentialRef)
	if credentialRef == "" {
		return Resource{}, fmt.Errorf("%w: credential_ref is required", ErrInvalidArgument)
	}

	status, err := parseCreateStatus(input.Status)
	if err != nil {
		return Resource{}, err
	}

	resource := Resource{
		ID:            uuid.New(),
		Category:      category,
		Provider:      provider,
		ResourceKey:   resourceKey,
		Name:          name,
		SchemaVersion: input.SchemaVersion,
		Capabilities:  capabilities,
		Config:        config,
		CredentialRef: credentialRef,
		Status:        status,
		CreatedBy:     createdBy,
	}

	created, err := s.repo.Create(ctx, resource)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Resource{}, ErrConflict
		}
		if errors.Is(err, ErrInvalidArgument) {
			return Resource{}, err
		}
		return Resource{}, fmt.Errorf("create platform resource: %w", err)
	}

	return created, nil
}

func (s *Service) List(ctx context.Context, input ListInput) ([]Resource, error) {
	if err := s.validateReady(); err != nil {
		return nil, err
	}

	filter := ListFilter{}
	if strings.TrimSpace(input.Category) != "" {
		category, err := parseCategory(input.Category)
		if err != nil {
			return nil, err
		}
		filter.Category = &category
	}

	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	if provider != "" {
		if !providerPattern.MatchString(provider) {
			return nil, fmt.Errorf("%w: provider %q has invalid format", ErrInvalidArgument, input.Provider)
		}
		if _, ok := allSupportedProviders()[provider]; !ok {
			return nil, fmt.Errorf("%w: provider %q is not supported", ErrInvalidArgument, provider)
		}
		filter.Provider = provider
	}

	if strings.TrimSpace(input.Status) != "" {
		status, err := parseStatus(input.Status)
		if err != nil {
			return nil, err
		}
		filter.Status = &status
	}

	if filter.Category != nil && filter.Provider != "" {
		if err := validateProviderForCategory(*filter.Category, filter.Provider); err != nil {
			return nil, err
		}
	}

	resources, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list platform resources: %w", err)
	}

	return resources, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (Resource, error) {
	if err := s.validateReady(); err != nil {
		return Resource{}, err
	}
	if id == uuid.Nil {
		return Resource{}, fmt.Errorf("%w: id is required", ErrInvalidArgument)
	}
	if !input.HasChanges() {
		return Resource{}, fmt.Errorf("%w: at least one field is required", ErrInvalidArgument)
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Resource{}, ErrNotFound
		}
		return Resource{}, fmt.Errorf("load platform resource: %w", err)
	}

	patch := UpdatePatch{}
	effectiveCategory := existing.Category
	effectiveProvider := existing.Provider

	if input.Category != nil {
		category, parseErr := parseCategory(*input.Category)
		if parseErr != nil {
			return Resource{}, parseErr
		}
		effectiveCategory = category
		patch.Category = &category
	}

	if input.Provider != nil {
		provider, parseErr := normalizeProvider(*input.Provider)
		if parseErr != nil {
			return Resource{}, parseErr
		}
		effectiveProvider = provider
		patch.Provider = &provider
	}

	if err := validateProviderForCategory(effectiveCategory, effectiveProvider); err != nil {
		return Resource{}, err
	}

	if input.ResourceKey != nil {
		resourceKey, parseErr := normalizeResourceKey(*input.ResourceKey)
		if parseErr != nil {
			return Resource{}, parseErr
		}
		patch.ResourceKey = &resourceKey
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return Resource{}, fmt.Errorf("%w: name is required", ErrInvalidArgument)
		}
		patch.Name = &name
	}

	if input.SchemaVersion != nil {
		if *input.SchemaVersion <= 0 {
			return Resource{}, fmt.Errorf("%w: schema_version must be > 0", ErrInvalidArgument)
		}
		schemaVersion := *input.SchemaVersion
		patch.SchemaVersion = &schemaVersion
	}

	if input.Capabilities != nil {
		capabilities, parseErr := normalizeJSONObject(*input.Capabilities, "capabilities")
		if parseErr != nil {
			return Resource{}, parseErr
		}
		patch.Capabilities = &capabilities
	}

	if input.Config != nil {
		config, parseErr := normalizeJSONObject(*input.Config, "config")
		if parseErr != nil {
			return Resource{}, parseErr
		}
		patch.Config = &config
	}

	if input.CredentialRef != nil {
		credentialRef := strings.TrimSpace(*input.CredentialRef)
		if credentialRef == "" {
			return Resource{}, fmt.Errorf("%w: credential_ref is required", ErrInvalidArgument)
		}
		patch.CredentialRef = &credentialRef
	}

	if input.Status != nil {
		status, parseErr := parseStatus(*input.Status)
		if parseErr != nil {
			return Resource{}, parseErr
		}
		patch.Status = &status
	}

	updated, err := s.repo.Update(ctx, id, patch)
	if err != nil {
		switch {
		case errors.Is(err, ErrConflict):
			return Resource{}, ErrConflict
		case errors.Is(err, ErrNotFound):
			return Resource{}, ErrNotFound
		case errors.Is(err, ErrInvalidArgument):
			return Resource{}, err
		default:
			return Resource{}, fmt.Errorf("update platform resource: %w", err)
		}
	}

	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.validateReady(); err != nil {
		return err
	}
	if id == uuid.Nil {
		return fmt.Errorf("%w: id is required", ErrInvalidArgument)
	}

	err := s.repo.Delete(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("delete platform resource: %w", err)
	}

	return nil
}

func (s *Service) validateReady() error {
	if s.repo == nil {
		return errors.New("platform resource service dependencies are not initialized")
	}
	return nil
}

func parseCategory(value string) (contracts.ResourceCategory, error) {
	category := contracts.ResourceCategory(strings.ToLower(strings.TrimSpace(value)))
	switch category {
	case contracts.ResourceLLM, contracts.ResourceASR, contracts.ResourceTTS:
		return category, nil
	default:
		return "", fmt.Errorf("%w: unsupported category %q", ErrInvalidArgument, value)
	}
}

func parseCreateStatus(value string) (contracts.ResourceStatus, error) {
	if strings.TrimSpace(value) == "" {
		return contracts.ResourceStatusActive, nil
	}
	return parseStatus(value)
}

func parseStatus(value string) (contracts.ResourceStatus, error) {
	status := contracts.ResourceStatus(strings.ToLower(strings.TrimSpace(value)))
	switch status {
	case contracts.ResourceStatusActive, contracts.ResourceStatusInactive:
		return status, nil
	default:
		return "", fmt.Errorf("%w: unsupported status %q", ErrInvalidArgument, value)
	}
}

func normalizeProvider(value string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(value))
	if provider == "" {
		return "", fmt.Errorf("%w: provider is required", ErrInvalidArgument)
	}
	if !providerPattern.MatchString(provider) {
		return "", fmt.Errorf("%w: provider %q has invalid format", ErrInvalidArgument, value)
	}
	return provider, nil
}

func normalizeResourceKey(value string) (string, error) {
	resourceKey := strings.ToLower(strings.TrimSpace(value))
	if resourceKey == "" {
		return "", fmt.Errorf("%w: resource_key is required", ErrInvalidArgument)
	}
	if !resourceKeyPattern.MatchString(resourceKey) {
		return "", fmt.Errorf("%w: resource_key %q has invalid format", ErrInvalidArgument, value)
	}
	return resourceKey, nil
}

func normalizeJSONObject(raw json.RawMessage, fieldName string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%w: %s is required", ErrInvalidArgument, fieldName)
	}

	var payload any
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, fmt.Errorf("%w: %s must be valid json", ErrInvalidArgument, fieldName)
	}
	if _, ok := payload.(map[string]any); !ok {
		return nil, fmt.Errorf("%w: %s must be json object", ErrInvalidArgument, fieldName)
	}

	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", fieldName, err)
	}
	return json.RawMessage(normalized), nil
}

func validateProviderForCategory(category contracts.ResourceCategory, provider string) error {
	allowed, ok := supportedProvidersByCategory[category]
	if !ok {
		return fmt.Errorf("%w: unsupported category %q", ErrInvalidArgument, category)
	}
	if _, exists := allowed[provider]; !exists {
		return fmt.Errorf("%w: provider %q is not supported for category %q", ErrInvalidArgument, provider, category)
	}
	return nil
}

func allSupportedProviders() map[string]struct{} {
	providers := make(map[string]struct{})
	for _, set := range supportedProvidersByCategory {
		for provider := range set {
			providers[provider] = struct{}{}
		}
	}
	return providers
}
