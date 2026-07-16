package sessionparity

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCompare_ReportsEveryRetainedFactMismatch(t *testing.T) {
	expected := normalizedFixtureProjection(t, TerminalSuccessObservations())
	actual := normalizedFixtureProjection(t, TerminalSuccessObservations())
	actual.Identity.SessionID = "session-other"
	actual.Lifecycle.Status = "FAILED"
	actual.Hashes.SourceHash = "sha256:other"
	actual.Progress.CompletedDispatches = 1
	actual.Dispatches[0].Status = "FAILED"
	actual.Artifacts[0].Kind = "DIAGNOSTIC"
	actual.Results[0].Value = "other"
	actual.Failures = []FailureFact{{SessionID: "session-other", ID: "failure-1", Order: 1, Code: "FAILED", Message: "other"}}
	actual.EventCursors[0].Cursor = "other-cursor"

	differences := Compare(expected, actual)
	for _, path := range []string{
		"identity.sessionId",
		"lifecycle.status",
		"hashes.sourceHash",
		"progress.completedDispatches",
		"dispatches[0].status",
		"artifacts[0].kind",
		"results[0].value",
		"failures[0]",
		"eventCursors[0].cursor",
	} {
		if !containsDifference(differences, path) {
			t.Errorf("differences did not report %s: %#v", path, differences)
		}
	}
}

func TestCompare_ReportsMissingUnexpectedDuplicateAndReorderedFacts(t *testing.T) {
	expected := normalizedFixtureProjection(t, TerminalSuccessObservations())
	actual := normalizedFixtureProjection(t, TerminalSuccessObservations())
	actual.Dispatches = append([]DispatchFact{expected.Dispatches[1]}, expected.Dispatches[0])
	actual.Artifacts = []ArtifactFact{}
	actual.Results = append(actual.Results, expected.Results[0])
	actual.EventCursors[0], actual.EventCursors[1] = actual.EventCursors[1], actual.EventCursors[0]

	differences := Compare(expected, actual)
	for _, path := range []string{
		"dispatches[0].id",
		"artifacts[0]",
		"results[1]",
		"eventCursors[0].cursor",
	} {
		if !containsDifference(differences, path) {
			t.Errorf("differences did not report %s: %#v", path, differences)
		}
	}
}

func TestCompare_IsDeterministicAndIgnoresTransportOnlyObservationFields(t *testing.T) {
	observations := TerminalFailureObservations()
	first := normalizedFixtureProjection(t, observations)
	var captured map[string]any
	if err := json.Unmarshal(observations.REST, &captured); err != nil {
		t.Fatalf("unmarshal REST capture: %v", err)
	}
	captured["http"] = map[string]any{"requestId": "rest-request-2", "headers": map[string]any{"x-request-id": "rest-request-2"}, "status": 200}
	transportOnlyREST, err := json.Marshal(captured)
	if err != nil {
		t.Fatalf("marshal REST capture: %v", err)
	}
	second := normalizedFixtureProjection(t, CapturedObservations{REST: transportOnlyREST})
	if differences := Compare(first, second); len(differences) != 0 {
		t.Fatalf("equivalent projections have differences: %#v", differences)
	}
	if !reflect.DeepEqual(Compare(first, second), Compare(first, second)) {
		t.Fatal("comparison report changed across identical calls")
	}
}

func normalizedFixtureProjection(t *testing.T, observations CapturedObservations) Projection {
	t.Helper()
	projection, err := NormalizeREST(observations.REST)
	if err != nil {
		t.Fatalf("NormalizeREST: %v", err)
	}
	return projection
}

func containsDifference(differences []Difference, path string) bool {
	for _, difference := range differences {
		if difference.Path == path {
			return true
		}
	}
	return false
}
