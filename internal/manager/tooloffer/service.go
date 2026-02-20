package tooloffer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
)

type Service struct {
	repo       Repository
	itemReader ToolItemReader
}

func NewService(repo Repository, itemReader ToolItemReader) *Service {
	return &Service{repo: repo, itemReader: itemReader}
}

func (s *Service) Create(ctx context.Context, toolItemID uuid.UUID, input CreateInput) (Offer, error) {
	if err := s.validateReady(); err != nil {
		return Offer{}, err
	}
	if toolItemID == uuid.Nil {
		return Offer{}, fmt.Errorf("%w: tool_item_id is required", ErrInvalidArgument)
	}
	if _, err := s.itemReader.GetByID(ctx, toolItemID); err != nil {
		if errors.Is(err, toolmarket.ErrNotFound) {
			return Offer{}, fmt.Errorf("%w: tool market item not found", ErrNotFound)
		}
		return Offer{}, fmt.Errorf("load tool market item: %w", err)
	}

	offerType, err := parseOfferType(input.OfferType)
	if err != nil {
		return Offer{}, err
	}
	price, currency, err := normalizePriceAndCurrency(input.Price, input.Currency)
	if err != nil {
		return Offer{}, err
	}
	if (offerType == contracts.OfferTypeActivationCode || offerType == contracts.OfferTypeAdminGrant) && price != nil {
		return Offer{}, fmt.Errorf("%w: %s does not support price", ErrInvalidArgument, offerType)
	}

	quotaTotal, err := normalizePositiveInt64(input.QuotaTotal, "quota_total")
	if err != nil {
		return Offer{}, err
	}
	durationSeconds, err := normalizePositiveInt64(input.DurationSeconds, "duration_seconds")
	if err != nil {
		return Offer{}, err
	}
	status, err := parseCreateStatus(input.Status)
	if err != nil {
		return Offer{}, err
	}

	created, err := s.repo.Create(ctx, Offer{
		ID:              uuid.New(),
		ToolItemID:      toolItemID,
		OfferType:       offerType,
		Price:           price,
		Currency:        currency,
		QuotaTotal:      quotaTotal,
		DurationSeconds: durationSeconds,
		Status:          status,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrConflict):
			return Offer{}, ErrConflict
		case errors.Is(err, ErrInvalidArgument):
			return Offer{}, err
		default:
			return Offer{}, fmt.Errorf("create tool offer: %w", err)
		}
	}
	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Offer, error) {
	if err := s.validateReady(); err != nil {
		return Offer{}, err
	}
	if id == uuid.Nil {
		return Offer{}, fmt.Errorf("%w: id is required", ErrInvalidArgument)
	}

	offer, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Offer{}, ErrNotFound
		}
		return Offer{}, fmt.Errorf("query tool offer: %w", err)
	}
	return offer, nil
}

func (s *Service) ListByItem(ctx context.Context, toolItemID uuid.UUID, input ListInput) ([]Offer, error) {
	if err := s.validateReady(); err != nil {
		return nil, err
	}
	if toolItemID == uuid.Nil {
		return nil, fmt.Errorf("%w: tool_item_id is required", ErrInvalidArgument)
	}
	if _, err := s.itemReader.GetByID(ctx, toolItemID); err != nil {
		if errors.Is(err, toolmarket.ErrNotFound) {
			return nil, fmt.Errorf("%w: tool market item not found", ErrNotFound)
		}
		return nil, fmt.Errorf("load tool market item: %w", err)
	}

	filter := ListFilter{}
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

	offers, err := s.repo.ListByItem(ctx, toolItemID, filter)
	if err != nil {
		return nil, fmt.Errorf("list tool offers: %w", err)
	}
	return offers, nil
}

func (s *Service) validateReady() error {
	if s.repo == nil || s.itemReader == nil {
		return errors.New("tool offer service dependencies are not initialized")
	}
	return nil
}

func parseOfferType(value string) (contracts.ToolOfferType, error) {
	offerType := contracts.ToolOfferType(strings.ToLower(strings.TrimSpace(value)))
	switch offerType {
	case contracts.OfferTypeFree,
		contracts.OfferTypeTrial,
		contracts.OfferTypePaid,
		contracts.OfferTypeActivationCode,
		contracts.OfferTypeAdminGrant,
		contracts.OfferTypeUsagePack,
		contracts.OfferTypeTimeLimited:
		return offerType, nil
	default:
		return "", fmt.Errorf("%w: unsupported offer_type %q", ErrInvalidArgument, value)
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

func normalizePriceAndCurrency(price *float64, currency *string) (*float64, *string, error) {
	if price == nil && currency == nil {
		return nil, nil, nil
	}
	if price == nil || currency == nil {
		return nil, nil, fmt.Errorf("%w: price and currency must be set together", ErrInvalidArgument)
	}
	if *price < 0 {
		return nil, nil, fmt.Errorf("%w: price must be >= 0", ErrInvalidArgument)
	}
	trimmedCurrency := strings.ToUpper(strings.TrimSpace(*currency))
	if trimmedCurrency == "" {
		return nil, nil, fmt.Errorf("%w: currency is required", ErrInvalidArgument)
	}
	normalizedPrice := *price
	return &normalizedPrice, &trimmedCurrency, nil
}

func normalizePositiveInt64(value *int64, field string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	if *value <= 0 {
		return nil, fmt.Errorf("%w: %s must be > 0", ErrInvalidArgument, field)
	}
	normalized := *value
	return &normalized, nil
}
