package runtime

import (
	"encoding/json"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestRestoredDefaultCapturePinsPhysicalOwner(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name, scope, dispatch, openingOwner string
		resolves                            bool
	}{
		{"default history", "~default", "dispatch", "original", true},
		{"foreign dispatch", "~default", "foreign", "original", false},
		{"foreign opening owner", "~default", "dispatch", "foreign", false},
		{"explicit foreign scope", "foreign", "dispatch", "original", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			scope, dispatch := scenario.scope, "dispatch"
			event := interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
				Context: interfaces.FactoryEventContext{SessionID: &scope, DispatchID: &dispatch},
				Payload: []byte(`{"workerSessionId":"worker"}`)}
			reader := &selectedCapturedIdentityReader{summary: selectedCapturedUsageSummary("worker", 4)}
			reader.summary.Capture.Catalog.FactorySessionID = "original"
			reader.summary.Capture.Opening.Payload, _ = json.Marshal(map[string]any{
				"kind": "SESSION", "phase": "STARTED", "payload": map[string]string{
					"workerSessionId": "worker", "dispatchId": scenario.dispatch, "factorySessionId": scenario.openingOwner,
				},
			})
			history := prepareRecordedObservationHistory([]interfaces.FactoryEvent{event}, nil, reader)
			service := &recordedWorkerSessionObservation{restoredWorkerScopes: history.restoredWorkerScopes}
			if got := service.selectedCaptureMatches("worker", reader.summary.Capture.Catalog); got != scenario.resolves {
				t.Fatalf("capture membership = %v, want %v; scopes=%v", got, scenario.resolves, history.restoredWorkerScopes)
			}
			if scenario.resolves {
				foreign := reader.summary.Capture.Catalog
				foreign.FactorySessionID = "foreign"
				if service.selectedCaptureMatches("worker", foreign) {
					t.Fatal("prepared owner accepted a foreign capture")
				}
				foreign = reader.summary.Capture.Catalog
				foreign.WorkerSessionID = "other"
				if service.selectedCaptureMatches("worker", foreign) {
					t.Fatal("prepared owner accepted another physical identity")
				}
				// The prepared owner stays detached from subsequent catalog reads.
				reader.summary.Capture.Catalog.FactorySessionID = "foreign"
				if history.restoredWorkerScopes["worker"] != "original" {
					t.Fatal("catalog mutation poisoned prepared owner")
				}
			}
		})
	}
}
