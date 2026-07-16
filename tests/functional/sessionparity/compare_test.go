package sessionparity

import (
	"bytes"
	"encoding/json"
	"strings"
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

func TestCompare_ReportsAdjacentLargeEventSequences(t *testing.T) {
	expected := normalizedFixtureProjection(t, TerminalSuccessObservations())
	actual := normalizedFixtureProjection(t, TerminalSuccessObservations())
	expected.EventCursors[0].Sequence = 9007199254740992
	actual.EventCursors[0].Sequence = 9007199254740993

	differences := Compare(expected, actual)
	want := Difference{
		Path: "eventCursors[0].sequence", Expected: "9007199254740992", Actual: "9007199254740993",
	}
	if len(differences) != 1 || differences[0] != want {
		t.Fatalf("Compare() = %#v, want %#v", differences, []Difference{want})
	}
}

func TestNormalizers_ExcludeTransportDetailsAndRetainSessionFacts(t *testing.T) {
	observations := TerminalFailureObservations()
	for _, test := range []struct {
		name      string
		input     []byte
		normalize func([]byte) (Projection, error)
		metadata  func(*testing.T, []byte) []byte
		fact      func(*testing.T, []byte) []byte
		factPath  string
	}{
		{"REST", observations.REST, NormalizeREST, changeRESTTransportFraming, changeRESTEventType, "eventCursors[0].eventType"},
		{"CLI JSON", observations.CLIJSON, NormalizeCLIJSON, addCLITransportMetadata, changeDirectSessionStatus, "lifecycle.status"},
		{"MCP", observations.MCP, NormalizeMCP, changeMCPTransportMetadata, changeMCPSessionStatus, "lifecycle.status"},
	} {
		t.Run(test.name, func(t *testing.T) {
			expected := normalizeObservation(t, test.normalize, test.input)
			transportOnly := normalizeObservation(t, test.normalize, test.metadata(t, test.input))
			if differences := Compare(expected, transportOnly); len(differences) != 0 {
				t.Fatalf("transport-only mutation produced differences: %#v", differences)
			}

			retainedFact := normalizeObservation(t, test.normalize, test.fact(t, test.input))
			differences := Compare(expected, retainedFact)
			if !containsDifference(differences, test.factPath) {
				t.Fatalf("retained fact mutation was not reported at %s: %#v", test.factPath, differences)
			}
			first := mustMarshal(t, differences)
			second := mustMarshal(t, Compare(expected, retainedFact))
			if !bytes.Equal(first, second) {
				t.Fatalf("failure report changed across identical comparisons\nfirst: %s\nsecond: %s", first, second)
			}
		})
	}
}

func normalizeObservation(t *testing.T, normalize func([]byte) (Projection, error), observation []byte) Projection {
	t.Helper()
	projection, err := normalize(observation)
	if err != nil {
		t.Fatalf("normalize observation: %v", err)
	}
	return projection
}

func changeRESTTransportFraming(t *testing.T, observation []byte) []byte {
	t.Helper()
	bundle := observationObject(t, observation)
	bundle["http"] = map[string]any{
		"status": 200, "headers": map[string]any{"x-request-id": "rest-request-2"},
		"request": map[string]any{"method": "GET", "path": "/factory-sessions/example"},
	}
	body := bundle["events"].(string)
	bundle["events"] = strings.ReplaceAll(body, "data: ",
		": ignored keepalive\nid: ignored-frame\nevent: factory-event\nretry: 1500\ndata: ")
	return mustMarshal(t, bundle)
}

func addCLITransportMetadata(t *testing.T, observation []byte) []byte {
	t.Helper()
	bundle := observationObject(t, observation)
	bundle["presentation"] = map[string]any{"color": true, "format": "table"}
	bundle["diagnostics"] = map[string]any{"command": "you session show", "elapsedMs": 12}
	return mustMarshal(t, bundle)
}

func changeMCPTransportMetadata(t *testing.T, observation []byte) []byte {
	t.Helper()
	bundle := observationObject(t, observation)
	for _, field := range []string{"session", "dispatches", "artifacts", "result", "events"} {
		response := bundle[field].(map[string]any)
		response["id"] = "replacement-" + field
	}
	return mustMarshal(t, bundle)
}

func changeRESTEventType(t *testing.T, observation []byte) []byte {
	t.Helper()
	bundle := observationObject(t, observation)
	body := bundle["events"].(string)
	changed := strings.Replace(body, `"type":"FACTORY_SESSION_STARTED"`, `"type":"FACTORY_SESSION_RESTARTED"`, 1)
	if changed == body {
		t.Fatal("REST fixture did not contain the retained event type")
	}
	bundle["events"] = changed
	return mustMarshal(t, bundle)
}

func changeDirectSessionStatus(t *testing.T, observation []byte) []byte {
	t.Helper()
	bundle := observationObject(t, observation)
	bundle["session"].(map[string]any)["status"] = "SUCCEEDED"
	return mustMarshal(t, bundle)
}

func changeMCPSessionStatus(t *testing.T, observation []byte) []byte {
	t.Helper()
	bundle := observationObject(t, observation)
	mutateMCPToolResult(t, bundle, "session", func(result map[string]any) {
		result["status"] = "SUCCEEDED"
	})
	return mustMarshal(t, bundle)
}

func observationObject(t *testing.T, observation []byte) map[string]any {
	t.Helper()
	var bundle map[string]any
	if err := json.Unmarshal(observation, &bundle); err != nil {
		t.Fatalf("unmarshal observation: %v", err)
	}
	return bundle
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
