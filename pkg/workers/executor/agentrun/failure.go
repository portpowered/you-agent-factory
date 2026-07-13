package agentrun

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
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

func failureMetadataForError(err error) *interfaces.WorkFailureMetadata {
	if err == nil {
		return nil
	}
	if metadata := modelhostFailureMetadata(err); metadata != nil {
		return metadata
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
	if errors.Is(err, modelhost.ErrCapacityExhausted) {
		return "retry later or increase managed runtime resource capacity"
	}
	var readinessErr *modelhost.ReadinessError
	if errors.As(err, &readinessErr) {
		return recoveryActionForReadiness(readinessErr.Snapshot.ReadinessState)
	}
	var invocationErr *apisurface.ManagedRuntimeInvocationError
	if errors.As(err, &invocationErr) {
		return recoveryActionForReadiness(invocationErr.ReadinessState)
	}
	if errors.Is(err, modelhost.ErrProcessCrash) {
		return "resolve the managed runtime failure before retrying the agent run"
	}
	return ""
}

func recoveryActionForReadiness(readiness factoryapi.ManagedRuntimeReadinessState) string {
	switch readiness {
	case factoryapi.ManagedRuntimeReadinessStateMISSING:
		return "pull or install the managed runtime before retrying the agent run"
	case factoryapi.ManagedRuntimeReadinessStateLOADING:
		return "wait for the managed runtime to finish loading before retrying the agent run"
	case factoryapi.ManagedRuntimeReadinessStateFAILED:
		return "resolve the managed runtime failure before retrying the agent run"
	case factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED:
		return "use a supported managed runtime configuration for the agent worker"
	default:
		return ""
	}
}

func modelhostFailureClass(err error) (string, bool) {
	if errors.Is(err, modelhost.ErrCapacityExhausted) {
		return FailureClassLeaseDenied, true
	}
	var readinessErr *modelhost.ReadinessError
	if errors.As(err, &readinessErr) {
		return readinessFailureClass(readinessErr), true
	}
	var invocationErr *apisurface.ManagedRuntimeInvocationError
	if errors.As(err, &invocationErr) {
		return managedRuntimeInvocationFailureClass(invocationErr), true
	}
	if errors.Is(err, modelhost.ErrProcessCrash) ||
		errors.Is(err, modelhost.ErrUnsupportedRuntime) ||
		errors.Is(err, modelhost.ErrMissingAssets) {
		return modelhostOperationalFailureClass(err), true
	}
	return "", false
}

func readinessFailureClass(err *modelhost.ReadinessError) string {
	if err == nil {
		return FailureClassModelRuntime
	}
	switch err.Snapshot.FailureClass {
	case modelhost.FailureClassCapacityExhausted:
		return FailureClassLeaseDenied
	case modelhost.FailureClassMissingAssets, modelhost.FailureClassLoadingTimeout:
		return FailureClassModelNotReady
	default:
		switch err.Snapshot.ReadinessState {
		case factoryapi.ManagedRuntimeReadinessStateMISSING, factoryapi.ManagedRuntimeReadinessStateLOADING:
			return FailureClassModelNotReady
		default:
			return FailureClassModelRuntime
		}
	}
}

func managedRuntimeInvocationFailureClass(err *apisurface.ManagedRuntimeInvocationError) string {
	switch {
	case errors.Is(err, apisurface.ErrManagedRuntimeMissing),
		errors.Is(err, apisurface.ErrManagedRuntimeLoading):
		return FailureClassModelNotReady
	default:
		return FailureClassModelRuntime
	}
}

func modelhostOperationalFailureClass(err error) string {
	switch {
	case errors.Is(err, modelhost.ErrMissingAssets):
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

func modelhostFailureMetadata(err error) *interfaces.WorkFailureMetadata {
	class, ok := modelhostFailureClass(err)
	if !ok {
		return nil
	}
	switch class {
	case FailureClassLeaseDenied:
		return &interfaces.WorkFailureMetadata{
			Family: interfaces.WorkFailureFamilyThrottle,
			Type:   interfaces.WorkFailureTypeThrottled,
		}
	case FailureClassModelNotReady:
		return &interfaces.WorkFailureMetadata{
			Family: interfaces.WorkFailureFamilyRetryable,
			Type:   interfaces.WorkFailureTypeTimeout,
		}
	default:
		return &interfaces.WorkFailureMetadata{
			Family: interfaces.WorkFailureFamilyTerminal,
			Type:   interfaces.WorkFailureTypeInternalServerError,
		}
	}
}
