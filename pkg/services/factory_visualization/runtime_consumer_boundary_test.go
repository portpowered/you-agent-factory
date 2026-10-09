package factory_visualization_test

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
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/service"
)

// Selected runtime source retains its event stream and maps captured owner facts.
func TestRuntimeSourcePreservesSelectedObservationAndRetainedStream(t *testing.T) {
	t.Parallel()

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
			return fn(&factorysessions.LiveRuntime{
				Factory:             runtimeFactory,
				WorkAndEventIngress: runtimeFactory,
			})
		},
	}
	source := internalservice.NewRuntimeSourceOpening(reader)("selected-session")
	stream, err := source.SubscribeFactoryEvents(t.Context(), nil, factorydefinitions.FactoryEventReconnectScope{})
	if err != nil || stream != runtimeFactory.stream || len(stream.History) != 1 || stream.History[0].Id != history.Id {
		t.Fatalf("selected stream=(%+v,%v)", stream, err)
	}
	facts, err := internalservice.NewRuntimeSourceOpening(reader)("selected-session").GetRuntimeSnapshotFacts(context.Background())
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

	if facts.TickCount != 13 {
		t.Fatalf("snapshot tick=%d, want 13", facts.TickCount)
	}
	if len(runtimeFactory.observeRequests) != 1 || runtimeFactory.observeRequests[0].Scope != factoryruntime.ObservationScopeFull {
		t.Fatalf("observe requests=%+v", runtimeFactory.observeRequests)
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
				Status:   factoryruntime.ObservationStatusActive,
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
	source := internalservice.NewRuntimeSourceOpening(reader)("selected-session")

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

// The source preserves observation failures from its injected runtime.
func TestRuntimeSourcePropagatesObserveFailure(t *testing.T) {
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
	source := internalservice.NewRuntimeSourceOpening(reader)("selected-session")
	_, err := source.GetRuntimeSnapshotFacts(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("snapshot error=%v, want %v", err, wantErr)
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
