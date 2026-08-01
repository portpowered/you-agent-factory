package factory_visualization_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualizationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/wire"
)

// TestRuntimeObservationUsesRootServiceObserve proves CUT-VIS-RUN story 002:
// leased Visualization observation paths construct root ObserveRequest values
// and call Service.Observe, mapping returned observation facts into
// Visualization-owned runtime snapshot fields.
func TestRuntimeObservationUsesRootServiceObserve(t *testing.T) {
	t.Parallel()

	uptime := 42 * time.Second
	runtimeFactory := &sessionBoundRuntimeFactory{
		stream: &factorydefinitions.FactoryEventStream{
			Events: make(chan factorydefinitions.FactoryEvent),
		},
		observation: factoryruntime.Observation{
			Status: factoryruntime.ObservationStatusActive,
			Progress: factoryruntime.ObservationProgress{
				TickCount: 7,
			},
			Health: factoryruntime.ObservationHealth{
				FactoryState: "RUNNING",
				Uptime:       uptime,
				ActiveThrottlePauses: []factorydefinitions.ActiveThrottlePause{
					{Provider: "provider-a", Model: "model-a"},
				},
			},
		},
	}
	reader := sessionRuntimeReaderStub{
		withRuntimeRead: func(fn func(*factorysessions.LiveRuntime) error) error {
			return fn(&factorysessions.LiveRuntime{Factory: runtimeFactory})
		},
	}
	source := factoryvisualizationwire.NewCurrentRuntimeSource(reader)

	facts, err := source.GetRuntimeSnapshotFacts(context.Background())
	if err != nil {
		t.Fatalf("GetRuntimeSnapshotFacts: %v", err)
	}
	if len(runtimeFactory.observeRequests) != 1 {
		t.Fatalf("observe calls = %d, want 1 root observation path", len(runtimeFactory.observeRequests))
	}
	if runtimeFactory.observeRequests[0].Scope != factoryruntime.ObservationScopeFull {
		t.Fatalf(
			"observe scope = %q, want %q",
			runtimeFactory.observeRequests[0].Scope,
			factoryruntime.ObservationScopeFull,
		)
	}
	if facts == nil {
		t.Fatal("GetRuntimeSnapshotFacts returned nil facts")
	}
	if facts.TickCount != 7 {
		t.Fatalf("tick count = %d, want 7", facts.TickCount)
	}
	if facts.FactoryState != "RUNNING" {
		t.Fatalf("factory state = %q, want RUNNING", facts.FactoryState)
	}
	if facts.RuntimeStatus != factorydefinitions.RuntimeStatusActive {
		t.Fatalf("runtime status = %q, want ACTIVE", facts.RuntimeStatus)
	}
	if facts.Uptime != uptime {
		t.Fatalf("uptime = %v, want %v", facts.Uptime, uptime)
	}
	if len(facts.ActiveThrottlePauses) != 1 ||
		facts.ActiveThrottlePauses[0].Provider != "provider-a" ||
		facts.ActiveThrottlePauses[0].Model != "model-a" {
		t.Fatalf("active throttle pauses = %#v, want provider-a/model-a", facts.ActiveThrottlePauses)
	}
}

func TestRuntimeObservationPropagatesRootObserveFailure(t *testing.T) {
	t.Parallel()

	wantErr := factoryruntime.ErrNotRunning
	runtimeFactory := &sessionBoundRuntimeFactory{
		stream: &factorydefinitions.FactoryEventStream{
			Events: make(chan factorydefinitions.FactoryEvent),
		},
		observeErr: wantErr,
	}
	reader := sessionRuntimeReaderStub{
		withRuntimeRead: func(fn func(*factorysessions.LiveRuntime) error) error {
			return fn(&factorysessions.LiveRuntime{Factory: runtimeFactory})
		},
	}
	source := factoryvisualizationwire.NewCurrentRuntimeSource(reader)

	_, err := source.GetRuntimeSnapshotFacts(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("GetRuntimeSnapshotFacts error = %v, want %v", err, wantErr)
	}
	if len(runtimeFactory.observeRequests) != 1 {
		t.Fatalf("observe calls = %d, want 1 before failure propagation", len(runtimeFactory.observeRequests))
	}
}

func TestRuntimeObservationUnavailableRuntimeDoesNotCallObserve(t *testing.T) {
	t.Parallel()

	observeCalls := 0
	runtimeFactory := &sessionBoundRuntimeFactory{
		observeRequests: make([]factoryruntime.ObserveRequest, 0),
	}
	reader := sessionRuntimeReaderStub{
		withRuntimeRead: func(func(*factorysessions.LiveRuntime) error) error {
			return factorysessions.ErrRuntimeNotAvailable
		},
	}
	source := factoryvisualizationwire.NewCurrentRuntimeSource(reader)

	_, err := source.GetRuntimeSnapshotFacts(context.Background())
	if !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("GetRuntimeSnapshotFacts error = %v, want ErrRuntimeNotAvailable", err)
	}
	observeCalls = len(runtimeFactory.observeRequests)
	if observeCalls != 0 {
		t.Fatalf("observe calls = %d, want 0 when runtime is unavailable", observeCalls)
	}
}

func TestRuntimeSubscribeUsesMigrationOnlyAPIFactoryCast(t *testing.T) {
	t.Parallel()

	runtimeFactory := &sessionBoundRuntimeFactory{
		stream: &factorydefinitions.FactoryEventStream{
			Events: make(chan factorydefinitions.FactoryEvent),
		},
	}
	reader := sessionRuntimeReaderStub{
		withRuntimeRead: func(fn func(*factorysessions.LiveRuntime) error) error {
			return fn(&factorysessions.LiveRuntime{Factory: runtimeFactory})
		},
	}
	source := factoryvisualizationwire.NewCurrentRuntimeSource(reader)

	stream, err := source.SubscribeFactoryEvents(
		context.Background(),
		nil,
		factorydefinitions.FactoryEventReconnectScope{},
	)
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents: %v", err)
	}
	if stream == nil || stream.Events == nil {
		t.Fatal("SubscribeFactoryEvents returned invalid stream")
	}
	if len(runtimeFactory.observeRequests) != 0 {
		t.Fatalf("observe calls during subscribe = %d, want 0", len(runtimeFactory.observeRequests))
	}
}
