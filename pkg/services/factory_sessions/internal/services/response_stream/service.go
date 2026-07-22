// Package response_stream defines the owner-private Factory Session response
// stream capability. The outer Factory Sessions service is the only consumer
// of this contract.
package response_stream

import (
	"context"
	"errors"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
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

// Tracker coordinates one consumer's persisted acknowledged cursor without
// exposing the concrete tracker implementation.
type Tracker struct {
	RestoreCursor func(context.Context) (cursors.Checkpoint, bool, error)
	AdvanceCursor func(context.Context, cursors.Checkpoint) error
	CurrentCursor func() (cursors.Checkpoint, bool)
}

func (t *Tracker) Restore(ctx context.Context) (cursors.Checkpoint, bool, error) {
	return t.RestoreCursor(ctx)
}

func (t *Tracker) Advance(ctx context.Context, checkpoint cursors.Checkpoint) error {
	return t.AdvanceCursor(ctx, checkpoint)
}

func (t *Tracker) Current() (cursors.Checkpoint, bool) { return t.CurrentCursor() }

// Publisher owns one internal stream's publication and diagnostics callback.
type Publisher struct {
	PublishEvent     func(responsestream.Event) responsestream.Event
	ReportCompaction func(responsestream.CompactionSummary)
	ReadDiagnostics  func() responsestream.PublicationDiagnostics
}

func (p *Publisher) Publish(event responsestream.Event) responsestream.Event {
	return p.PublishEvent(event)
}

func (p *Publisher) Diagnostics() responsestream.PublicationDiagnostics {
	return p.ReadDiagnostics()
}

// Service owns response-event validation, retention, publication lifecycle,
// subscriptions, reconnect cursor semantics, and internal stream allocation.
type Service interface {
	NewEventStore(string, factoryruntime.Clock) (*responseeventstore.SessionResponseEventStore, error)
	NewStreamRegistry(factoryruntime.Clock) (*responsestream.Registry, error)
	Subscribe(context.Context, *responseeventstore.SessionResponseEventStore, SubscriptionRequest) (*Cursor, error)
	NewCursorTracker(cursors.Store, cursors.StorageIdentity) (*Tracker, error)
	NewPublisher(*responsestream.SessionResponseStream, responsestream.DiagnosticsObserver) *Publisher
	Publish(*responseeventstore.SessionResponseEventStore, responseevents.FactoryResponseEvent) (responseevents.FactoryResponseEvent, error)
	Complete(*responseeventstore.SessionResponseEventStore)
	Close(*responseeventstore.SessionResponseEventStore)
}
