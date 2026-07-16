package providers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
		{name: "logger", clock: factory.RealClock{}, build: build, want: "logger is required"},
		{name: "builder", clock: factory.RealClock{}, logger: zap.NewNop(), want: "runtime builder is required"},
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
	history.AppendRecordedEvent(factoryapi.FactoryEvent{
		Id:      "recorded-event-1",
		Type:    factoryapi.FactoryEventTypeRunRequest,
		Context: factoryapi.FactoryEventContext{EventTime: eventTime},
	})

	recorded := history.Events()
	if len(recorded) != 1 {
		t.Fatalf("len(Events()) = %d, want 1", len(recorded))
	}
	if recorded[0].Id != "recorded-event-1" || recorded[0].SchemaVersion != factoryapi.AgentFactoryEventV1 {
		t.Fatalf("recorded event identity = (%q, %q), want preserved ID and canonical schema", recorded[0].Id, recorded[0].SchemaVersion)
	}
	if got, want := recorded[0].Context.EventTime, eventTime.UTC(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("recorded event time = %v (%v), want %v (UTC)", got, got.Location(), want)
	}
}
