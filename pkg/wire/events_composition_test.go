package wire

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	events "github.com/portpowered/infinite-you/pkg/services/events"
)

func eventsWireTestAppendRequest() events.AppendRequest {
	return events.AppendRequest{
		Topic:          "chat-session/wire-composition/events",
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"ok":true}`),
	}
}

// TestProvideEventsServiceConstructsAnIndependentServiceDirectly proves the
// exact provider function registered in this graph's servicesSet returns a
// functional, independently isolated events.Service, and that InjectBundle
// itself still succeeds with that provider registered.
func TestProvideEventsServiceConstructsAnIndependentServiceDirectly(t *testing.T) {
	t.Parallel()

	if _, err := InjectBundle(context.Background(), serviceedges.Edges{}); err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}

	zapLogger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("logging.NewDefaultLogger() error = %v", err)
	}
	logger := logging.NewZapLogger(zapLogger, false)

	first, err := provideEventsService(logger)
	if err != nil {
		t.Fatalf("provideEventsService() error = %v", err)
	}
	if first == nil {
		t.Fatal("provideEventsService() = nil, want a constructed events.Service")
	}

	ctx := context.Background()
	if _, err := first.Append(ctx, eventsWireTestAppendRequest()); err != nil {
		t.Fatalf("Append() on provider-constructed service error = %v", err)
	}

	second, err := provideEventsService(logger)
	if err != nil {
		t.Fatalf("provideEventsService() second call error = %v", err)
	}
	result, err := second.Read(ctx, events.ReadRequest{
		Topic: "chat-session/wire-composition/events",
		From:  events.Cursor{Topic: "chat-session/wire-composition/events"},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Read() on second provider call error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeAtHead {
		t.Fatalf("Read() on second provider call Outcome = %v, want ReadOutcomeAtHead (a second provider call must not observe the first call's independent store)", result.Outcome)
	}
}

// TestProvideApplicationProcessLifecycleSharesTheExactEventsInstance proves
// the exact sequence InjectBundle's generated body performs (construct one
// events.Service, thread that same value into
// provideApplicationProcessLifecycle) results in one shared root: closing the
// composed ProcessLifecycle deterministically shuts down the very instance
// application code would observably Append/Read through, not an independent
// copy. This is the canonical construction proof for shared identity: it
// exercises the registered provider chain directly (matching wire_gen.go's
// generated call order) rather than a source-topology assertion.
func TestProvideApplicationProcessLifecycleSharesTheExactEventsInstance(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{}
	providersService, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}

	zapLogger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("logging.NewDefaultLogger() error = %v", err)
	}
	logger := logging.NewZapLogger(zapLogger, false)

	eventsService, err := provideEventsService(logger)
	if err != nil {
		t.Fatalf("provideEventsService() error = %v", err)
	}

	lifecycle, err := provideApplicationProcessLifecycle(providersService, &closingModelsService{}, eventsService, nil, &localWorkerSessionsBoundary{}, nil)
	if err != nil {
		t.Fatalf("provideApplicationProcessLifecycle() error = %v", err)
	}

	ctx := context.Background()
	if _, err := eventsService.Append(ctx, eventsWireTestAppendRequest()); err != nil {
		t.Fatalf("Append() before shutdown error = %v", err)
	}

	if err := lifecycle.Close(ctx); err != nil {
		t.Fatalf("ProcessLifecycle.Close() error = %v", err)
	}
	if err := lifecycle.Close(ctx); err != nil {
		t.Fatalf("second ProcessLifecycle.Close() error = %v, want idempotent no-op", err)
	}

	if _, err := eventsService.Append(ctx, eventsWireTestAppendRequest()); !errors.Is(err, events.ErrOperationFailed) {
		t.Fatalf("Append() on the same events.Service after ProcessLifecycle.Close() error = %v, want it classified as events.ErrOperationFailed (proves the closed instance and the append/read-observable instance are the same shared root, not an independent copy)", err)
	}
}

func TestProvideApplicationProcessLifecycleRequiresEventsLifecycle(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{}
	providersService, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}

	if _, err := provideApplicationProcessLifecycle(providersService, &closingModelsService{}, staticEventsServiceWithoutLifecycle{}, nil, &localWorkerSessionsBoundary{}, nil); err == nil {
		t.Fatal("provideApplicationProcessLifecycle() error = nil, want a diagnostic when the Events service does not expose a Close(context.Context) error shutdown method")
	}
}

// staticEventsServiceWithoutLifecycle is a minimal events.Service double that
// deliberately does not implement Close(context.Context) error, proving
// provideApplicationProcessLifecycle rejects an Events service that cannot be
// shut down instead of silently skipping its Close.
type staticEventsServiceWithoutLifecycle struct {
	events.Service
}
