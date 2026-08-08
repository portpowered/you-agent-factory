package workers

import (
	"errors"
	"testing"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

func TestContainsStopToken_CompleteMarkerMustBeFinalNonEmptyLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "final marker", output: "finished\n<COMPLETE>\n", want: true},
		{name: "continue wins", output: "completion uses <COMPLETE>\n<CONTINUE>"},
		{name: "inline mention", output: "finished with <COMPLETE> in prose"},
		{name: "trailing prose", output: "<COMPLETE>\nadditional caveat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ContainsStopToken(tc.output, "<COMPLETE>"); got != tc.want {
				t.Fatalf("ContainsStopToken() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestContainsStopToken_LegacyTokensRetainSubstringSemantics(t *testing.T) {
	t.Parallel()
	if !ContainsStopToken("Work done. COMPLETE", "COMPLETE") {
		t.Fatal("plain legacy stop token did not match inline output")
	}
	if !ContainsStopToken("prefix <result>ACCEPTED</result> suffix", "<result>ACCEPTED</result>") {
		t.Fatal("structured legacy stop token did not match inline output")
	}
}

func TestNormalizeProviderExecutionError_PreservesBoundedProviderSessionInspectionCause(t *testing.T) {
	err := &providersessions.LookupError{
		Provider:  providersessions.ProviderCodex,
		SessionID: "rollout-resource-limit",
		Err:       errors.New("rollout contents must not be copied into the worker error"),
	}
	err.Err = errors.Join(providersessions.ErrResourceLimitExceeded, err.Err)

	normalized := NormalizeProviderExecutionError(err)
	if normalized == nil {
		t.Fatal("NormalizeProviderExecutionError() = nil, want a typed provider error")
	}
	if normalized.Type != WorkFailureTypeUnknown || normalized.Family != WorkFailureFamilyTerminal {
		t.Fatalf("normalized = %#v, want terminal unknown classification", normalized)
	}
	if normalized.Message != "provider session inspection reached its configured limit" {
		t.Fatalf("normalized.Message = %q, want bounded resource-limit cause", normalized.Message)
	}
	if !errors.Is(normalized, providersessions.ErrResourceLimitExceeded) {
		t.Fatal("normalized error did not retain the typed inspection-limit cause")
	}
	if normalized.Diagnostics == nil || normalized.Diagnostics.Provider == nil {
		t.Fatalf("normalized.Diagnostics = %#v, want provider diagnostics", normalized.Diagnostics)
	}
	if normalized.ProviderSession == nil || normalized.ProviderSession.ID != "rollout-resource-limit" {
		t.Fatalf("normalized.ProviderSession = %#v, want stable provider session identity", normalized.ProviderSession)
	}
	metadata := normalized.Diagnostics.Provider.ResponseMetadata
	if metadata[ProviderResponseMetadataFailureOperation] != "provider_session_ingestion" ||
		metadata[ProviderResponseMetadataFailureClassification] != "resource_limit" ||
		metadata["provider_session_id"] != "rollout-resource-limit" {
		t.Fatalf("inspection diagnostics = %#v, want stable operation/classification", metadata)
	}
	if normalized.Error() == "" || normalized.Error() == err.Error() {
		t.Fatalf("normalized.Error() = %q, want safe bounded text", normalized.Error())
	}
}

func TestNormalizeProviderExecutionError_ClassifiesBareProviderSessionCancellation(t *testing.T) {
	normalized := NormalizeProviderExecutionError(providersessions.ErrOperationCanceled)
	if normalized == nil {
		t.Fatal("NormalizeProviderExecutionError() = nil, want a typed cancellation error")
	}
	if normalized.Type != WorkFailureTypeUnknown || normalized.Family != WorkFailureFamilyTerminal {
		t.Fatalf("normalized = %#v, want terminal unknown cancellation classification", normalized)
	}
	if normalized.Message != "provider session inspection was canceled" {
		t.Fatalf("normalized.Message = %q, want safe cancellation cause", normalized.Message)
	}
	if normalized.Diagnostics == nil || normalized.Diagnostics.Provider == nil ||
		normalized.Diagnostics.Provider.ResponseMetadata[ProviderResponseMetadataFailureClassification] != "canceled" {
		t.Fatalf("normalized.Diagnostics = %#v, want canceled inspection classification", normalized.Diagnostics)
	}
}
