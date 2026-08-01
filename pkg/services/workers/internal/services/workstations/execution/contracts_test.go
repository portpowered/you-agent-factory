package workerexecution

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
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
	for _, tc := range []struct {
		input string
		want  string
	}{
		{input: "", want: ""},
		{input: " cursor ", want: "cursor"},
		{input: "agent", want: "cursor"},
		{input: "cursor-agent", want: "cursor"},
		{input: "cursor-cli", want: "cursor"},
		{input: "acme", want: "acme"},
	} {
		if got := CanonicalProviderSessionProvider(tc.input); got != tc.want {
			t.Fatalf("CanonicalProviderSessionProvider(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCloneProviderSessionMetadataDetachesAndAcceptsNil(t *testing.T) {
	t.Parallel()

	original := &ProviderSessionMetadata{Provider: "cursor", Kind: "session_id", ID: "session-1"}
	clone := CloneProviderSessionMetadata(original)
	if clone == nil || clone == original {
		t.Fatalf("CloneProviderSessionMetadata() = %#v, want a detached non-nil value", clone)
	}
	clone.Provider = "mutated"
	clone.ID = "mutated"
	if original.Provider != "cursor" || original.ID != "session-1" {
		t.Fatalf("clone mutation changed original metadata: original = %#v", original)
	}
	if got := CloneProviderSessionMetadata(nil); got != nil {
		t.Fatalf("CloneProviderSessionMetadata(nil) = %#v, want nil", got)
	}
}

func TestProviderSessionMetadataFromGenerated_CanonicalizesLegacyCursorProvider(t *testing.T) {
	t.Parallel()

	metadata := ProviderSessionMetadata{Provider: "agent", Kind: "session_id", ID: "cursor-session-123"}
	metadata.Provider = CanonicalProviderSessionProvider(metadata.Provider)
	if metadata.Provider != "cursor" || metadata.Kind != "session_id" || metadata.ID != "cursor-session-123" {
		t.Fatalf("metadata = %#v, want canonical cursor session metadata", metadata)
	}
}
