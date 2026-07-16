package sessionparity

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestTerminalFixtureObservations_NormalizeAcrossCustomerInterfaces(t *testing.T) {
	for _, test := range []struct {
		name         string
		observations CapturedObservations
		status       string
		results      int
		failures     int
	}{
		{name: "success", observations: TerminalSuccessObservations(), status: "SUCCEEDED", results: 1, failures: 0},
		{name: "failure", observations: TerminalFailureObservations(), status: "FAILED", results: 0, failures: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeFixtureObservations(t, test.observations)
			if got.Lifecycle.Status != test.status {
				t.Fatalf("status = %q, want %q", got.Lifecycle.Status, test.status)
			}
			if len(got.Results) != test.results || len(got.Failures) != test.failures {
				t.Fatalf("results/failures = %d/%d, want %d/%d", len(got.Results), len(got.Failures), test.results, test.failures)
			}
			if len(got.Dispatches) == 0 || len(got.Artifacts) == 0 || len(got.EventCursors) == 0 {
				t.Fatal("fixture omitted required dispatch, artifact, or canonical event cursor")
			}
		})
	}
}

func TestTerminalFixtureObservations_AreDeterministic(t *testing.T) {
	for _, fixture := range []func() CapturedObservations{TerminalSuccessObservations, TerminalFailureObservations} {
		first := normalizedFixtureJSON(t, fixture())
		second := normalizedFixtureJSON(t, fixture())
		if !bytes.Equal(first, second) {
			t.Fatalf("serialized projection changed between identical fixture observations\nfirst:  %s\nsecond: %s", first, second)
		}
	}
}

func normalizeFixtureObservations(t *testing.T, observations CapturedObservations) Projection {
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
	if !reflect.DeepEqual(cli, rest) || !reflect.DeepEqual(mcp, rest) {
		t.Fatalf("customer-interface projections differ\nREST: %#v\nCLI: %#v\nMCP: %#v", rest, cli, mcp)
	}
	return rest
}

func normalizedFixtureJSON(t *testing.T, observations CapturedObservations) []byte {
	t.Helper()
	encoded, err := json.Marshal(normalizeFixtureObservations(t, observations))
	if err != nil {
		t.Fatalf("marshal normalized fixture: %v", err)
	}
	return encoded
}
