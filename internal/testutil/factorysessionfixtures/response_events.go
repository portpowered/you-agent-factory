package factorysessionfixtures

import (
	"context"
	"sync"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// FactoryResponseEventSubscription is a programmable HTTP-facing projection
// of a Factory Sessions response-event cursor.
type FactoryResponseEventSubscription struct {
	Batches  chan []apisurface.FactoryResponseEventRecord
	Detached chan struct{}
	once     sync.Once
}

func NewFactoryResponseEventSubscription(
	buffer int,
) *FactoryResponseEventSubscription {
	return &FactoryResponseEventSubscription{
		Batches:  make(chan []apisurface.FactoryResponseEventRecord, buffer),
		Detached: make(chan struct{}),
	}
}

func (s *FactoryResponseEventSubscription) Next(
	ctx context.Context,
) ([]apisurface.FactoryResponseEventRecord, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case batch, ok := <-s.Batches:
		if !ok {
			return nil, factorysessions.ErrResponseEventSubscriptionClosed
		}
		return batch, nil
	}
}

func (s *FactoryResponseEventSubscription) Detach() {
	if s == nil {
		return
	}
	s.once.Do(func() { close(s.Detached) })
}
