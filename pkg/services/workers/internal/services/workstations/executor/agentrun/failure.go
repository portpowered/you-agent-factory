package agentrun

import (
	"context"
	"errors"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

const (
	DiagnosticExecutionBehavior = "execution_behavior"
	ExecutionBehaviorAgentRun   = "agent_run"

	DiagnosticFailureClass   = "failure_class"
	DiagnosticRecoveryAction = "recovery_action"

	FailureClassHarnessRuntime = "agent_run_harness_failure"
	FailureClassCanceled       = "agent_run_canceled"
	FailureClassTimeout        = "agent_run_timeout"
	FailureClassLeaseDenied    = "agent_run_lease_denied"
	FailureClassModelNotReady  = "agent_run_model_not_ready"
	FailureClassModelRuntime   = "agent_run_model_runtime_failure"
	FailureClassToolDenied     = "agent_run_tool_denied"
	FailureClassToolPolicy     = "agent_run_tool_policy_violation"
	FailureClassToolRuntime    = "agent_run_tool_failure"
)

func agentRunDiagnostics(extra map[string]string) *workerexecution.WorkDiagnostics {
	metadata := map[string]string{
		DiagnosticExecutionBehavior: ExecutionBehaviorAgentRun,
	}
	for key, value := range extra {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		metadata[key] = value
	}
	return &workerexecution.WorkDiagnostics{Metadata: metadata}
}

func failureClassForError(err error) string {
	if err == nil {
		return ""
	}
	if class, ok := toolFailureClass(err); ok {
		return class
	}
	if class, ok := modelhostFailureClass(err); ok {
		return class
	}
	if errors.Is(err, context.Canceled) {
		return FailureClassCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureClassTimeout
	}
	return FailureClassHarnessRuntime
}

func failureMetadataForError(err error) *workerexecution.WorkFailureMetadata {
	if err == nil {
		return nil
	}
	if metadata := modelhostFailureMetadata(err); metadata != nil {
		return metadata
	}
	family := workerexecution.WorkFailureFamilyTerminal
	failureType := workerexecution.WorkFailureTypeInternalServerError
	if errors.Is(err, context.Canceled) {
		failureType = workerexecution.WorkFailureTypeUnknown
	}
	if errors.Is(err, context.DeadlineExceeded) {
		family = workerexecution.WorkFailureFamilyRetryable
		failureType = workerexecution.WorkFailureTypeTimeout
	}
	return &workerexecution.WorkFailureMetadata{
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
	case FailureClassLeaseDenied:
		return "agent run lease denied: " + err.Error()
	case FailureClassModelNotReady:
		return "agent run model not ready: " + err.Error()
	case FailureClassModelRuntime:
		return "agent run model runtime failure: " + err.Error()
	case FailureClassToolDenied, FailureClassToolPolicy:
		return "agent run tool policy violation: " + safeToolPolicyFailureSummary(err)
	case FailureClassToolRuntime:
		return "agent run tool failure: " + safeToolRuntimeFailureSummary(err)
	default:
		return "agent run harness failure: " + err.Error()
	}
}

func agentRunFailureDiagnostics(err error) map[string]string {
	diagnostics := map[string]string{
		DiagnosticFailureClass: failureClassForError(err),
	}
	if action := recoveryActionForError(err); action != "" {
		diagnostics[DiagnosticRecoveryAction] = action
	}
	return diagnostics
}

func recoveryActionForError(err error) string {
	if errors.Is(err, models.ErrHostCapacityExhausted) {
		return "retry later or increase managed runtime resource capacity"
	}
	var readinessErr *models.HostReadinessError
	if errors.As(err, &readinessErr) {
		return recoveryActionForReadiness(models.ReadinessState(readinessErr.Snapshot.ReadinessState))
	}
	if readiness, ok := managedRuntimeReadinessState(err); ok {
		return recoveryActionForReadiness(readiness)
	}
	if errors.Is(err, models.ErrHostProcessCrash) {
		return "resolve the managed runtime failure before retrying the agent run"
	}
	return ""
}

func recoveryActionForReadiness(readiness models.ReadinessState) string {
	switch readiness {
	case models.ReadinessStateMissing:
		return "pull or install the managed runtime before retrying the agent run"
	case models.ReadinessStateLoading:
		return "wait for the managed runtime to finish loading before retrying the agent run"
	case models.ReadinessStateFailed:
		return "resolve the managed runtime failure before retrying the agent run"
	case models.ReadinessStateUnsupported:
		return "use a supported managed runtime configuration for the agent worker"
	default:
		return ""
	}
}

func modelhostFailureClass(err error) (string, bool) {
	if errors.Is(err, models.ErrHostCapacityExhausted) {
		return FailureClassLeaseDenied, true
	}
	var readinessErr *models.HostReadinessError
	if errors.As(err, &readinessErr) {
		return readinessFailureClass(readinessErr), true
	}
	if class, ok := managedRuntimeInvocationFailureClass(err); ok {
		return class, true
	}
	if errors.Is(err, models.ErrHostProcessCrash) ||
		errors.Is(err, models.ErrHostUnsupportedRuntime) ||
		errors.Is(err, models.ErrHostMissingAssets) {
		return modelhostOperationalFailureClass(err), true
	}
	return "", false
}

func readinessFailureClass(err *models.HostReadinessError) string {
	if err == nil {
		return FailureClassModelRuntime
	}
	switch err.Snapshot.FailureClass {
	case models.HostFailureClassCapacityExhausted:
		return FailureClassLeaseDenied
	case models.HostFailureClassMissingAssets, models.HostFailureClassLoadingTimeout:
		return FailureClassModelNotReady
	default:
		switch models.ReadinessState(err.Snapshot.ReadinessState) {
		case models.ReadinessStateMissing, models.ReadinessStateLoading:
			return FailureClassModelNotReady
		default:
			return FailureClassModelRuntime
		}
	}
}

func managedRuntimeInvocationFailureClass(err error) (string, bool) {
	readiness, ok := managedRuntimeReadinessState(err)
	if !ok {
		return "", false
	}
	switch readiness {
	case models.ReadinessStateMissing, models.ReadinessStateLoading:
		return FailureClassModelNotReady, true
	default:
		return FailureClassModelRuntime, true
	}
}

func managedRuntimeReadinessState(err error) (models.ReadinessState, bool) {
	var readinessErr models.InvocationReadinessError
	if errors.As(err, &readinessErr) {
		return readinessErr.ManagedRuntimeReadinessState(), true
	}
	switch {
	case errors.Is(err, models.ErrMissing):
		return models.ReadinessStateMissing, true
	case errors.Is(err, models.ErrLoading):
		return models.ReadinessStateLoading, true
	case errors.Is(err, models.ErrFailed):
		return models.ReadinessStateFailed, true
	case errors.Is(err, models.ErrUnsupported):
		return models.ReadinessStateUnsupported, true
	default:
		return "", false
	}
}

func modelhostOperationalFailureClass(err error) string {
	switch {
	case errors.Is(err, models.ErrHostMissingAssets):
		return FailureClassModelNotReady
	default:
		return FailureClassModelRuntime
	}
}

func safeToolPolicyFailureSummary(err error) string {
	if err == nil {
		return "tool policy violation"
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "tool policy violation"
	}
	return message
}

func safeToolRuntimeFailureSummary(err error) string {
	var toolErr *toolRuntimeError
	if errors.As(err, &toolErr) {
		return toolErr.Error()
	}
	return "tool execution failed"
}

func toolFailureClass(err error) (string, bool) {
	if errors.Is(err, ErrToolPolicyDenied) {
		return FailureClassToolPolicy, true
	}
	if errors.Is(err, ErrToolNotSupported) {
		return FailureClassToolDenied, true
	}
	if isToolRuntimeError(err) {
		return FailureClassToolRuntime, true
	}
	return "", false
}

func modelhostFailureMetadata(err error) *workerexecution.WorkFailureMetadata {
	class, ok := modelhostFailureClass(err)
	if !ok {
		return nil
	}
	switch class {
	case FailureClassLeaseDenied:
		return &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyThrottle,
			Type:   workerexecution.WorkFailureTypeThrottled,
		}
	case FailureClassModelNotReady:
		return &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeTimeout,
		}
	default:
		return &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyTerminal,
			Type:   workerexecution.WorkFailureTypeInternalServerError,
		}
	}
}
