package tooloffer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
)

type fakeOfferRepository struct {
	offers map[uuid.UUID]Offer
}

func newFakeOfferRepository() *fakeOfferRepository {
	return &fakeOfferRepository{offers: make(map[uuid.UUID]Offer)}
}

func (r *fakeOfferRepository) Create(_ context.Context, offer Offer) (Offer, error) {
	now := time.Now().UTC()
	offer.CreatedAt = now
	offer.UpdatedAt = now
	r.offers[offer.ID] = offer
	return offer, nil
}

func (r *fakeOfferRepository) GetByID(_ context.Context, id uuid.UUID) (Offer, error) {
	offer, exists := r.offers[id]
	if !exists {
		return Offer{}, ErrNotFound
	}
	return offer, nil
}

func (r *fakeOfferRepository) ListByItem(_ context.Context, toolItemID uuid.UUID, filter ListFilter) ([]Offer, error) {
	offers := make([]Offer, 0)
	for _, offer := range r.offers {
		if offer.ToolItemID != toolItemID {
			continue
		}
		if filter.Status != nil && offer.Status != *filter.Status {
			continue
		}
		offers = append(offers, offer)
	}
	return offers, nil
}

type fakeToolItemReader struct {
	items map[uuid.UUID]toolmarket.Item
}

func (r *fakeToolItemReader) GetByID(_ context.Context, id uuid.UUID) (toolmarket.Item, error) {
	item, exists := r.items[id]
	if !exists {
		return toolmarket.Item{}, toolmarket.ErrNotFound
	}
	return item, nil
}

func TestService_CreateAndListByItem(t *testing.T) {
	itemID := uuid.New()
	service := NewService(newFakeOfferRepository(), &fakeToolItemReader{items: map[uuid.UUID]toolmarket.Item{
		itemID: {
			ID:      itemID,
			Status:  contracts.ToolStatusActive,
			ToolKey: "mcp-device-helper",
		},
	}})

	quota := int64(1000)
	duration := int64(86400)
	offer, err := service.Create(context.Background(), itemID, CreateInput{
		OfferType:       "activation_code",
		QuotaTotal:      &quota,
		DurationSeconds: &duration,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if offer.OfferType != contracts.OfferTypeActivationCode {
		t.Fatalf("expected offer type activation_code, got %q", offer.OfferType)
	}

	items, err := service.ListByItem(context.Background(), itemID, ListInput{OnlyActive: true})
	if err != nil {
		t.Fatalf("ListByItem() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 offer, got %d", len(items))
	}
}

func TestService_CreateRejectsInvalidPriceCurrency(t *testing.T) {
	itemID := uuid.New()
	service := NewService(newFakeOfferRepository(), &fakeToolItemReader{items: map[uuid.UUID]toolmarket.Item{
		itemID: {ID: itemID, Status: contracts.ToolStatusActive},
	}})

	price := 19.9
	_, err := service.Create(context.Background(), itemID, CreateInput{
		OfferType: "paid",
		Price:     &price,
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestService_CreateReturnsNotFoundWhenItemMissing(t *testing.T) {
	service := NewService(newFakeOfferRepository(), &fakeToolItemReader{items: map[uuid.UUID]toolmarket.Item{}})

	_, err := service.Create(context.Background(), uuid.New(), CreateInput{OfferType: "activation_code"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
