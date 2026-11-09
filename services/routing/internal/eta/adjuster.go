package eta

import (
	"context"

	"github.com/city-connector-backend/routing/internal/raptor"
)

// Adjuster defines the interface for adjusting scheduled itineraries with live delay data.
type Adjuster interface {
	Adjust(ctx context.Context, journeys []raptor.Journey) []raptor.Journey
}

// NoopAdjuster passes through static schedule journeys without modification (Phase 1 MVP).
type NoopAdjuster struct{}

func NewNoopAdjuster() *NoopAdjuster {
	return &NoopAdjuster{}
}

func (a *NoopAdjuster) Adjust(ctx context.Context, journeys []raptor.Journey) []raptor.Journey {
	return journeys
}
