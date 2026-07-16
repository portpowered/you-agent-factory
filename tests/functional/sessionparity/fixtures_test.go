package sessionparity

import (
	"bytes"
	"encoding/json"
	"testing"
)

type namedFixtureProjection struct {
	interfaceName string
	projection    Projection
}

func TestTerminalFixtureObservations_NormalizeAcrossCustomerInterfaces(t *testing.T) {
	for _, test := range []struct {
		name         string
		observations CapturedObservations
		want         Projection
	}{
		{name: "success", observations: TerminalSuccessObservations(), want: terminalSuccessProjection()},
		{name: "failure", observations: TerminalFailureObservations(), want: terminalFailureProjection()},
	} {
		t.Run(test.name, func(t *testing.T) {
			projections := normalizeFixtureObservations(t, test.observations)
			for _, got := range projections {
				t.Run(got.interfaceName, func(t *testing.T) {
					if differences := Compare(test.want, got.projection); len(differences) != 0 {
						t.Fatalf("fixture projection differs from declared stable facts: %#v", differences)
					}
				})
			}
		})
	}
}

func TestTerminalFixtureObservations_AreDeterministic(t *testing.T) {
	for _, test := range []struct {
		name    string
		fixture func() CapturedObservations
	}{
		{name: "success", fixture: TerminalSuccessObservations},
		{name: "failure", fixture: TerminalFailureObservations},
	} {
		t.Run(test.name, func(t *testing.T) {
			first := normalizeFixtureObservations(t, test.fixture())
			second := normalizeFixtureObservations(t, test.fixture())
			for index := range first {
				t.Run(first[index].interfaceName, func(t *testing.T) {
					if differences := Compare(first[index].projection, second[index].projection); len(differences) != 0 {
						t.Fatalf("unchanged fixture produced semantic differences: %#v", differences)
					}
					firstJSON := projectionJSON(t, first[index].projection)
					secondJSON := projectionJSON(t, second[index].projection)
					if !bytes.Equal(firstJSON, secondJSON) {
						t.Fatalf("serialized projection changed between identical fixture observations\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
					}
				})
			}
		})
	}
}

func terminalSuccessProjection() Projection {
	sessionID := "dur-sess-terminal-success"
	return Projection{
		Identity:  FactorySessionIdentity{SessionID: sessionID},
		Lifecycle: LifecycleFacts{Status: "SUCCEEDED", Phase: fixtureString("completed")},
		Hashes: HashFacts{
			SourceHash: "sha256:success-source", RequestedPolicyHash: fixtureString("sha256:success-requested-policy"),
			EffectivePolicyHash: fixtureString("sha256:success-effective-policy"),
		},
		Progress: ProgressFacts{TotalDispatches: 2, CompletedDispatches: 2},
		Dispatches: []DispatchFact{
			{SessionID: sessionID, ID: "dispatch-success-1", Order: 1, Status: "COMPLETED", Kind: "JAVASCRIPT_TASK"},
			{SessionID: sessionID, ID: "dispatch-success-2", Order: 2, Status: "COMPLETED", Kind: "JAVASCRIPT_TASK"},
		},
		Artifacts: []ArtifactFact{{SessionID: sessionID, ID: "artifact-success-result", Order: 1, Kind: "FINAL_RESULT"}},
		Results: []ResultFact{{
			SessionID: sessionID, ID: sessionID + ":result", Order: 1, Status: "FINAL",
			Value: `[{"text":"fixture success","type":"text"}]`,
		}},
		Failures: []FailureFact{},
		EventCursors: []FactoryEventCursor{
			{SessionID: sessionID, Cursor: "success-cursor-100", Sequence: 100, EventType: "FACTORY_SESSION_STARTED"},
			{SessionID: sessionID, Cursor: "success-cursor-101", Sequence: 101, EventType: "DISPATCH_RESPONSE"},
			{SessionID: sessionID, Cursor: "success-cursor-102", Sequence: 102, EventType: "SESSION_RESULT_UPDATED"},
		},
	}
}

func terminalFailureProjection() Projection {
	sessionID := "dur-sess-terminal-failure"
	dispatchID := "dispatch-failure-2"
	return Projection{
		Identity:  FactorySessionIdentity{SessionID: sessionID},
		Lifecycle: LifecycleFacts{Status: "FAILED", Phase: fixtureString("failed")},
		Hashes: HashFacts{
			SourceHash: "sha256:failure-source", RequestedPolicyHash: fixtureString("sha256:failure-requested-policy"),
			EffectivePolicyHash: fixtureString("sha256:failure-effective-policy"),
		},
		Progress: ProgressFacts{TotalDispatches: 2, CompletedDispatches: 1, FailedDispatches: 1},
		Dispatches: []DispatchFact{
			{SessionID: sessionID, ID: "dispatch-failure-1", Order: 1, Status: "COMPLETED", Kind: "JAVASCRIPT_TASK"},
			{SessionID: sessionID, ID: dispatchID, Order: 2, Status: "FAILED", Kind: "JAVASCRIPT_TASK"},
		},
		Artifacts: []ArtifactFact{{SessionID: sessionID, ID: "artifact-failure-diagnostic", Order: 1, Kind: "DIAGNOSTIC"}},
		Results:   []ResultFact{},
		Failures: []FailureFact{
			{SessionID: sessionID, ID: sessionID + ":failure", Order: 1, Code: "WORKER_FAILED", Message: "fixture worker failed"},
			{SessionID: sessionID, ID: dispatchID + ":failure", Order: 2, Code: "WORKER_FAILED", Message: "fixture worker failed", DispatchID: &dispatchID},
			{SessionID: sessionID, ID: sessionID + ":result-failure", Order: 3, Code: "WORKER_FAILED", Message: "fixture worker failed"},
		},
		EventCursors: []FactoryEventCursor{
			{SessionID: sessionID, Cursor: "failure-cursor-200", Sequence: 200, EventType: "FACTORY_SESSION_STARTED"},
			{SessionID: sessionID, Cursor: "failure-cursor-201", Sequence: 201, EventType: "DISPATCH_RESPONSE"},
			{SessionID: sessionID, Cursor: "failure-cursor-202", Sequence: 202, EventType: "SESSION_RESULT_UPDATED"},
		},
	}
}

func fixtureString(value string) *string {
	return &value
}

func normalizeFixtureObservations(t *testing.T, observations CapturedObservations) []namedFixtureProjection {
	t.Helper()
	rest, err := NormalizeREST(observations.REST)
	if err != nil {
		t.Fatalf("NormalizeREST: %v", err)
	}
	cli, err := NormalizeCLIJSON(observations.CLIJSON)
	if err != nil {
		t.Fatalf("NormalizeCLIJSON: %v", err)
	}
	mcp, err := NormalizeMCP(observations.MCP)
	if err != nil {
		t.Fatalf("NormalizeMCP: %v", err)
	}
	projections := []namedFixtureProjection{
		{interfaceName: "REST", projection: rest},
		{interfaceName: "CLI JSON", projection: cli},
		{interfaceName: "MCP", projection: mcp},
	}
	for _, projection := range projections[1:] {
		if differences := Compare(rest, projection.projection); len(differences) != 0 {
			t.Fatalf("%s fixture differs from REST fixture: %#v", projection.interfaceName, differences)
		}
	}
	return projections
}

func projectionJSON(t *testing.T, projection Projection) []byte {
	t.Helper()
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("marshal normalized fixture: %v", err)
	}
	return encoded
}
