package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	runtimehttp "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http"
)

type countingObservationLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger
	canonicalEvents atomic.Int32
}

func (l *countingObservationLedger) CanonicalEvents() []interfaces.FactoryEvent {
	l.canonicalEvents.Add(1)
	return l.ScriptedRuntimeLedger.CanonicalEvents()
}

func TestFactoryImpl_ObserveDoesNotVisitCanonicalHistory(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ledger := &countingObservationLedger{
		ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{
			Events:       make([]interfaces.FactoryEvent, 1600),
			GenerationID: "bounded-observation-generation",
		},
	}
	impl.eventHistory = ledger
	var projectorCalls atomic.Int32
	impl.cfg.worldStateProjector = func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
		projectorCalls.Add(1)
		return interfaces.FactoryWorldState{}, nil
	}
	impl.state = interfaces.FactoryStateRunning

	scopes := []factory.ObservationScope{
		factory.ObservationScopeFull,
		factory.ObservationScopeStatus,
		factory.ObservationScopeProgress,
		factory.ObservationScopeDispatches,
		factory.ObservationScopeResults,
		factory.ObservationScopeResources,
		factory.ObservationScopeHealth,
	}
	for _, scope := range scopes {
		t.Run(string(scope), func(t *testing.T) {
			result, err := impl.Observe(context.Background(), factory.ObserveRequest{Scope: scope})
			if err != nil {
				t.Fatalf("Observe(%s): %v", scope, err)
			}
			if result.Observation.Health.StreamGenerationID != "bounded-observation-generation" && scope == factory.ObservationScopeFull {
				t.Fatalf("Observe(%s) stream generation = %q, want bounded-observation-generation", scope, result.Observation.Health.StreamGenerationID)
			}
		})
	}

	if got := ledger.canonicalEvents.Load(); got != 0 {
		t.Fatalf("CanonicalEvents calls = %d, want 0", got)
	}
	if got := projectorCalls.Load(); got != 0 {
		t.Fatalf("world-state projector calls = %d, want 0", got)
	}
}

func TestFactoryImpl_ObserveReturnsContextFailureWithoutPartialObservation(t *testing.T) {
	impl := newRootContractTestFactory(t)
	impl.state = interfaces.FactoryStateRunning
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := impl.Observe(ctx, factory.ObserveRequest{Scope: factory.ObservationScopeFull})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Observe(canceled) error = %v, want context.Canceled", err)
	}
	if !reflect.DeepEqual(result, factory.ObserveResult{}) {
		t.Fatalf("Observe(canceled) result = %#v, want zero result", result)
	}
}

func TestFactoryImpl_StatusHTTPDoesNotVisitCanonicalHistory(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ledger := &countingObservationLedger{
		ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{
			Events: make([]interfaces.FactoryEvent, 1600),
		},
	}
	impl.eventHistory = ledger
	var projectorCalls atomic.Int32
	impl.cfg.worldStateProjector = func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
		projectorCalls.Add(1)
		return interfaces.FactoryWorldState{}, nil
	}
	impl.state = interfaces.FactoryStateRunning

	adapter := runtimehttp.NewAdapter(impl)
	recorder := httptest.NewRecorder()
	adapter.GetStatus(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if got := ledger.canonicalEvents.Load(); got != 0 {
		t.Fatalf("CanonicalEvents calls = %d, want 0", got)
	}
	if got := projectorCalls.Load(); got != 0 {
		t.Fatalf("world-state projector calls = %d, want 0", got)
	}
}

func TestFactoryImpl_ObservePreservesLifecycleFacts(t *testing.T) {
	tests := []struct {
		state       interfaces.FactoryState
		wantStatus  factory.ObservationStatus
		wantControl string
	}{
		{state: interfaces.FactoryStateIdle, wantStatus: factory.ObservationStatusIdle, wantControl: "RUNNING"},
		{state: interfaces.FactoryStateRunning, wantStatus: factory.ObservationStatusIdle, wantControl: "RUNNING"},
		{state: interfaces.FactoryStatePaused, wantStatus: factory.ObservationStatusIdle, wantControl: "PAUSED"},
		{state: interfaces.FactoryStateCompleted, wantStatus: factory.ObservationStatusFinished, wantControl: "SUCCEEDED"},
		{state: interfaces.FactoryStateFailed, wantStatus: factory.ObservationStatusFinished, wantControl: "FAILED"},
	}
	for _, test := range tests {
		t.Run(string(test.state), func(t *testing.T) {
			impl := newRootContractTestFactory(t)
			impl.state = test.state
			result, err := impl.Observe(context.Background(), factory.ObserveRequest{Scope: factory.ObservationScopeFull})
			if err != nil {
				t.Fatalf("Observe(%s): %v", test.state, err)
			}
			observation := result.Observation
			if observation.Status != test.wantStatus {
				t.Fatalf("status = %q, want %q", observation.Status, test.wantStatus)
			}
			if observation.Health.FactoryState != string(test.state) {
				t.Fatalf("factory state = %q, want %q", observation.Health.FactoryState, test.state)
			}
			if observation.Health.LifecycleControlStatus != test.wantControl {
				t.Fatalf("lifecycle control = %q, want %q", observation.Health.LifecycleControlStatus, test.wantControl)
			}
		})
	}
}
