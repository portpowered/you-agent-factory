package projections_test

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/projections"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factoryeventprojection"
)

// ReconstructFactoryWorldState keeps generated-event compatibility assertions
// at the test boundary while production reducers consume canonical events.
func ReconstructFactoryWorldState(
	events []factoryapi.FactoryEvent,
	selectedTick int,
) (interfaces.FactoryWorldState, error) {
	return factoryeventprojection.ReconstructFactoryWorldState(projections.ReconstructCanonicalFactoryWorldState, events, selectedTick)
}

func TestReconstructFactoryWorldState_PauseResumeHistoryReconstructsLifecycleControlStatus(t *testing.T) {
	t0 := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	sessionID := "session-pause-resume"
	kind := factoryapi.JAVASCRIPT
	source := "runtime"
	events := []factoryapi.FactoryEvent{
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionStarted, "event-session-started", 1, t0, factoryapi.FactoryEventContext{
			Sequence:         1,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(0),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionStartedEventPayload{
			StartedAt: t0,
		}),
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionPaused, "event-session-paused", 2, t0.Add(time.Second), factoryapi.FactoryEventContext{
			Sequence:         2,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(1),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionPausedEventPayload{
			Status:   factoryapi.FactorySessionDurableLifecycleStatusPaused,
			PausedAt: t0.Add(time.Second),
		}),
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionResumed, "event-session-resumed", 3, t0.Add(2*time.Second), factoryapi.FactoryEventContext{
			Sequence:         3,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(2),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionResumedEventPayload{
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
			ResumedAt: t0.Add(2 * time.Second),
		}),
	}

	worldState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	bracket := worldState.SessionBracket
	if bracket == nil {
		t.Fatal("session bracket = nil, want pause/resume lifecycle")
	}
	if bracket.LifecycleControlStatus != string(factoryapi.FactorySessionDurableLifecycleStatusRunning) {
		t.Fatalf("lifecycle control status = %q, want RUNNING", bracket.LifecycleControlStatus)
	}
	if bracket.PausedAt.IsZero() || bracket.ResumedAt.IsZero() {
		t.Fatalf("paused/resumed timestamps = %#v, want both set", bracket)
	}
}
