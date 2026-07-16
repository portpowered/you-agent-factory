package providers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/workers/provider/agy"
	"go.uber.org/zap"
)

func TestRuntimeBuildConstructionRejectsMissingOwnedDependencies(t *testing.T) {
	t.Parallel()

	build := func(context.Context, runtimebuild.SessionBuildSpec) (any, error) {
		return struct{}{}, nil
	}
	tests := []struct {
		name   string
		clock  factory.Clock
		logger *zap.Logger
		build  runtimebuild.BundleBuilder
		want   string
	}{
		{name: "clock", logger: zap.NewNop(), build: build, want: "clock is required"},
		{name: "logger", clock: platformclock.Real{}, build: build, want: "logger is required"},
		{name: "builder", clock: platformclock.Real{}, logger: zap.NewNop(), want: "runtime builder is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := runtimebuild.New(runtimebuild.Config{}, test.clock, test.logger, test.build)
			if service != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() = (%v, %v), want nil service and error containing %q", service, err, test.want)
			}
		})
	}
}

func TestAgyConstructionRejectsMissingPTYAllocator(t *testing.T) {
	t.Parallel()

	adapter, err := agy.NewAdapterWithAllocator(t.TempDir(), nil)
	if adapter != nil || err == nil || !strings.Contains(err.Error(), "PTY allocator is required") {
		t.Fatalf("NewAdapterWithAllocator() = (%v, %v), want nil adapter and required-allocator error", adapter, err)
	}
}

func TestRecordedEventBridgePreservesCanonicalEvent(t *testing.T) {
	t.Parallel()

	history := factoryevents.NewFactoryEventHistory(nil, nil)
	eventTime := time.Date(2026, time.July, 16, 1, 2, 3, 0, time.FixedZone("test", -7*60*60))
	history.AppendRecordedEvent(interfaces.FactoryEvent{
		Id:      "recorded-event-1",
		Type:    interfaces.FactoryEventTypeRunRequest,
		Context: interfaces.FactoryEventContext{EventTime: eventTime},
	})

	recorded := history.CanonicalEvents()
	if len(recorded) != 1 {
		t.Fatalf("len(Events()) = %d, want 1", len(recorded))
	}
	if recorded[0].Id != "recorded-event-1" || recorded[0].SchemaVersion != interfaces.FactoryEventSchemaVersionV1 {
		t.Fatalf("recorded event identity = (%q, %q), want preserved ID and canonical schema", recorded[0].Id, recorded[0].SchemaVersion)
	}
	if got, want := recorded[0].Context.EventTime, eventTime.UTC(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("recorded event time = %v (%v), want %v (UTC)", got, got.Location(), want)
	}
}

func TestSessionResponseStreamPublishesAndCompletesOneOrderedDispatch(t *testing.T) {
	t.Parallel()

	stream := responsestream.NewSessionResponseStream()
	subscription, err := stream.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	t.Cleanup(subscription.Detach)

	stored, compaction := stream.Append(responsestream.Event{
		Kind:       responsestream.EventKindResponseFragment,
		Type:       responsestream.EventTypeTextDelta,
		DispatchID: "dispatch-1",
		Payload:    "hello",
	})
	if compaction != nil || stored.Sequence != 1 {
		t.Fatalf("Append() = sequence %d, compaction %+v; want sequence 1 without compaction", stored.Sequence, compaction)
	}

	result, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if len(result.Events) != 1 || result.Events[0].Payload != "hello" {
		t.Fatalf("Next() events = %+v, want one ordered response fragment", result.Events)
	}
	if got := stream.RetentionAccounting(); got.EventCount != 1 || got.TotalPayloadBytes != len("hello") {
		t.Fatalf("RetentionAccounting() = %+v, want one five-byte event", got)
	}
	if stream.LatestSequence() != 1 || len(stream.EventsAfter(0).Events) != 1 || stream.SubscriberCount() != 1 {
		t.Fatalf("live stream state = latest %d, retained %d, subscribers %d; want 1, 1, 1", stream.LatestSequence(), len(stream.EventsAfter(0).Events), stream.SubscriberCount())
	}

	stream.CompleteDispatch()
	if !stream.DispatchCompleted() || stream.DispatchCompletedAt().IsZero() || stream.SubscriberCount() != 0 {
		t.Fatalf("completed stream state = completed %t at %v with %d subscribers", stream.DispatchCompleted(), stream.DispatchCompletedAt(), stream.SubscriberCount())
	}
	if _, err := subscription.Next(context.Background()); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("Next() after completion error = %v, want %v", err, responsestream.ErrSubscriptionClosed)
	}
	stream.Append(responsestream.Event{Payload: "ignored after completion"})
	if got := stream.Events(); len(got) != 1 {
		t.Fatalf("len(Events()) after completion append = %d, want retained event only", len(got))
	}
}
