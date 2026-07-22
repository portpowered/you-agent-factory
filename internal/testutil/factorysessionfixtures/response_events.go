package factorysessionfixtures

import (
	"context"
	"sync"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// ResponseEventCursor is a programmable Factory Sessions root cursor.
type ResponseEventCursor struct {
	Batches  chan []factorysessions.FactoryResponseEvent
	Detached chan struct{}
	once     sync.Once
}

func NewResponseEventCursor(buffer int) *ResponseEventCursor {
	return &ResponseEventCursor{
		Batches:  make(chan []factorysessions.FactoryResponseEvent, buffer),
		Detached: make(chan struct{}),
	}
}

func (c *ResponseEventCursor) Next(
	ctx context.Context,
) ([]factorysessions.FactoryResponseEvent, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case batch, ok := <-c.Batches:
		if !ok {
			return nil, factorysessions.ErrResponseEventSubscriptionClosed
		}
		return batch, nil
	}
}

func (c *ResponseEventCursor) Drain() ([]factorysessions.FactoryResponseEvent, error) {
	var events []factorysessions.FactoryResponseEvent
	for {
		select {
		case batch, ok := <-c.Batches:
			if !ok {
				return events, nil
			}
			events = append(events, batch...)
		default:
			return events, nil
		}
	}
}

func (c *ResponseEventCursor) Detach() {
	if c == nil {
		return
	}
	c.once.Do(func() { close(c.Detached) })
}

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
