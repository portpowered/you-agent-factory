package projections

import (
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const (
	modulePrefix              = "github.com/portpowered/infinite-you/"
	factoryRuntimeRoot        = modulePrefix + "pkg/services/factory_runtime"
	recordingsProjectionsRoot = modulePrefix + "pkg/services/recordings/internal/projections"
)

// TestProjectionsImportRuntimeRootOnly seals CUT-REC-RUN story 004: Recordings
// projection and observation surfaces may depend on Factory Runtime only through
// the service root contract.

// TestProjectionsConstructRuntimeObservationAndResultShapesThroughRoot proves
// projection/observation edges construct Runtime-owned marking, result, and
// observation vocabulary through the sealed Runtime root boundary.
func TestProjectionsConstructRuntimeObservationAndResultShapesThroughRoot(t *testing.T) {
	t.Parallel()

	uptime := 11 * time.Second
	engineSnapshot := factoryruntime.DashboardEngineStateSnapshot(
		"RUNNING",
		interfaces.RuntimeStatusActive,
		11,
		uptime,
	)
	if engineSnapshot.FactoryState != "RUNNING" || engineSnapshot.TickCount != 11 || engineSnapshot.Uptime != uptime {
		t.Fatalf("engine snapshot = %#v, want RUNNING tick=11 uptime=%v", engineSnapshot, uptime)
	}

	observeResult := factoryruntime.ObserveResult{
		Observation: factoryruntime.Observation{
			Status: factoryruntime.ObservationStatusActive,
			Progress: factoryruntime.ObservationProgress{
				InFlightDispatchCount: 1,
				TickCount:             engineSnapshot.TickCount,
			},
			Health: factoryruntime.ObservationHealth{
				FactoryState: engineSnapshot.FactoryState,
				Uptime:       engineSnapshot.Uptime,
			},
		},
	}
	if observeResult.Observation.Progress.TickCount != 11 {
		t.Fatalf("observation tick count = %d, want 11", observeResult.Observation.Progress.TickCount)
	}
	if observeResult.Observation.Health.FactoryState != "RUNNING" {
		t.Fatalf("observation factory state = %q, want RUNNING", observeResult.Observation.Health.FactoryState)
	}

	sessionID := "session-projection-root"
	primaryJSON, err := json.Marshal(map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	resultParts := []work.WorkContentPart{{
		Type: work.WorkContentPartTypeJSON,
		JSON: primaryJSON,
	}}
	ownerProjection := factoryruntime.SessionResultProjection{
		Durable: factoryruntime.SessionResult{
			SessionID:     sessionID,
			ResultStatus:  factoryruntime.ResultStatusFinal,
			PrimaryResult: resultParts,
		},
		Updated: factoryruntime.SessionResultUpdatedPayload{
			ResultStatus:  interfaces.FactorySessionResultStatusFinal,
			ResultSummary: resultParts,
		},
	}
	eventPayload := apisurface.WorkflowSessionResultUpdatedPayloadToAPI(ownerProjection.Updated)
	t0 := time.Date(2026, 7, 27, 18, 0, 0, 0, time.UTC)
	events := []interfaces.FactoryEvent{
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionResultUpdated, interfaces.FactoryEventContext{
			EventTime: t0.Add(2 * time.Second),
			Tick:      2,
		}, interfaces.FactorySessionResultUpdatedEventPayload{
			ResultStatus:  interfaces.FactorySessionResultStatusFinal,
			ResultSummary: resultParts,
		}),
	}
	worldState, err := ReconstructCanonicalFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructCanonicalFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket == nil || worldState.SessionBracket.ResultStatus != string(interfaces.FactorySessionResultStatusFinal) {
		t.Fatalf("session bracket = %#v, want FINAL result status", worldState.SessionBracket)
	}
	durableResult := apisurface.WorkflowSessionResultToAPI(ownerProjection.Durable)
	if durableResult.SessionId != sessionID {
		t.Fatalf("durable session id = %q, want %q", durableResult.SessionId, sessionID)
	}
	if eventPayload.ResultStatus != factoryapi.FactoryEventSessionResultStatusFinal {
		t.Fatalf("event payload result status = %q, want FINAL", eventPayload.ResultStatus)
	}
}
