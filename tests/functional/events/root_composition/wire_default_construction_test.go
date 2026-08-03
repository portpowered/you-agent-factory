// Package root_composition_test proves the Events wire boundary's public
// default-retention constructor (eventswire.NewService) functions as a real,
// independently-scoped Events root. Production construction (pkg/wire's
// provideEventsService) always calls eventswire.NewServiceWithRetention with
// an explicit override, which tests/functional/events/response_events
// already exercises end-to-end through root.BuildProcess; this cell closes
// the remaining direct-construction proof for the plain NewService entry
// point that no root.BuildProcess-driven functional test reaches.
package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	events "github.com/portpowered/infinite-you/pkg/services/events"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
)

func wireDefaultConstructionAppendRequest(topic events.Topic) events.AppendRequest {
	return events.AppendRequest{
		Topic:          topic,
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"ok":true}`),
	}
}

// TestEventsWireNewServiceConstructsAnIndependentFunctionalRoot proves
// eventswire.NewService (the published default-retention constructor)
// returns a working, independently-scoped Events root: appends on one
// instance are invisible to a second instance, and the returned root accepts
// Append/Read traffic without requiring a retention override.
func TestEventsWireNewServiceConstructsAnIndependentFunctionalRoot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	const topic = events.Topic("chat-session/wire-default-construction/events")

	first, err := eventswire.NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if _, err := first.Append(ctx, wireDefaultConstructionAppendRequest(topic)); err != nil {
		t.Fatalf("Append() on first root error = %v", err)
	}

	second, err := eventswire.NewService()
	if err != nil {
		t.Fatalf("NewService() second call error = %v", err)
	}
	result, err := second.Read(ctx, events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Read() on second root error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeAtHead {
		t.Fatalf("Read() on second root Outcome = %v, want ReadOutcomeAtHead (each NewService call must construct an independent store)", result.Outcome)
	}
}

// TestEventsWireNewServiceSupportsShutdown proves the root returned by
// eventswire.NewService exposes the same Close(context.Context) error
// shutdown role production construction relies on, and that Close is
// idempotent and rejects further Append traffic.
func TestEventsWireNewServiceSupportsShutdown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	const topic = events.Topic("chat-session/wire-default-construction-shutdown/events")

	service, err := eventswire.NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	lifecycle, ok := service.(interface {
		Close(context.Context) error
	})
	if !ok {
		t.Fatal("NewService() implementation does not expose a Close(context.Context) error shutdown method")
	}
	if err := lifecycle.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := lifecycle.Close(ctx); err != nil {
		t.Fatalf("second Close() error = %v, want idempotent no-op", err)
	}

	if _, err := service.Append(ctx, wireDefaultConstructionAppendRequest(topic)); !errors.Is(err, events.ErrClosed) {
		t.Fatalf("Append() after Close() error = %v, want ErrClosed", err)
	}
}
