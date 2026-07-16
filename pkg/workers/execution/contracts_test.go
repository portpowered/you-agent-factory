package workerexecution

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/work"
)

func TestExecutionContractsPreserveJSONAndCloneIsolation(t *testing.T) {
	t.Parallel()
	original := ProviderInferenceRequest{
		Dispatch:                     work.WorkDispatch{DispatchID: "dispatch-1"},
		RequiredOptionalCapabilities: []RunnerOptionalCapability{RunnerOptionalCapabilityStructuredOutput},
		EnvVars:                      map[string]string{"SAFE": "value"},
	}
	clone := CloneProviderInferenceRequest(original)
	clone.RequiredOptionalCapabilities[0] = RunnerOptionalCapabilityWorktree
	clone.EnvVars["SAFE"] = "changed"
	if original.RequiredOptionalCapabilities[0] != RunnerOptionalCapabilityStructuredOutput || original.EnvVars["SAFE"] != "value" {
		t.Fatal("clone mutated the original request")
	}
	encoded, err := json.Marshal(WorkResult{DispatchID: "dispatch-1", Outcome: OutcomeFailed, FailureMetadata: &WorkFailureMetadata{Family: WorkFailureFamilyTerminal, Type: WorkFailureTypeAuthFailure}})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"dispatch_id":"dispatch-1","transition_id":"","outcome":"FAILED","failure_metadata":{"family":"terminal","type":"auth_failure"},"metrics":{"duration":0,"cost":0,"retry_count":0}}`
	if string(encoded) != want {
		t.Fatalf("encoded result = %s, want %s", encoded, want)
	}
}

func TestCanonicalProviderSessionProviderPreservesAliasesAndUnknowns(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{"agent": "cursor", "cursor-agent": "cursor", "cursor": "cursor", "acme": "acme"} {
		if got := CanonicalProviderSessionProvider(input); got != want {
			t.Fatalf("CanonicalProviderSessionProvider(%q) = %q, want %q", input, got, want)
		}
	}
}
