package factorycontracts

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
)

type SafeWorkDiagnostics = workerdiagnostics.SafeWorkDiagnostics
type SafeRenderedPromptDiagnostic = workerdiagnostics.SafeRenderedPromptDiagnostic
type SafeProviderDiagnostic = workerdiagnostics.SafeProviderDiagnostic
type SafeAgentRunDiagnostic = workerdiagnostics.SafeAgentRunDiagnostic
type AgentRunToolDiagnostic = workerdiagnostics.AgentRunToolDiagnostic
type AgentRunTranscriptEntry = workerdiagnostics.AgentRunTranscriptEntry

const (
	AgentRunExecutionBehavior         = workerdiagnostics.AgentRunExecutionBehavior
	AgentRunMetadataExecutionBehavior = workerdiagnostics.AgentRunMetadataExecutionBehavior
	AgentRunMetadataFailureClass      = workerdiagnostics.AgentRunMetadataFailureClass
	AgentRunMetadataRecoveryAction    = workerdiagnostics.AgentRunMetadataRecoveryAction
	AgentRunMetadataToolPolicy        = workerdiagnostics.AgentRunMetadataToolPolicy
	AgentRunMetadataToolCallCount     = workerdiagnostics.AgentRunMetadataToolCallCount
	AgentRunMetadataToolDiagnostics   = workerdiagnostics.AgentRunMetadataToolDiagnostics
)

func SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics *WorkDiagnostics) *SafeWorkDiagnostics {
	return workerdiagnostics.SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics)
}

func SafeWorkDiagnosticsFromGenerated(diagnostics *factoryapi.SafeWorkDiagnostics) *SafeWorkDiagnostics {
	return workerdiagnostics.SafeWorkDiagnosticsFromGenerated(diagnostics)
}

func GeneratedSafeWorkDiagnostics(diagnostics *SafeWorkDiagnostics) *factoryapi.SafeWorkDiagnostics {
	return workerdiagnostics.GeneratedSafeWorkDiagnostics(diagnostics)
}

func WorkDiagnosticsFromSafeWorkDiagnostics(diagnostics *SafeWorkDiagnostics) *WorkDiagnostics {
	return workerdiagnostics.WorkDiagnosticsFromSafeWorkDiagnostics(diagnostics)
}

func ProviderSessionMetadataFromGenerated(session *factoryapi.ProviderSessionMetadata) *ProviderSessionMetadata {
	return workerdiagnostics.ProviderSessionMetadataFromGenerated(session)
}

func SafeAgentRunDiagnosticFromWorkDiagnostics(diagnostics *WorkDiagnostics) *SafeAgentRunDiagnostic {
	return workerdiagnostics.SafeAgentRunDiagnosticFromWorkDiagnostics(diagnostics)
}
