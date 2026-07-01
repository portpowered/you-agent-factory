package apicontract_test

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/sessionpersistence"
)

// Gate evidence for session-persistence-hardening-and-observability-005: proves the
// verification gates still protect observable recovery behavior, not only compile-time
// contract shape.
func TestSessionPersistenceHardeningGateEvidence_RecoveryOutcomesControlCheckpointReuse(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	catalog := loadSyncPreflightRecoveryFixtureCatalog(t)

	var reusableScenario *syncPreflightRecoveryScenario
	var staleScenario *syncPreflightRecoveryScenario
	for index := range catalog.Scenarios {
		scenario := catalog.Scenarios[index]
		switch scenario.Tags.ReasonCode {
		case "ok":
			reusableScenario = &scenario
		case "cursor_stale":
			staleScenario = &scenario
		}
	}
	if reusableScenario == nil || staleScenario == nil {
		t.Fatal("sync preflight recovery fixtures missing ok or cursor_stale scenario")
	}

	assertSyncPreflightRecoveryScenarioFixture(t, doc, *reusableScenario)
	assertSyncPreflightRecoveryScenarioFixture(t, doc, *staleScenario)

	var reusableResponse factoryapi.FactorySessionSyncPreflightResponse
	assertGeneratedFixtureRoundTrip(t, reusableScenario.Response, "FactorySessionSyncPreflightResponse", func(raw []byte) {
		decodeRoundTripJSON(t, raw, &reusableResponse, reusableScenario.ID+" reusable response")
	})
	var staleResponse factoryapi.FactorySessionSyncPreflightResponse
	assertGeneratedFixtureRoundTrip(t, staleScenario.Response, "FactorySessionSyncPreflightResponse", func(raw []byte) {
		decodeRoundTripJSON(t, raw, &staleResponse, staleScenario.ID+" stale response")
	})

	if !reusableResponse.CheckpointReusable {
		t.Fatal("reusable recovery outcome checkpointReusable = false, want true")
	}
	if staleResponse.CheckpointReusable {
		t.Fatal("stale cursor recovery outcome checkpointReusable = true, want false")
	}

	staleDiagnostic, ok := sessionpersistence.InvalidationFromSyncPreflight(staleResponse)
	if !ok {
		t.Fatal("stale cursor recovery outcome missing invalidation diagnostic")
	}
	if staleDiagnostic.Reason != sessionpersistence.ReasonCursorStale {
		t.Fatalf("stale cursor diagnostic reason = %q, want %q", staleDiagnostic.Reason, sessionpersistence.ReasonCursorStale)
	}
	if staleDiagnostic.RecoveryAction != sessionpersistence.RecoveryReplayWithoutCursor {
		t.Fatalf(
			"stale cursor diagnostic recovery = %q, want %q",
			staleDiagnostic.RecoveryAction,
			sessionpersistence.RecoveryReplayWithoutCursor,
		)
	}
}
