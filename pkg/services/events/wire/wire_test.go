package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
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

func TestNewServiceLoggerModesPreserveObservations(t *testing.T) {
	t.Parallel()
	core, captured := observer.New(zapcore.DebugLevel)
	logger := logging.NewZapLogger(zap.New(core), true)
	for _, test := range []struct {
		name    string
		loggers []logging.Logger
		logged  bool
	}{
		{"omitted", nil, false},
		{"nil", []logging.Logger{nil}, false},
		{"noop", []logging.Logger{logging.NoopLogger{}}, false},
		{"first nil stays quiet", []logging.Logger{nil, logger}, false},
		{"supplied", []logging.Logger{logger, logging.NoopLogger{}}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			captured.TakeAll()
			service, err := NewService(test.loggers...)
			if err != nil {
				t.Fatal(err)
			}
			if captured.Len() != 0 {
				t.Fatal("construction emitted diagnostics")
			}
			assertOwnerObservations(t, service)
			logs := captured.TakeAll()
			if test.logged && len(logs) == 0 {
				t.Fatal("supplied logger received no operations")
			}
			if !test.logged && len(logs) != 0 {
				t.Fatalf("quiet mode emitted %d records", len(logs))
			}
			if test.logged {
				assertOwnerDiagnostics(t, logs)
			}
		})
	}
}

func assertOwnerDiagnostics(t *testing.T, logs []observer.LoggedEntry) {
	t.Helper()
	var accepted, duplicate bool
	for _, entry := range logs {
		if strings.Contains(fmt.Sprint(entry.Message, entry.ContextMap()), "owner-private-payload") {
			t.Fatal("payload leaked to logger")
		}
		if entry.Message != "events append outcome" {
			continue
		}
		fields := entry.ContextMap()
		if fields["topic"] != "chat-session/abc/events" || fields["source_id"] != "worker-1" || fields["position"] != uint64(1) {
			t.Fatalf("unsafe or missing correlation: %v", fields)
		}
		accepted = accepted || fields["outcome"] == "accepted"
		duplicate = duplicate || fields["outcome"] == "duplicate"
	}
	if !accepted || !duplicate {
		t.Fatal("supplied logger lost append outcomes")
	}
}

func assertOwnerObservations(t *testing.T, service events.Service) {
	t.Helper()
	ctx := t.Context()
	req := validAppendRequest()
	req.Payload = json.RawMessage(`{"content":"owner-private-payload"}`)
	first, err := service.Append(ctx, req)
	if err != nil || first.Outcome != events.AppendOutcomeAccepted || first.Record.ID.Position != 1 {
		t.Fatalf("first append = %+v, %v", first, err)
	}
	duplicate, err := service.Append(ctx, req)
	if err != nil || duplicate.Outcome != events.AppendOutcomeDuplicate || !reflect.DeepEqual(first.Record, duplicate.Record) {
		t.Fatalf("duplicate = %+v, %v", duplicate, err)
	}
	read, err := service.Read(ctx, events.ReadRequest{Topic: req.Topic, From: events.Cursor{Topic: req.Topic}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := events.ReadResult{Records: []events.Record{first.Record}, Next: events.Cursor{Topic: req.Topic, Position: 1}, Retained: events.RetainedRange{Topic: req.Topic, Earliest: 1, Head: 1}, Outcome: events.ReadOutcomeProgress}
	if !reflect.DeepEqual(read, want) {
		t.Fatalf("read = %+v, %v", read, err)
	}
	child, cancel := context.WithCancel(ctx)
	sub, err := service.Subscribe(child, events.SubscribeRequest{Topic: req.Topic, From: read.Next, Limit: 10})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	if delivery := sub.Next(child); delivery.Kind != events.DeliveryCanceled {
		t.Fatalf("canceled delivery = %+v", delivery)
	}
	if err := service.(eventsWireTestCloser).Close(ctx); err != nil {
		t.Fatal(err)
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

	if _, err := service.Append(ctx, validAppendRequest()); !errors.Is(err, events.ErrOperationFailed) {
		t.Fatalf("Append() after Close() error = %v, want it classified as events.ErrOperationFailed", err)
	}
}
