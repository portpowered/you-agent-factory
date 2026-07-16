package sessionparity

import (
	"encoding/json"
	"testing"
)

func TestProjectionContract_RetainsEveryStableFactorySessionFact(t *testing.T) {
	phase := "completed"
	requestedPolicyHash := "sha256:requested-policy"
	effectivePolicyHash := "sha256:effective-policy"
	dispatchID := "dispatch-1"
	projection := Projection{
		Identity:  FactorySessionIdentity{SessionID: "session-1"},
		Lifecycle: LifecycleFacts{Status: "SUCCEEDED", Phase: &phase},
		Hashes: HashFacts{
			SourceHash:          "sha256:source",
			RequestedPolicyHash: &requestedPolicyHash,
			EffectivePolicyHash: &effectivePolicyHash,
		},
		Progress:     ProgressFacts{TotalDispatches: 1, CompletedDispatches: 1},
		Dispatches:   []DispatchFact{{SessionID: "session-1", ID: dispatchID, Order: 1, Status: "SUCCEEDED", Kind: "PETRI_TRANSITION"}},
		Artifacts:    []ArtifactFact{{SessionID: "session-1", ID: "artifact-1", Order: 1, Kind: "RESULT"}},
		Results:      []ResultFact{{SessionID: "session-1", ID: "result-1", Order: 1, Status: "FINAL", Value: "done"}},
		Failures:     []FailureFact{{SessionID: "session-1", ID: "failure-1", Order: 1, Code: "RETRIED", Message: "recovered", DispatchID: &dispatchID}},
		EventCursors: []FactoryEventCursor{{SessionID: "session-1", Cursor: "cursor-1", Sequence: 1, EventType: "FACTORY_SESSION_COMPLETED"}},
	}

	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	var observed map[string]any
	if err := json.Unmarshal(encoded, &observed); err != nil {
		t.Fatalf("unmarshal projection: %v", err)
	}
	for _, field := range []string{"identity", "lifecycle", "hashes", "progress", "dispatches", "artifacts", "results", "failures", "eventCursors"} {
		if _, ok := observed[field]; !ok {
			t.Errorf("projection omitted stable field %q", field)
		}
	}
}

func TestProjectionContract_PreservesOptionalStableFactAbsence(t *testing.T) {
	projection := Projection{
		Identity:  FactorySessionIdentity{SessionID: "session-1"},
		Lifecycle: LifecycleFacts{Status: "RUNNING"},
		Hashes:    HashFacts{SourceHash: "sha256:source"},
	}

	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	var observed map[string]any
	if err := json.Unmarshal(encoded, &observed); err != nil {
		t.Fatalf("unmarshal projection: %v", err)
	}
	lifecycle := observed["lifecycle"].(map[string]any)
	if _, ok := lifecycle["phase"]; ok {
		t.Fatal("phase present, want meaningful optional absence")
	}
	hashes := observed["hashes"].(map[string]any)
	for _, field := range []string{"requestedPolicyHash", "effectivePolicyHash"} {
		if _, ok := hashes[field]; ok {
			t.Errorf("%s present, want meaningful optional absence", field)
		}
	}
}
