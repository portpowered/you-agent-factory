// Package response_stream defines the owner-private Factory Session response
// stream capability. The outer Factory Sessions service is the only consumer
// of this contract.
package response_stream

import (
	"context"
	"errors"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responsestream"
)

var (
	ErrInvalidCursor = errors.New("invalid factory response-event cursor")
	ErrInvalidFilter = errors.New("invalid factory response-event filter")
)

// SubscriptionRequest selects a retained-then-live response-event cursor.
type SubscriptionRequest struct {
	AfterSequence int64
	DispatchID    string
	Kinds         []responseevents.Kind
}

// Cursor is the bounded response-event read value returned to the outer
// Factory Sessions service. Its operations keep the private implementation
// and concrete store subscription out of the outer service.
type Cursor struct {
	NextEvents   func(context.Context) ([]responseevents.FactoryResponseEvent, error)
	DrainEvents  func() ([]responseevents.FactoryResponseEvent, error)
	DetachCursor func()
}

func (c *Cursor) Next(ctx context.Context) ([]responseevents.FactoryResponseEvent, error) {
	return c.NextEvents(ctx)
}

func (c *Cursor) Drain() ([]responseevents.FactoryResponseEvent, error) {
	return c.DrainEvents()
}

func (c *Cursor) Detach() { c.DetachCursor() }

// Service owns response-event validation, retention, publication lifecycle,
// subscriptions, reconnect cursor semantics, and internal stream allocation.
type Service interface {
	NewEventStore(string, factoryruntime.Clock) (*responseeventstore.SessionResponseEventStore, error)
	NewStreamRegistry(factoryruntime.Clock) (*responsestream.Registry, error)
	Subscribe(context.Context, *responseeventstore.SessionResponseEventStore, SubscriptionRequest) (*Cursor, error)
	Complete(*responseeventstore.SessionResponseEventStore)
	Close(*responseeventstore.SessionResponseEventStore)
}
