package runtime

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestRecordedWorkerSessionObservationScopesCanonicalEventsToFactorySession(t *testing.T) {
	firstSessionID := "factory-session-first"
	secondSessionID := "factory-session-second"
	events := []interfaces.FactoryEvent{
		{Context: interfaces.FactoryEventContext{SessionID: &firstSessionID, Tick: 1, Sequence: 1}},
		{Context: interfaces.FactoryEventContext{SessionID: &secondSessionID, Tick: 1, Sequence: 2}},
		{Context: interfaces.FactoryEventContext{SessionID: &firstSessionID, Tick: 2, Sequence: 3}},
	}

	service := &recordedWorkerSessionObservation{
		ledger:           &recordingfixtures.ScriptedRuntimeLedger{Events: events},
		factorySessionID: firstSessionID,
	}

	scoped := service.canonicalEvents()
	if len(scoped) != 2 {
		t.Fatalf("canonicalEvents() returned %d events, want two scoped events: %#v", len(scoped), scoped)
	}
	for _, event := range scoped {
		if event.Context.SessionID == nil || *event.Context.SessionID != firstSessionID {
			t.Fatalf("canonicalEvents() returned foreign session event: %#v", event)
		}
	}
}

func TestRecordedWorkerSessionObservationFiltersLiveFleetPageToFactorySession(t *testing.T) {
	page := []workersessions.Observation{
		{WorkerSessionID: "worker-first", FactorySessionID: "factory-session-first"},
		{WorkerSessionID: "worker-second", FactorySessionID: "factory-session-second"},
		{WorkerSessionID: "worker-recorded", FactorySessionID: ""},
	}
	recorded := []workersessions.Observation{{WorkerSessionID: "worker-recorded"}}

	filtered := filterObservationPageForFactorySession(page, "factory-session-first", recorded)
	if got := []string{filtered[0].WorkerSessionID, filtered[1].WorkerSessionID}; !reflect.DeepEqual(got, []string{"worker-first", "worker-recorded"}) {
		t.Fatalf("filtered Worker Session IDs = %v, want first-session and recorded identities", got)
	}

	defaultFiltered := filterObservationPageForFactorySession(
		[]workersessions.Observation{{WorkerSessionID: "worker-direct"}},
		factory_context.DefaultSessionID,
		nil,
	)
	if got := []string{defaultFiltered[0].WorkerSessionID}; !reflect.DeepEqual(got, []string{"worker-direct"}) {
		t.Fatalf("default filtered Worker Session IDs = %v, want direct identity", got)
	}
}
