package agentrun

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	DiagnosticExecutionBehavior = "execution_behavior"
	ExecutionBehaviorAgentRun   = "agent_run"

	DiagnosticFailureClass     = "failure_class"
	FailureClassHarnessRuntime = "agent_run_harness_failure"
	FailureClassCanceled       = "agent_run_canceled"
	FailureClassTimeout        = "agent_run_timeout"
)

func agentRunDiagnostics(extra map[string]string) *interfaces.WorkDiagnostics {
	metadata := map[string]string{
		DiagnosticExecutionBehavior: ExecutionBehaviorAgentRun,
	}
	for key, value := range extra {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		metadata[key] = value
	}
	return &interfaces.WorkDiagnostics{Metadata: metadata}
}

func failureClassForError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return FailureClassCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureClassTimeout
	}
	return FailureClassHarnessRuntime
}

func failureMetadataForError(err error) *interfaces.WorkFailureMetadata {
	if err == nil {
		return nil
	}
	family := interfaces.WorkFailureFamilyTerminal
	failureType := interfaces.WorkFailureTypeInternalServerError
	if errors.Is(err, context.Canceled) {
		failureType = interfaces.WorkFailureTypeUnknown
	}
	if errors.Is(err, context.DeadlineExceeded) {
		family = interfaces.WorkFailureFamilyRetryable
		failureType = interfaces.WorkFailureTypeTimeout
	}
	return &interfaces.WorkFailureMetadata{
		Family: family,
		Type:   failureType,
	}
}

func formatAgentRunError(err error) string {
	if err == nil {
		return "agent run failed"
	}
	switch failureClassForError(err) {
	case FailureClassCanceled:
		return "agent run canceled"
	case FailureClassTimeout:
		return "agent run timeout"
	default:
		return "agent run harness failure: " + err.Error()
	}
}
