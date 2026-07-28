package factory_visualization

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
)

// TestVisualizationConsumerObservationExercisesRuntimeRoot proves CUT-VIS-RUN story 004:
// leased session-bound activation and detached Observe paths reach Factory Runtime
// only through root Service.Observe, mapping returned observation facts into
// activation/view outcomes reviewers can verify.
func TestVisualizationConsumerObservationExercisesRuntimeRoot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 28, 7, 55, 0, 0, time.UTC)
	uptime := 17 * time.Second
	history := event("retained", 1)
	runtimeFactory := &sessionBoundRuntimeFactory{
		stream: &factorydefinitions.FactoryEventStream{
			History: []factorydefinitions.FactoryEvent{history},
			Events:  make(chan factorydefinitions.FactoryEvent),
		},
		observation: factoryruntime.Observation{
			Status: factoryruntime.ObservationStatusActive,
			Progress: factoryruntime.ObservationProgress{
				TickCount: 13,
			},
			Health: factoryruntime.ObservationHealth{
				FactoryState: "RUNNING",
				Uptime:       uptime,
			},
		},
	}
	reader := sessionRuntimeReaderStub{
		withRuntimeRead: func(fn func(*factorysessions.LiveRuntime) error) error {
			return fn(&factorysessions.LiveRuntime{Factory: runtimeFactory})
		},
	}
	emitted := make([]View, 0, 2)
	service, err := New(
		NewCurrentRuntimeSource(reader),
		&recordingsstub.Service{},
		fixedClock{now: now},
		SinkFunc(func(view View) {
			emitted = append(emitted, view)
		}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() with session-bound runtime source: %v", err)
	}
	var root Root = service

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := root.Activate(ctx, ActivateRequest{Mode: ActivateModeRetainedThenLive}); err != nil {
		t.Fatalf("Activate through session-bound runtime: %v", err)
	}

	result, err := root.Observe(context.Background(), ObserveRequest{Mode: ObserveModeRetainedThenLive})
	if err != nil {
		t.Fatalf("Observe after Activate: %v", err)
	}
	if len(runtimeFactory.observeRequests) == 0 {
		t.Fatal("detached Observe did not exercise root Service.Observe")
	}
	lastObserve := runtimeFactory.observeRequests[len(runtimeFactory.observeRequests)-1]
	if lastObserve.Scope != factoryruntime.ObservationScopeFull {
		t.Fatalf("observe scope = %q, want %q", lastObserve.Scope, factoryruntime.ObservationScopeFull)
	}
	if result.View.TickCount != 13 {
		t.Fatalf("Observe TickCount = %d, want 13 from root observation facts", result.View.TickCount)
	}
	if result.View.RetainedEventCount != 1 {
		t.Fatalf("Observe RetainedEventCount = %d, want 1", result.View.RetainedEventCount)
	}
	if !result.View.ObservedAt.Equal(now) {
		t.Fatalf("Observe ObservedAt = %v, want %v", result.View.ObservedAt, now)
	}

	facts, err := NewCurrentRuntimeSource(reader).GetRuntimeSnapshotFacts(context.Background())
	if err != nil {
		t.Fatalf("GetRuntimeSnapshotFacts after Observe: %v", err)
	}
	if facts.FactoryState != "RUNNING" {
		t.Fatalf("snapshot factory state = %q, want RUNNING", facts.FactoryState)
	}
	if facts.RuntimeStatus != factorydefinitions.RuntimeStatusActive {
		t.Fatalf("snapshot runtime status = %q, want ACTIVE", facts.RuntimeStatus)
	}
	if facts.Uptime != uptime {
		t.Fatalf("snapshot uptime = %v, want %v", facts.Uptime, uptime)
	}

	foundActivationTick := false
	for _, view := range emitted {
		if view.Runtime.TickCount == 13 {
			foundActivationTick = true
			break
		}
	}
	if !foundActivationTick {
		t.Fatalf("activation views = %#v, want tick count from root observation facts", emitted)
	}
}

// TestVisualizationConsumerObservationFailsClosedWithoutPetriSnapshot proves the
// leased observation path does not require Petri-shaped GetEngineStateSnapshot or
// RuntimeEngineStateSnapshot aliases. A root Service-only runtime still supplies
// snapshot facts through Service.Observe.
func TestVisualizationConsumerObservationFailsClosedWithoutPetriSnapshot(t *testing.T) {
	t.Parallel()

	runtimeFactory := &rootObservationOnlyFactory{
		sessionBoundRuntimeFactory: sessionBoundRuntimeFactory{
			observation: factoryruntime.Observation{
				Status: factoryruntime.ObservationStatusActive,
				Progress: factoryruntime.ObservationProgress{TickCount: 4},
				Health: factoryruntime.ObservationHealth{
					FactoryState: "RUNNING",
				},
			},
		},
	}
	reader := sessionRuntimeReaderStub{
		withRuntimeRead: func(fn func(*factorysessions.LiveRuntime) error) error {
			return fn(&factorysessions.LiveRuntime{Factory: runtimeFactory})
		},
	}
	source := NewCurrentRuntimeSource(reader)

	facts, err := source.GetRuntimeSnapshotFacts(context.Background())
	if err != nil {
		t.Fatalf("GetRuntimeSnapshotFacts through root Service-only runtime: %v", err)
	}
	if facts == nil || facts.TickCount != 4 {
		t.Fatalf("snapshot facts = %#v, want tick 4 from root Observe", facts)
	}
	if runtimeFactory.snapshotAccessAttempts != 0 {
		t.Fatalf(
			"legacy snapshot access attempts = %d, want 0; observation must not use Petri snapshots",
			runtimeFactory.snapshotAccessAttempts,
		)
	}
	if len(runtimeFactory.observeRequests) != 1 {
		t.Fatalf("observe calls = %d, want 1 root observation path", len(runtimeFactory.observeRequests))
	}
}

// TestVisualizationConsumerDetachedObservePropagatesRootObserveFailure proves typed
// Runtime observation failures propagate through detached Observe as
// Visualization-owned projection failures with the root cause attached.
func TestVisualizationConsumerDetachedObservePropagatesRootObserveFailure(t *testing.T) {
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
	service, err := New(
		NewCurrentRuntimeSource(reader),
		&recordingsstub.Service{},
		fixedClock{now: time.Unix(1, 0)},
		SinkFunc(func(View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() with failing runtime observe: %v", err)
	}

	_, err = service.Observe(context.Background(), ObserveRequest{Mode: ObserveModeRetainedThenLive})
	var projErr *ProjectionError
	if !errors.As(err, &projErr) || projErr.Kind != ProjectionErrorSnapshotUnavailable {
		t.Fatalf("Observe error = %v, want ProjectionErrorSnapshotUnavailable", err)
	}
	if !errors.Is(projErr.Cause, wantErr) {
		t.Fatalf("Observe cause = %v, want %v", projErr.Cause, wantErr)
	}
}

// TestVisualizationRuntimeSourceDoesNotReferencePetriSnapshotHelpers seals the
// production observation adapter against Petri-shaped snapshot helper names.
func TestVisualizationRuntimeSourceDoesNotReferencePetriSnapshotHelpers(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"GetEngineStateSnapshot",
		"RuntimeEngineStateSnapshot",
		"StateSnapshot",
	}
	source, err := os.ReadFile(filepath.Join("runtime_source.go"))
	if err != nil {
		t.Fatalf("read runtime_source.go: %v", err)
	}
	content := string(source)
	for _, needle := range forbidden {
		if strings.Contains(content, needle) {
			t.Fatalf("runtime_source.go contains forbidden Petri snapshot reference %q", needle)
		}
	}
}

// rootObservationOnlyFactory embeds the session-bound Service stub and records
// legacy snapshot access attempts so tests fail closed when a consumer edge
// reaches for Petri-shaped snapshot helpers instead of Service.Observe.
type rootObservationOnlyFactory struct {
	sessionBoundRuntimeFactory
	snapshotAccessAttempts int
}

func (f *rootObservationOnlyFactory) GetEngineStateSnapshot(context.Context) (any, error) {
	f.snapshotAccessAttempts++
	return nil, errors.New("petri snapshot access is forbidden on Visualization consumer edges")
}
