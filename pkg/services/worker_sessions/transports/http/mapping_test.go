package http

import (
	"encoding/json"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestWorkerSessionObservationToAPIPreservesOptionalTurnUsage(t *testing.T) {
	populated := WorkerSessionObservationToAPI(workersessions.Observation{
		WorkerSessionID: "worker-session-1",
		AttemptID:       "attempt-1",
		State:           workersessions.StateCompleted,
		DurationBasis:   workersessions.DurationBasisRecordedTimestamps,
		Transcript:      workersessions.TranscriptAvailabilityAvailable,
		TurnUsage:       &workersessions.TurnUsage{TurnCount: 3, FinalContextTokens: 450, PeakContextTokens: 700},
	})
	if populated.TurnUsage == nil || populated.TurnUsage.TurnCount != 3 || populated.TurnUsage.FinalContextTokens != 450 || populated.TurnUsage.PeakContextTokens != 700 {
		t.Fatalf("mapped turn usage = %#v, want count/final/peak 3/450/700", populated.TurnUsage)
	}
	assertTurnUsageJSONPresence(t, populated, true)

	omitted := WorkerSessionObservationToAPI(workersessions.Observation{
		WorkerSessionID: "worker-session-2",
		AttemptID:       "attempt-2",
		State:           workersessions.StateCompleted,
		DurationBasis:   workersessions.DurationBasisRecordedTimestamps,
		Transcript:      workersessions.TranscriptAvailabilityUnavailable,
	})
	if omitted.TurnUsage != nil {
		t.Fatalf("mapped turn usage = %#v, want nil without supported evidence", omitted.TurnUsage)
	}
	assertTurnUsageJSONPresence(t, omitted, false)
}

func assertTurnUsageJSONPresence(t *testing.T, observation any, wantPresent bool) {
	t.Helper()
	payload, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("marshal observation: %v", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode observation: %v", err)
	}
	_, present := document["turnUsage"]
	if present != wantPresent {
		t.Fatalf("turnUsage present = %t, want %t; payload=%s", present, wantPresent, payload)
	}
}
