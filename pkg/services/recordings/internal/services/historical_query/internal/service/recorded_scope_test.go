package service

import (
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"testing"
)

func TestWorkerWorkAttributionInfersOnlySelectedArtifactScope(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct{ name, payload, scope string }{
		{"legacy-default", `{"schemaVersion":"agent-factory.replay.v1","events":[{"context":{"sessionId":"~default"}}]}`, "~default"},
		{"legacy-scoped", `{"schemaVersion":"agent-factory.replay.v1","events":[{"context":{"sessionId":"selected"}}]}`, "selected"},
		{"portable", `{"schemaVersion":"recordings.portable-artifact.v1","summary":{"scope":{"factorySessionId":"selected"}}}`, "selected"},
		{"malformed", `{`, ""},
		{"missing", `{"events":[]}`, ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			identity := recordings.HistoricalRecordingIdentity{RecordingID: "capture", Artifact: "exact.json", Scope: recordings.CanonicalEventScope{FactorySessionID: "requested"}}
			got, err := inferRecordedScope([]byte(scenario.payload), identity)
			if scenario.scope == "" {
				assertHistoricalQueryKind(t, err, recordings.HistoricalRecordingQueryErrorCorruptHistory)
				return
			}
			if err != nil || got.Scope.FactorySessionID != scenario.scope || got.Artifact != identity.Artifact || got.RecordingID != identity.RecordingID {
				t.Fatalf("recorded source identity = %+v, %v", got, err)
			}
		})
	}
}
