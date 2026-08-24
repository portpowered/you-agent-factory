package cli

import (
	"errors"
	"net/http"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPullMappingDefaultAndDiagnosticBranches(t *testing.T) {
	t.Parallel()

	result := modelinference.PullResult{
		ModelName: "voice", Outcome: "PULLED", CachePath: "cache", Revision: "rev",
		PullDiagnostics: modelinference.PullDiagnostics{
			ModelName: "voice", ResolvedRepository: "owner/repo", Revision: "rev",
			File: "weights.gguf", Operation: "download", RequestURL: "https://assets.example.test/weights.gguf",
			UpstreamStatusCode: http.StatusBadGateway,
		},
		SourceKind: "UPSTREAM_REPOSITORY", SourceID: "owner/repo", ResolverNotes: "resolved",
	}
	diagnostics := managedRuntimePullDiagnostics(result)
	if diagnostics == nil || diagnostics.ModelName == nil || *diagnostics.ModelName != "voice" || diagnostics.RequestUrl == nil || diagnostics.UpstreamStatusCode == nil || *diagnostics.UpstreamStatusCode != http.StatusBadGateway {
		t.Fatalf("managed pull diagnostics = %#v, want all safe diagnostic fields", diagnostics)
	}
	if source := managedRuntimePullSourceDiagnostics(result); source == nil || source.SourceKind == nil || source.SourceId == nil || source.ResolverNotes == nil {
		t.Fatalf("source diagnostics = %#v, want source facts", source)
	}
	if managedRuntimePullDiagnostics(modelinference.PullResult{}) != nil || managedRuntimePullSourceDiagnostics(modelinference.PullResult{}) != nil {
		t.Fatal("empty pull diagnostics unexpectedly projected")
	}

	if got := managedRuntimePullOutcome(modelinference.PullResult{Outcome: "PULLED"}); got != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY {
		t.Fatalf("blank managed outcome = %q, want INSTALLED_SUCCESSFULLY", got)
	}
	if got := managedRuntimePullOutcome(modelinference.PullResult{}); got != factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME {
		t.Fatalf("empty managed outcome = %q, want UNSUPPORTED_RUNTIME", got)
	}
	if got := managedRuntimePullReadiness(modelinference.PullResult{}); got != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("blank readiness = %q, want READY", got)
	}
	if got := managedRuntimeLifecycleFromPull(factoryapi.ModelPullResponse{ManagedRuntimePull: factoryapi.ManagedRuntimePullResult{PullOutcome: factoryapi.ManagedRuntimePullOutcomeSTILLLOADING}}); got != string(factoryapi.ManagedRuntimeLifecycleStateINSTALLING) {
		t.Fatalf("loading lifecycle = %q, want INSTALLING", got)
	}
	if got := managedRuntimeLifecycleFromPull(factoryapi.ModelPullResponse{}); got != "UNKNOWN" {
		t.Fatalf("unknown lifecycle = %q, want UNKNOWN", got)
	}

	for _, testCase := range []struct {
		name       string
		statusCode int
		body       string
		wantError  bool
	}{
		{name: "non classified status", statusCode: http.StatusOK, body: `{}`, wantError: false},
		{name: "invalid body", statusCode: http.StatusUnprocessableEntity, body: `{`, wantError: false},
		{name: "successful outcome", statusCode: http.StatusGatewayTimeout, body: `{"managedRuntimePull":{"pullOutcome":"ALREADY_READY"}}`, wantError: false},
		{name: "gateway failure", statusCode: http.StatusGatewayTimeout, body: `{"managedRuntimePull":{"pullOutcome":"TIMED_OUT","readinessState":"FAILED"}}`, wantError: true},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := managedRuntimePullResponseError(testCase.statusCode, []byte(testCase.body))
			if (err != nil) != testCase.wantError {
				t.Fatalf("managedRuntimePullResponseError() = %v, want error=%v", err, testCase.wantError)
			}
		})
	}

	var nilFailure *managedRuntimePullFailure
	if nilFailure.Error() != "" || nilFailure.Unwrap() != nil {
		t.Fatalf("nil pull failure methods = (%q, %v), want empty/nil", nilFailure.Error(), nilFailure.Unwrap())
	}
	failure := &managedRuntimePullFailure{Outcome: factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED, Readiness: factoryapi.ManagedRuntimeReadinessStateFAILED, Diagnostics: errors.New("diagnostic")}
	if failure.Error() == "" || !errors.Is(failure, failure.Diagnostics) || failure.CLIErrorCode() != managedRuntimePullFailureCode {
		t.Fatalf("pull failure = %v, want coded diagnostic failure", failure)
	}
}
