package wire

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	events "github.com/portpowered/infinite-you/pkg/services/events"
)

func validAppendRequest() events.AppendRequest {
	return events.AppendRequest{
		Topic:          "chat-session/abc/events",
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"tool":"grep","status":"ok"}`),
	}
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	service, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}

	ctx := context.Background()
	if _, err := service.Read(ctx, events.ReadRequest{
		Topic: "chat-session/inert/events",
		From:  events.Cursor{Topic: "chat-session/inert/events"},
		Limit: 10,
	}); err != nil {
		t.Fatalf("Read() on a freshly constructed root error = %v, want ReadOutcomeAtHead with no error", err)
	}
}

func TestNewServiceReturnsAFunctionalIndependentRoot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	first, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if _, err := first.Append(ctx, validAppendRequest()); err != nil {
		t.Fatalf("Append() on first root error = %v", err)
	}

	second, err := NewService()
	if err != nil {
		t.Fatalf("NewService() second call error = %v", err)
	}
	result, err := second.Read(ctx, events.ReadRequest{
		Topic: "chat-session/abc/events",
		From:  events.Cursor{Topic: "chat-session/abc/events"},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Read() on second root error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeAtHead {
		t.Fatalf("Read() on second root Outcome = %v, want ReadOutcomeAtHead (each NewService call must construct an independent store)", result.Outcome)
	}
}

// eventsWireTestCloser mirrors the exact unexported structural interface
// pkg/wire asserts against the constructed root (see
// pkg/wire/events_providers.go's eventsLifecycle): pkg/services/events
// publishes exactly one interface (Service), so shutdown capability is
// proven structurally here rather than through a second published contract.
type eventsWireTestCloser interface {
	Close(context.Context) error
}

func TestNewServiceSatisfiesCloseWithoutWideningService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	lifecycle, ok := service.(eventsWireTestCloser)
	if !ok {
		t.Fatal("NewService() implementation does not expose a Close(context.Context) error shutdown method")
	}
	if err := lifecycle.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := lifecycle.Close(ctx); err != nil {
		t.Fatalf("second Close() error = %v, want idempotent no-op", err)
	}

	if _, err := service.Append(ctx, validAppendRequest()); !errors.Is(err, events.ErrClosed) {
		t.Fatalf("Append() after Close() error = %v, want ErrClosed", err)
	}
}
