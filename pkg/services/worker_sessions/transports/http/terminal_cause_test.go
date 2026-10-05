package http

import (
	"encoding/json"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestWorkerSessionObservationToAPIPreservesOptionalTerminalCause(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "COMPLETED", "FAILED", "OPERATOR_CANCEL", "OPERATOR_TERMINATE"} {
		observation := workersessions.Observation{}
		if value != "" {
			observation.TerminalCause = &value
		}
		api := WorkerSessionObservationToAPI(observation)
		if value == "" {
			if api.TerminalCause != nil {
				t.Fatal("unknown cause became known")
			}
			continue
		}
		if api.TerminalCause == nil || string(*api.TerminalCause) != value {
			t.Fatalf("mapped cause=%v", api.TerminalCause)
		}
		payload, err := json.Marshal(api)
		var document map[string]any
		if err != nil || json.Unmarshal(payload, &document) != nil || document["terminalCause"] != value {
			t.Fatalf("serialized cause=%s, %v", payload, err)
		}
	}
}
