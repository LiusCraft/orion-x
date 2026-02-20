package toolmarket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolvalidator"
)

var (
	toolKeyPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-_]{2,127}$`)
	providerPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

type Service struct {
	repo      Repository
	validator ConfigValidator
}

func NewService(repo Repository, validator ConfigValidator) *Service {
	return &Service{repo: repo, validator: validator}
}

func (s *Service) Create(ctx context.Context, createdBy uuid.UUID, input CreateInput) (Item, error) {
	if err := s.validateReady(); err != nil {
		return Item{}, err
	}
	if createdBy == uuid.Nil {
		return Item{}, fmt.Errorf("%w: created_by is required", ErrInvalidArgument)
	}

	toolKey, err := normalizeToolKey(input.ToolKey)
	if err != nil {
		return Item{}, err
	}
	provider, err := normalizeProvider(input.Provider)
	if err != nil {
		return Item{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Item{}, fmt.Errorf("%w: name is required", ErrInvalidArgument)
	}

	protocol, err := parseProtocol(input.Protocol, true)
	if err != nil {
		return Item{}, err
	}
	config, err := s.validateConfig(ctx, protocol, input.Config)
	if err != nil {
		return Item{}, err
	}
	status, err := parseCreateStatus(input.Status)
	if err != nil {
		return Item{}, err
	}

	created, err := s.repo.Create(ctx, Item{
		ID:        uuid.New(),
		ToolKey:   toolKey,
		Name:      name,
		Provider:  provider,
		Protocol:  protocol,
		Config:    config,
		Status:    status,
		CreatedBy: createdBy,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrConflict):
			return Item{}, ErrConflict
		case errors.Is(err, ErrInvalidArgument):
			return Item{}, err
		default:
			return Item{}, fmt.Errorf("create tool market item: %w", err)
		}
	}

	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Item, error) {
	if err := s.validateReady(); err != nil {
		return Item{}, err
	}
	if id == uuid.Nil {
		return Item{}, fmt.Errorf("%w: id is required", ErrInvalidArgument)
	}

	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Item{}, ErrNotFound
		}
		return Item{}, fmt.Errorf("query tool market item: %w", err)
	}
	return item, nil
}

func (s *Service) List(ctx context.Context, input ListInput) ([]Item, error) {
	if err := s.validateReady(); err != nil {
		return nil, err
	}

	filter := ListFilter{}
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	if provider != "" {
		if !providerPattern.MatchString(provider) {
			return nil, fmt.Errorf("%w: provider %q has invalid format", ErrInvalidArgument, input.Provider)
		}
		filter.Provider = provider
	}

	if strings.TrimSpace(input.Status) != "" {
		status, err := parseStatus(input.Status)
		if err != nil {
			return nil, err
		}
		filter.Status = &status
	} else if input.OnlyActive {
		status := contracts.ToolStatusActive
		filter.Status = &status
	}

	items, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list tool market items: %w", err)
	}
	return items, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (Item, error) {
	if err := s.validateReady(); err != nil {
		return Item{}, err
	}
	if id == uuid.Nil {
		return Item{}, fmt.Errorf("%w: id is required", ErrInvalidArgument)
	}
	if !input.HasChanges() {
		return Item{}, fmt.Errorf("%w: at least one field is required", ErrInvalidArgument)
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Item{}, ErrNotFound
		}
		return Item{}, fmt.Errorf("load tool market item: %w", err)
	}

	patch := UpdatePatch{}
	effectiveProtocol := existing.Protocol
	effectiveConfig := existing.Config

	if input.ToolKey != nil {
		toolKey, parseErr := normalizeToolKey(*input.ToolKey)
		if parseErr != nil {
			return Item{}, parseErr
		}
		patch.ToolKey = &toolKey
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return Item{}, fmt.Errorf("%w: name is required", ErrInvalidArgument)
		}
		patch.Name = &name
	}
	if input.Provider != nil {
		provider, parseErr := normalizeProvider(*input.Provider)
		if parseErr != nil {
			return Item{}, parseErr
		}
		patch.Provider = &provider
	}
	if input.Protocol != nil {
		protocol, parseErr := parseProtocol(*input.Protocol, false)
		if parseErr != nil {
			return Item{}, parseErr
		}
		effectiveProtocol = protocol
		patch.Protocol = &protocol
	}
	if input.Config != nil {
		effectiveConfig = *input.Config
	}
	if input.Protocol != nil || input.Config != nil {
		config, validateErr := s.validateConfig(ctx, effectiveProtocol, effectiveConfig)
		if validateErr != nil {
			return Item{}, validateErr
		}
		patch.Config = &config
	}
	if input.Status != nil {
		status, parseErr := parseStatus(*input.Status)
		if parseErr != nil {
			return Item{}, parseErr
		}
		patch.Status = &status
	}

	updated, err := s.repo.Update(ctx, id, patch)
	if err != nil {
		switch {
		case errors.Is(err, ErrConflict):
			return Item{}, ErrConflict
		case errors.Is(err, ErrNotFound):
			return Item{}, ErrNotFound
		case errors.Is(err, ErrInvalidArgument):
			return Item{}, err
		default:
			return Item{}, fmt.Errorf("update tool market item: %w", err)
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
		return fmt.Errorf("delete tool market item: %w", err)
	}

	return nil
}

func (s *Service) validateReady() error {
	if s.repo == nil || s.validator == nil {
		return errors.New("tool market service dependencies are not initialized")
	}
	return nil
}

func (s *Service) validateConfig(ctx context.Context, protocol contracts.ToolProtocol, raw json.RawMessage) (json.RawMessage, error) {
	normalized, err := s.validator.Validate(ctx, protocol, raw)
	if err == nil {
		return normalized, nil
	}
	if errors.Is(err, toolvalidator.ErrInvalidArgument) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidArgument, err.Error())
	}
	return nil, fmt.Errorf("validate tool config: %w", err)
}

func normalizeToolKey(value string) (string, error) {
	toolKey := strings.ToLower(strings.TrimSpace(value))
	if toolKey == "" {
		return "", fmt.Errorf("%w: tool_key is required", ErrInvalidArgument)
	}
	if !toolKeyPattern.MatchString(toolKey) {
		return "", fmt.Errorf("%w: tool_key %q has invalid format", ErrInvalidArgument, value)
	}
	return toolKey, nil
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

func parseProtocol(value string, allowDefault bool) (contracts.ToolProtocol, error) {
	protocol := contracts.ToolProtocol(strings.ToLower(strings.TrimSpace(value)))
	if protocol == "" && allowDefault {
		protocol = contracts.ToolProtocolMCP
	}
	switch protocol {
	case contracts.ToolProtocolMCP:
		return protocol, nil
	default:
		return "", fmt.Errorf("%w: unsupported protocol %q", ErrInvalidArgument, value)
	}
}

func parseCreateStatus(value string) (contracts.ToolStatus, error) {
	if strings.TrimSpace(value) == "" {
		return contracts.ToolStatusActive, nil
	}
	return parseStatus(value)
}

func parseStatus(value string) (contracts.ToolStatus, error) {
	status := contracts.ToolStatus(strings.ToLower(strings.TrimSpace(value)))
	switch status {
	case contracts.ToolStatusActive, contracts.ToolStatusInactive:
		return status, nil
	default:
		return "", fmt.Errorf("%w: unsupported status %q", ErrInvalidArgument, value)
	}
}
