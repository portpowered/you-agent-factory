package service

import (
	"errors"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestSafeDetail_NoFailureMetadata_ReturnsFixedGenericPlaceholderForKind(t *testing.T) {
	for kind, want := range genericFailureDetail {
		if got := safeDetail(kind, nil); got != want {
			t.Fatalf("safeDetail(%q, nil) = %q, want %q", kind, got, want)
		}
	}
}

func TestSafeDetail_WithFailureMetadata_ReturnsClosedVocabularyFamilyAndType(t *testing.T) {
	metadata := &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyRetryable,
		Type:   workers.WorkFailureTypeTimeout,
	}
	got := safeDetail(workersessions.FailureCauseWorkersExecutionFailure, metadata)
	want := "family=retryable type=timeout"
	if got != want {
		t.Fatalf("safeDetail() = %q, want %q", got, want)
	}
}

func TestSafeDetail_WithPartialFailureMetadata_FillsMissingHalfWithUnknown(t *testing.T) {
	metadata := &workers.WorkFailureMetadata{Type: workers.WorkFailureTypeAuthFailure}
	got := safeDetail(workersessions.FailureCauseWorkersExecutionFailure, metadata)
	want := "family=unknown type=auth_failure"
	if got != want {
		t.Fatalf("safeDetail() = %q, want %q", got, want)
	}
}

func TestSafeDetail_WithEmptyFailureMetadata_FallsBackToGenericPlaceholder(t *testing.T) {
	got := safeDetail(workersessions.FailureCauseWorkersExecutionFailure, &workers.WorkFailureMetadata{})
	want := genericFailureDetail[workersessions.FailureCauseWorkersExecutionFailure]
	if got != want {
		t.Fatalf("safeDetail() = %q, want fixed generic placeholder %q", got, want)
	}
}

// TestSafeDetail_WithUnrecognizedTypeValue_FallsBackToGenericPlaceholder
// proves the exact review concern: WorkFailureType is an exported Go string
// type, not a runtime-validated enum, so any string (including
// attacker-controlled prompt/command/credential text) can be constructed and
// attached as WorkFailureMetadata.Type. A value outside the whitelisted
// constants must never be echoed into Detail.
func TestSafeDetail_WithUnrecognizedTypeValue_FallsBackToGenericPlaceholder(t *testing.T) {
	metadata := &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyTerminal,
		Type:   workers.WorkFailureType("codex exec summarize confidential acquisition memo"),
	}
	got := safeDetail(workersessions.FailureCauseWorkersExecutionFailure, metadata)
	want := genericFailureDetail[workersessions.FailureCauseWorkersExecutionFailure]
	if got != want {
		t.Fatalf("safeDetail() = %q, want fixed generic placeholder %q (unrecognized Type must never be echoed)", got, want)
	}
	if strings.Contains(got, "confidential") {
		t.Fatalf("safeDetail() leaked unrecognized Type text: %q", got)
	}
}

// TestSafeDetail_WithUnrecognizedFamilyValue_FallsBackToGenericPlaceholder
// mirrors TestSafeDetail_WithUnrecognizedTypeValue_FallsBackToGenericPlaceholder
// for WorkFailureMetadata.Family.
func TestSafeDetail_WithUnrecognizedFamilyValue_FallsBackToGenericPlaceholder(t *testing.T) {
	metadata := &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamily("password=hunter2"),
		Type:   workers.WorkFailureTypeTimeout,
	}
	got := safeDetail(workersessions.FailureCauseWorkersExecutionFailure, metadata)
	want := genericFailureDetail[workersessions.FailureCauseWorkersExecutionFailure]
	if got != want {
		t.Fatalf("safeDetail() = %q, want fixed generic placeholder %q (unrecognized Family must never be echoed)", got, want)
	}
	if strings.Contains(got, "hunter2") {
		t.Fatalf("safeDetail() leaked unrecognized Family text: %q", got)
	}
}

// TestClassifyTerminal_UnrecognizedFailureMetadataWithSensitiveText_NeverExposesItInDetail
// proves the review's exact end-to-end scenario through classifyTerminal:
// a WorkResult carrying an unrecognized (attacker-controlled-shaped)
// WorkFailureMetadata.Type never surfaces that text in the committed
// FailureCause.Detail.
func TestClassifyTerminal_UnrecognizedFailureMetadataWithSensitiveText_NeverExposesItInDetail(t *testing.T) {
	dispatchResult := workers.WorkstationDispatchResult{
		Result: workers.WorkResult{
			Outcome: workers.OutcomeFailed,
			FailureMetadata: &workers.WorkFailureMetadata{
				Type: workers.WorkFailureType("codex exec summarize confidential acquisition memo"),
			},
		},
	}

	terminal := classifyTerminal(nil, dispatchResult)

	if terminal.Cause == nil {
		t.Fatal("terminal cause = nil, want non-nil")
	}
	if strings.Contains(terminal.Cause.Detail, "confidential") {
		t.Fatalf("terminal cause detail leaked unrecognized metadata text: %q", terminal.Cause.Detail)
	}
	want := genericFailureDetail[workersessions.FailureCauseWorkersExecutionFailure]
	if terminal.Cause.Detail != want {
		t.Fatalf("terminal cause detail = %q, want fixed generic placeholder %q", terminal.Cause.Detail, want)
	}
}

// TestClassifyTerminal_FailureMetadataPresentAlongsideSensitiveRawText_UsesOnlyMetadata
// proves classifyTerminal's Detail comes exclusively from the closed-
// vocabulary FailureMetadata even when a sensitive/free-form WorkResult.Error
// is also present: the raw text is never consulted for Detail, so it cannot
// leak regardless of its content.
func TestClassifyTerminal_FailureMetadataPresentAlongsideSensitiveRawText_UsesOnlyMetadata(t *testing.T) {
	dispatchResult := workers.WorkstationDispatchResult{
		Result: workers.WorkResult{
			Outcome: workers.OutcomeFailed,
			Error:   "password=hunter2 while running the confidential board memo prompt",
			FailureMetadata: &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyTerminal,
				Type:   workers.WorkFailureTypeAuthFailure,
			},
		},
	}

	terminal := classifyTerminal(nil, dispatchResult)

	if terminal.Cause == nil {
		t.Fatal("terminal cause = nil, want non-nil")
	}
	want := "family=terminal type=auth_failure"
	if terminal.Cause.Detail != want {
		t.Fatalf("terminal cause detail = %q, want %q", terminal.Cause.Detail, want)
	}
	if strings.Contains(terminal.Cause.Detail, "hunter2") || strings.Contains(terminal.Cause.Detail, "confidential") {
		t.Fatalf("terminal cause detail leaked raw text: %q", terminal.Cause.Detail)
	}
}

// TestClassifyTerminal_ExecutorPanicWithSensitiveRawEvidence_ClassifiesCorrectlyWithoutLeakingDetail
// proves executor-panic classification still works from raw evidence text
// (isExecutorPanicEvidence legitimately inspects it), while Detail itself
// never reproduces any part of that raw text.
func TestClassifyTerminal_ExecutorPanicWithSensitiveRawEvidence_ClassifiesCorrectlyWithoutLeakingDetail(t *testing.T) {
	dispatchResult := workers.WorkstationDispatchResult{
		Result: workers.WorkResult{
			Outcome: workers.OutcomeFailed,
			Error:   "executor panic: authorization Bearer sk-live-abc123 rejected",
		},
	}

	terminal := classifyTerminal(nil, dispatchResult)

	if terminal.Outcome != workersessions.TerminalOutcomeFailed {
		t.Fatalf("terminal outcome = %q, want FAILED", terminal.Outcome)
	}
	if terminal.Cause == nil || terminal.Cause.Kind != workersessions.FailureCauseExecutorPanic {
		t.Fatalf("terminal cause = %+v, want Kind EXECUTOR_PANIC", terminal.Cause)
	}
	if strings.Contains(terminal.Cause.Detail, "sk-live-abc123") {
		t.Fatalf("terminal cause detail leaked secret: %q", terminal.Cause.Detail)
	}
	if terminal.Cause.Detail != genericFailureDetail[workersessions.FailureCauseExecutorPanic] {
		t.Fatalf("terminal cause detail = %q, want fixed generic placeholder %q", terminal.Cause.Detail, genericFailureDetail[workersessions.FailureCauseExecutorPanic])
	}
}

func TestClassifyTerminal_StartFailureWithSensitiveAdapterError_NeverAttachesRawText(t *testing.T) {
	dispatchErr := errors.New("dial tcp failed: password=hunter2")

	terminal := classifyTerminal(dispatchErr, workers.WorkstationDispatchResult{})

	if terminal.Cause == nil {
		t.Fatal("terminal cause = nil, want non-nil")
	}
	if strings.Contains(terminal.Cause.Detail, "hunter2") {
		t.Fatalf("terminal cause detail leaked secret: %q", terminal.Cause.Detail)
	}
	if terminal.Cause.Detail != genericFailureDetail[workersessions.FailureCauseStartFailure] {
		t.Fatalf("terminal cause detail = %q, want fixed generic placeholder %q", terminal.Cause.Detail, genericFailureDetail[workersessions.FailureCauseStartFailure])
	}
}
