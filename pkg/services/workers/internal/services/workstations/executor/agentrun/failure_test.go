package agentrun

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	managedruntime "github.com/portpowered/infinite-you/pkg/services/models"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestFailureClassForError_ModelhostLeaseDenied(t *testing.T) {
	t.Parallel()

	err := &modelhost.HostReadinessError{
		Snapshot: modelhost.HostReadinessSnapshot{
			Identity:       modelhost.HostIdentity{Name: "OMNIVOICE_Q4_K_M"},
			ReadinessState: managedruntime.ReadinessStateFailed,
			FailureClass:   modelhost.HostFailureClassCapacityExhausted,
		},
		Cause: modelhost.ErrHostCapacityExhausted,
	}
	if got := failureClassForError(err); got != FailureClassLeaseDenied {
		t.Fatalf("failureClassForError = %q, want %q", got, FailureClassLeaseDenied)
	}
}

func TestFailureClassForError_ModelhostMissingReadiness(t *testing.T) {
	t.Parallel()

	err := &modelhost.HostReadinessError{
		Snapshot: modelhost.HostReadinessSnapshot{
			Identity:       modelhost.HostIdentity{Name: "OMNIVOICE_Q4_K_M"},
			ReadinessState: managedruntime.ReadinessStateMissing,
			FailureClass:   modelhost.HostFailureClassMissingAssets,
		},
		Cause: modelhost.ErrHostRuntimeNotReady,
	}
	if got := failureClassForError(err); got != FailureClassModelNotReady {
		t.Fatalf("failureClassForError = %q, want %q", got, FailureClassModelNotReady)
	}
}

func TestFailureClassForError_ModelhostRuntimeFailure(t *testing.T) {
	t.Parallel()

	err := &modelhost.HostReadinessError{
		Snapshot: modelhost.HostReadinessSnapshot{
			Identity:       modelhost.HostIdentity{Name: "OMNIVOICE_Q4_K_M"},
			ReadinessState: managedruntime.ReadinessStateFailed,
			FailureClass:   modelhost.HostFailureClassProcessCrash,
		},
		Cause: modelhost.ErrHostProcessCrash,
	}
	if got := failureClassForError(err); got != FailureClassModelRuntime {
		t.Fatalf("failureClassForError = %q, want %q", got, FailureClassModelRuntime)
	}
}

func TestFailureClassForError_ManagedRuntimeInvocationMissing(t *testing.T) {
	t.Parallel()

	err := &managedruntime.InvocationError{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: managedruntime.ReadinessStateMissing,
		Cause:          managedruntime.ErrMissing,
	}
	if got := failureClassForError(err); got != FailureClassModelNotReady {
		t.Fatalf("failureClassForError = %q, want %q", got, FailureClassModelNotReady)
	}
}

func TestFailureClassForError_ManagedRuntimeInvocationWithoutCauseUsesReadiness(t *testing.T) {
	t.Parallel()

	err := &managedruntime.InvocationError{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: managedruntime.ReadinessStateFailed,
	}
	if got := failureClassForError(err); got != FailureClassModelRuntime {
		t.Fatalf("failureClassForError = %q, want %q", got, FailureClassModelRuntime)
	}
	if got := recoveryActionForError(err); got == "" {
		t.Fatal("expected recovery action for failed managed runtime")
	}
}

func TestAgentRunFailureDiagnostics_IncludesRecoveryAction(t *testing.T) {
	t.Parallel()

	diagnostics := agentRunFailureDiagnostics(modelhost.ErrHostCapacityExhausted)
	if diagnostics[DiagnosticFailureClass] != FailureClassLeaseDenied {
		t.Fatalf("failure class = %q, want %q", diagnostics[DiagnosticFailureClass], FailureClassLeaseDenied)
	}
	if diagnostics[DiagnosticRecoveryAction] == "" {
		t.Fatal("expected recovery action for lease denial")
	}
}

func TestFailureMetadataForError_ModelhostLeaseDeniedIsThrottle(t *testing.T) {
	t.Parallel()

	metadata := failureMetadataForError(modelhost.ErrHostCapacityExhausted)
	if metadata == nil {
		t.Fatal("expected failure metadata")
	}
	if metadata.Family != workerexecution.WorkFailureFamilyThrottle || metadata.Type != workerexecution.WorkFailureTypeThrottled {
		t.Fatalf("metadata = %#v, want throttle family", metadata)
	}
}

func TestFailureMetadataForError_PreservesRetryableProviderFailure(t *testing.T) {
	t.Parallel()

	err := workerexecution.NewProviderError(
		workerexecution.WorkFailureTypeInternalServerError,
		"temporary provider failure",
		errors.New("temporary provider failure"),
	)
	metadata := failureMetadataForError(err)
	if metadata == nil || metadata.Family != workerexecution.WorkFailureFamilyRetryable ||
		metadata.Type != workerexecution.WorkFailureTypeInternalServerError {
		t.Fatalf("metadata = %#v, want retryable internal server error", metadata)
	}
}

func TestAgentRunProviderFailureRecoveryUsesCanonicalFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		failureType   workerexecution.WorkFailureType
		wantFamily    workerexecution.WorkFailureFamily
		wantRetryable bool
		wantRecovery  string
	}{
		{
			name:          "dependency or overload normalized as internal server error",
			failureType:   workerexecution.WorkFailureTypeInternalServerError,
			wantFamily:    workerexecution.WorkFailureFamilyRetryable,
			wantRetryable: true,
			wantRecovery:  "retry the agent run after the provider recovers",
		},
		{
			name:          "provider timeout",
			failureType:   workerexecution.WorkFailureTypeTimeout,
			wantFamily:    workerexecution.WorkFailureFamilyRetryable,
			wantRetryable: true,
			wantRecovery:  "retry the agent run after the provider recovers",
		},
		{
			name:          "rate limited provider",
			failureType:   workerexecution.WorkFailureTypeThrottled,
			wantFamily:    workerexecution.WorkFailureFamilyThrottle,
			wantRetryable: true,
			wantRecovery:  "retry after provider capacity or rate limiting recovers",
		},
		{
			name:        "authentication failure",
			failureType: workerexecution.WorkFailureTypeAuthFailure,
			wantFamily:  workerexecution.WorkFailureFamilyTerminal,
		},
		{
			name:        "permanent invalid request",
			failureType: workerexecution.WorkFailureTypePermanentBadRequest,
			wantFamily:  workerexecution.WorkFailureFamilyTerminal,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := workerexecution.NewProviderError(
				test.failureType,
				"normalized provider failure",
				errors.New("provider payload must not become recovery guidance"),
			)
			metadata := failureMetadataForError(err)
			if metadata == nil || metadata.Family != test.wantFamily || metadata.Type != test.failureType {
				t.Fatalf("failure metadata = %#v, want family %q and type %q", metadata, test.wantFamily, test.failureType)
			}
			decision := workerexecution.FailureDecisionFromMetadata(metadata)
			if decision.Retryable != test.wantRetryable || decision.Terminal != !test.wantRetryable {
				t.Fatalf("failure decision = %#v, want retryable=%t and terminal=%t", decision, test.wantRetryable, !test.wantRetryable)
			}

			diagnostics := agentRunFailureDiagnostics(err)
			if got := diagnostics[DiagnosticRecoveryAction]; got != test.wantRecovery {
				t.Fatalf("recovery action = %q, want %q", got, test.wantRecovery)
			}
			if test.wantRecovery == "" && strings.Contains(diagnostics[DiagnosticRecoveryAction], "retry") {
				t.Fatalf("terminal provider failure received retry guidance: %q", diagnostics[DiagnosticRecoveryAction])
			}
		})
	}
}

func TestAgentRunFailureDiagnostics_ProviderFailurePreservesSafeType(t *testing.T) {
	t.Parallel()

	rawProviderPayload := "provider response contained a secret payload"
	err := workerexecution.NewProviderError(
		workerexecution.WorkFailureTypeInternalServerError,
		"temporary provider failure",
		errors.New(rawProviderPayload),
	)
	diagnostics := agentRunFailureDiagnostics(err)
	if diagnostics[DiagnosticFailureClass] != FailureClassProvider {
		t.Fatalf("failure class = %q, want %q", diagnostics[DiagnosticFailureClass], FailureClassProvider)
	}
	if diagnostics[DiagnosticProviderFailureType] != string(workerexecution.WorkFailureTypeInternalServerError) {
		t.Fatalf("provider failure type = %q, want %q", diagnostics[DiagnosticProviderFailureType], workerexecution.WorkFailureTypeInternalServerError)
	}
	formatted := formatAgentRunError(err)
	if !strings.HasPrefix(formatted, "agent run provider failure:") {
		t.Fatalf("formatAgentRunError() = %q, want provider prefix", formatted)
	}
	if !strings.Contains(formatted, string(workerexecution.WorkFailureTypeInternalServerError)) {
		t.Fatalf("formatAgentRunError() = %q, want normalized provider type", formatted)
	}
	if strings.Contains(formatted, rawProviderPayload) {
		t.Fatalf("formatAgentRunError() leaked raw provider payload: %q", formatted)
	}
}

func TestFailureClassForError_RawDeadlineRemainsAgentRunTimeout(t *testing.T) {
	t.Parallel()

	if got := failureClassForError(context.DeadlineExceeded); got != FailureClassTimeout {
		t.Fatalf("failureClassForError() = %q, want %q", got, FailureClassTimeout)
	}
}

func TestAgentRunFailureWorkResult_PreservesProviderContinuationClassification(t *testing.T) {
	t.Parallel()

	providerErr := workerexecution.NewProviderError(
		workerexecution.WorkFailureTypePermanentBadRequest,
		"provider session continuation is unsupported",
		nil,
	)
	providerErr.ProviderContinuationOutcome = providers.ContinuationOutcomeUnsupported
	result := agentRunFailureWorkResult(
		work.WorkDispatch{DispatchID: "dispatch-agent-run-continuation", TransitionID: "transition-1"},
		providerErr,
		time.Second,
		"",
		nil,
	)
	if result.Outcome != workerexecution.OutcomeFailed ||
		result.ProviderContinuationOutcome != providers.ContinuationOutcomeUnsupported ||
		result.ProviderFailureKind != "" || result.ProviderContinuationFailureKind != "" {
		t.Fatalf("agentRunFailureWorkResult() = %#v, want unsupported continuation classification", result)
	}
}

func TestModelhostOperationalFailureClass_MissingAssets(t *testing.T) {
	t.Parallel()

	if got := modelhostOperationalFailureClass(modelhost.ErrHostMissingAssets); got != FailureClassModelNotReady {
		t.Fatalf("modelhostOperationalFailureClass = %q, want %q", got, FailureClassModelNotReady)
	}
	if got := modelhostOperationalFailureClass(modelhost.ErrHostProcessCrash); got != FailureClassModelRuntime {
		t.Fatalf("modelhostOperationalFailureClass = %q, want %q", got, FailureClassModelRuntime)
	}
}

func TestRecoveryActionForReadiness_ReturnsActionableGuidance(t *testing.T) {
	t.Parallel()

	if got := recoveryActionForReadiness(managedruntime.ReadinessStateMissing); got == "" {
		t.Fatal("expected recovery action for missing runtime")
	}
	if got := recoveryActionForReadiness(managedruntime.ReadinessStateLoading); got == "" {
		t.Fatal("expected recovery action for loading runtime")
	}
}

func TestFormatAgentRunError_ToolRuntimeUsesSafeSummary(t *testing.T) {
	t.Parallel()

	err := newToolRuntimeError(ToolNameReadFile, `{"path":"missing.txt"}`, fs.ErrNotExist)
	got := formatAgentRunError(err)
	want := "agent run tool failure: read_file: path=missing.txt reason=not_found"
	if got != want {
		t.Fatalf("formatAgentRunError() = %q, want %q", got, want)
	}
	if strings.Contains(got, "open /") {
		t.Fatalf("formatAgentRunError() leaked absolute path details: %q", got)
	}
}

func TestFormatAgentRunError_AbsolutePathArgumentOmitsPath(t *testing.T) {
	t.Parallel()

	absolutePath := "/Users/test/secret.txt"
	err := newToolRuntimeError(
		ToolNameReadFile,
		fmt.Sprintf(`{"path":%q}`, absolutePath),
		errors.New("tool path must be relative to the agent working directory"),
	)
	got := formatAgentRunError(err)
	want := "agent run tool failure: read_file: reason=path_must_be_relative"
	if got != want {
		t.Fatalf("formatAgentRunError() = %q, want %q", got, want)
	}
	if strings.Contains(got, absolutePath) {
		t.Fatalf("formatAgentRunError() leaked absolute path argument: %q", got)
	}
}

func TestFormatAgentRunError_LegacyToolRuntimeMessageFallsBackToSafeSummary(t *testing.T) {
	t.Parallel()

	err := errors.New("read_file failed: open /tmp/agent/workdir/missing.txt: no such file or directory")
	got := formatAgentRunError(err)
	if got != "agent run tool failure: tool execution failed" {
		t.Fatalf("formatAgentRunError() = %q, want safe fallback summary", got)
	}
	if strings.Contains(got, "/tmp/agent/workdir") {
		t.Fatalf("formatAgentRunError() leaked absolute path: %q", got)
	}
}

func TestFormatAgentRunError_ModelhostErrorsUseAgentRunWording(t *testing.T) {
	t.Parallel()

	got := formatAgentRunError(errors.Join(errors.New("wrapped"), modelhost.ErrHostCapacityExhausted))
	if got == "" || got == "provider error" {
		t.Fatalf("formatAgentRunError = %q, want agent-run lease denial wording", got)
	}
}
