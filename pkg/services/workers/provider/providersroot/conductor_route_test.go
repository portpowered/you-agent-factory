package providersroot

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestExecuteFailureKindFromConductor_PreservesRetryableUnknownAsDependency(t *testing.T) {
	failure := inference.NewFailure(inference.FailureInput{
		Kind:      inference.FailureUnknown,
		Message:   "temporary service failure",
		Retryable: true,
	})
	if got := executeFailureKindFromConductor(failure); got != providers.ExecuteFailureKindDependency {
		t.Fatalf("kind = %q, want dependency for retryable unknown conductor failure", got)
	}
}

func TestExecuteFailureKindFromConductor_TerminalUnknownStaysUnknown(t *testing.T) {
	failure := inference.NewFailure(inference.FailureInput{
		Kind:    inference.FailureUnknown,
		Message: "provider exited unexpectedly",
	})
	if got := executeFailureKindFromConductor(failure); got != providers.ExecuteFailureKindUnknown {
		t.Fatalf("kind = %q, want unknown for non-retryable conductor failure", got)
	}
}

func TestInvocationRequestFromExecute_ForwardsEnvAndSkipPermissions(t *testing.T) {
	request := providers.ExecuteRequest{
		Provider:           providers.IDCursor,
		AttemptID:          "dispatch-env-1",
		WorkerType:         "cursor-worker",
		WorkstationName:    "review",
		Model:              "cursor-auto",
		SystemPrompt:       "system",
		UserMessage:        "user",
		EnvVars:            map[string]string{"CURSOR_FUNCTIONAL_CONTEXT": "configured"},
		ProcessEnvironment: []string{"CURSOR_FUNCTIONAL_CONTEXT=configured"},
	}

	invocation := invocationRequestFromExecute(request, true)
	execution := invocation.Execution()
	if execution.EnvVars["CURSOR_FUNCTIONAL_CONTEXT"] != "configured" {
		t.Fatalf("EnvVars = %#v, want configured workstation env", execution.EnvVars)
	}
	if len(execution.ProcessEnvironment) != 1 ||
		execution.ProcessEnvironment[0] != "CURSOR_FUNCTIONAL_CONTEXT=configured" {
		t.Fatalf("ProcessEnvironment = %#v, want forwarded process env", execution.ProcessEnvironment)
	}
	if !execution.SkipPermissions {
		t.Fatal("SkipPermissions = false, want true")
	}
}
