package sessionparity

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizers_EquivalentCapturedObservationsProduceOneProjection(t *testing.T) {
	session := stableSessionJSON(t)
	rest := append([]byte(`{"session":`), append(session, '}')...)
	cli := append([]byte(`{"factorySession":`), append(session, '}')...)
	mcp := append([]byte(`{"jsonrpc":"2.0","id":"request-7","result":{"factorySession":`), append(session, "}}"...)...)

	want, err := NormalizeREST(rest)
	if err != nil {
		t.Fatalf("NormalizeREST: %v", err)
	}
	for _, test := range []struct {
		name      string
		normalize func([]byte) (Projection, error)
		input     []byte
	}{
		{name: "CLI JSON", normalize: NormalizeCLIJSON, input: cli},
		{name: "MCP", normalize: NormalizeMCP, input: mcp},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.normalize(test.input)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("projection mismatch\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}

func TestNormalizeREST_RejectsMissingRequiredStableFact(t *testing.T) {
	_, err := NormalizeREST([]byte(`{"session":{"identity":{"sessionId":"session-1"}}}`))
	if err == nil {
		t.Fatal("NormalizeREST error = nil, want missing lifecycle error")
	}
	want := &NormalizationError{Interface: "REST", Field: "lifecycle", Reason: "is required"}
	if !reflect.DeepEqual(err, want) {
		t.Fatalf("NormalizeREST error = %#v, want %#v", err, want)
	}
}

func TestNormalizeMCP_RejectsReorderedCanonicalEventCursors(t *testing.T) {
	session := stableSessionJSON(t)
	var value map[string]any
	if err := json.Unmarshal(session, &value); err != nil {
		t.Fatalf("unmarshal stable session: %v", err)
	}
	value["eventCursors"] = []any{
		map[string]any{"sessionId": "session-1", "cursor": "cursor-12", "sequence": 12, "eventType": "DISPATCH_COMPLETED"},
		map[string]any{"sessionId": "session-1", "cursor": "cursor-11", "sequence": 11, "eventType": "SESSION_COMPLETED"},
	}
	encoded, err := json.Marshal(map[string]any{"result": map[string]any{"factorySession": value}})
	if err != nil {
		t.Fatalf("marshal observation: %v", err)
	}
	_, err = NormalizeMCP(encoded)
	if err == nil {
		t.Fatal("NormalizeMCP error = nil, want reordered event cursor error")
	}
	want := &NormalizationError{Interface: "MCP", Field: "eventCursors[1]", Reason: "must retain a correlated, strictly ordered event cursor"}
	if !reflect.DeepEqual(err, want) {
		t.Fatalf("NormalizeMCP error = %#v, want %#v", err, want)
	}
}

func TestNormalizeCLIJSON_PreservesCustomerVisibleCollectionOrder(t *testing.T) {
	projection, err := NormalizeCLIJSON([]byte(`{"factorySession":` + string(stableSessionJSON(t)) + `}`))
	if err != nil {
		t.Fatalf("NormalizeCLIJSON: %v", err)
	}
	if got, want := projection.Dispatches[0].ID, "dispatch-2"; got != want {
		t.Fatalf("first dispatch = %q, want %q", got, want)
	}
	if got, want := projection.EventCursors[0].Sequence, int64(11); got != want {
		t.Fatalf("first event sequence = %d, want %d", got, want)
	}
}

func stableSessionJSON(t *testing.T) []byte {
	t.Helper()
	value := Projection{
		Identity:  FactorySessionIdentity{SessionID: "session-1"},
		Lifecycle: LifecycleFacts{Status: "SUCCEEDED"},
		Hashes:    HashFacts{SourceHash: "sha256:source"},
		Progress:  ProgressFacts{TotalDispatches: 2, CompletedDispatches: 2},
		Dispatches: []DispatchFact{
			{SessionID: "session-1", ID: "dispatch-2", Order: 1, Status: "SUCCEEDED", Kind: "WORK"},
			{SessionID: "session-1", ID: "dispatch-1", Order: 2, Status: "SUCCEEDED", Kind: "WORK"},
		},
		Artifacts:    []ArtifactFact{{SessionID: "session-1", ID: "artifact-1", Order: 1, Kind: "RESULT"}},
		Results:      []ResultFact{{SessionID: "session-1", ID: "result-1", Order: 1, Status: "FINAL", Value: "done"}},
		Failures:     []FailureFact{},
		EventCursors: []FactoryEventCursor{{SessionID: "session-1", Cursor: "cursor-11", Sequence: 11, EventType: "DISPATCH_COMPLETED"}},
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal stable session: %v", err)
	}
	return encoded
}
