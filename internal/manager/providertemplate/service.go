package providertemplate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

var (
	providerPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	fieldPathPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, createdBy uuid.UUID, input CreateInput) (Template, error) {
	if err := s.validateReady(); err != nil {
		return Template{}, err
	}
	if createdBy == uuid.Nil {
		return Template{}, fmt.Errorf("%w: created_by is required", ErrInvalidArgument)
	}

	category, err := parseCategory(input.Category)
	if err != nil {
		return Template{}, err
	}
	provider, err := normalizeProvider(input.Provider)
	if err != nil {
		return Template{}, err
	}
	status, err := parseCreateStatus(input.Status)
	if err != nil {
		return Template{}, err
	}
	if input.Version <= 0 {
		return Template{}, fmt.Errorf("%w: version must be > 0", ErrInvalidArgument)
	}

	fields, err := validateAndNormalizeFields(input.Fields)
	if err != nil {
		return Template{}, err
	}
	fieldsRaw, err := json.Marshal(fields)
	if err != nil {
		return Template{}, fmt.Errorf("marshal fields: %w", err)
	}

	template := Template{
		ID:        uuid.New(),
		Category:  category,
		Provider:  provider,
		Status:    status,
		Version:   input.Version,
		Fields:    json.RawMessage(fieldsRaw),
		CreatedBy: createdBy,
	}

	created, err := s.repo.Create(ctx, template)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Template{}, ErrConflict
		}
		if errors.Is(err, ErrInvalidArgument) {
			return Template{}, err
		}
		return Template{}, fmt.Errorf("create provider template: %w", err)
	}

	return created, nil
}

func (s *Service) List(ctx context.Context, input ListInput) ([]Template, error) {
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

	if strings.TrimSpace(input.Provider) != "" {
		provider, err := normalizeProvider(input.Provider)
		if err != nil {
			return nil, err
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

	items, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list provider templates: %w", err)
	}
	return items, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (Template, error) {
	if err := s.validateReady(); err != nil {
		return Template{}, err
	}
	if id == uuid.Nil {
		return Template{}, fmt.Errorf("%w: id is required", ErrInvalidArgument)
	}
	if !input.HasChanges() {
		return Template{}, fmt.Errorf("%w: at least one field is required", ErrInvalidArgument)
	}

	patch := UpdatePatch{}
	if input.Category != nil {
		category, err := parseCategory(*input.Category)
		if err != nil {
			return Template{}, err
		}
		patch.Category = &category
	}
	if input.Provider != nil {
		provider, err := normalizeProvider(*input.Provider)
		if err != nil {
			return Template{}, err
		}
		patch.Provider = &provider
	}
	if input.Status != nil {
		status, err := parseStatus(*input.Status)
		if err != nil {
			return Template{}, err
		}
		patch.Status = &status
	}
	if input.Version != nil {
		if *input.Version <= 0 {
			return Template{}, fmt.Errorf("%w: version must be > 0", ErrInvalidArgument)
		}
		version := *input.Version
		patch.Version = &version
	}
	if input.Fields != nil {
		fields, err := validateAndNormalizeFields(*input.Fields)
		if err != nil {
			return Template{}, err
		}
		raw, err := json.Marshal(fields)
		if err != nil {
			return Template{}, fmt.Errorf("marshal fields: %w", err)
		}
		jsonRaw := json.RawMessage(raw)
		patch.Fields = &jsonRaw
	}

	updated, err := s.repo.Update(ctx, id, patch)
	if err != nil {
		switch {
		case errors.Is(err, ErrConflict):
			return Template{}, ErrConflict
		case errors.Is(err, ErrNotFound):
			return Template{}, ErrNotFound
		case errors.Is(err, ErrInvalidArgument):
			return Template{}, err
		default:
			return Template{}, fmt.Errorf("update provider template: %w", err)
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
		return fmt.Errorf("delete provider template: %w", err)
	}
	return nil
}

func (s *Service) validateReady() error {
	if s.repo == nil {
		return errors.New("provider template service dependencies are not initialized")
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

func validateAndNormalizeFields(fields []Field) ([]Field, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("%w: fields is required", ErrInvalidArgument)
	}

	normalized := make([]Field, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))

	for idx, field := range fields {
		key := strings.ToLower(strings.TrimSpace(field.Key))
		if !fieldPathPattern.MatchString(key) {
			return nil, fmt.Errorf("%w: fields[%d].key has invalid format", ErrInvalidArgument, idx)
		}
		if containsReservedFieldPathSegment(key) {
			return nil, fmt.Errorf("%w: fields[%d].key %q uses reserved segment", ErrInvalidArgument, idx, key)
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: duplicate fields key %q", ErrInvalidArgument, key)
		}
		for existing := range seen {
			if strings.HasPrefix(existing, key+".") || strings.HasPrefix(key, existing+".") {
				return nil, fmt.Errorf("%w: fields key conflict between %q and %q", ErrInvalidArgument, key, existing)
			}
		}

		label := strings.TrimSpace(field.Label)
		if label == "" {
			return nil, fmt.Errorf("%w: fields[%d].label is required", ErrInvalidArgument, idx)
		}

		fieldType := strings.ToLower(strings.TrimSpace(field.Type))
		switch fieldType {
		case "text", "number", "integer", "select":
		default:
			return nil, fmt.Errorf("%w: fields[%d].type %q is not supported", ErrInvalidArgument, idx, field.Type)
		}

		if fieldType == "select" {
			if len(field.Options) == 0 {
				return nil, fmt.Errorf("%w: fields[%d].options is required for select", ErrInvalidArgument, idx)
			}
			for optionIdx, option := range field.Options {
				if strings.TrimSpace(option.Label) == "" || strings.TrimSpace(option.Value) == "" {
					return nil, fmt.Errorf("%w: fields[%d].options[%d] must have label and value", ErrInvalidArgument, idx, optionIdx)
				}
			}
		}

		if field.Min != nil && field.Max != nil && *field.Min > *field.Max {
			return nil, fmt.Errorf("%w: fields[%d].min must be <= max", ErrInvalidArgument, idx)
		}
		if field.Step != nil && *field.Step <= 0 {
			return nil, fmt.Errorf("%w: fields[%d].step must be > 0", ErrInvalidArgument, idx)
		}
		if fieldType == "integer" {
			if field.Min != nil && !isWholeNumber(*field.Min) {
				return nil, fmt.Errorf("%w: fields[%d].min must be integer", ErrInvalidArgument, idx)
			}
			if field.Max != nil && !isWholeNumber(*field.Max) {
				return nil, fmt.Errorf("%w: fields[%d].max must be integer", ErrInvalidArgument, idx)
			}
			if field.Step != nil && !isWholeNumber(*field.Step) {
				return nil, fmt.Errorf("%w: fields[%d].step must be integer", ErrInvalidArgument, idx)
			}
		}

		normalized = append(normalized, Field{
			Key:          key,
			Label:        label,
			Type:         fieldType,
			Required:     field.Required,
			DefaultValue: field.DefaultValue,
			HelperText:   strings.TrimSpace(field.HelperText),
			Placeholder:  strings.TrimSpace(field.Placeholder),
			Min:          field.Min,
			Max:          field.Max,
			Step:         field.Step,
			Options:      field.Options,
		})
		seen[key] = struct{}{}
	}

	return normalized, nil
}

func containsReservedFieldPathSegment(path string) bool {
	for _, segment := range strings.Split(path, ".") {
		switch segment {
		case "base_url", "access_key":
			return true
		}
	}
	return false
}

func isWholeNumber(value float64) bool {
	return math.Mod(value, 1) == 0
}
