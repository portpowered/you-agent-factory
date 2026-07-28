// Package diagnostics retains same-service compatibility names while the
// customer-safe diagnostics contract lives at pkg/services/workers.
package diagnostics

import workers "github.com/portpowered/infinite-you/pkg/services/workers"

type SafeWorkDiagnostics = workers.SafeWorkDiagnostics
type SafeRenderedPromptDiagnostic = workers.SafeRenderedPromptDiagnostic
type SafeProviderDiagnostic = workers.SafeProviderDiagnostic
type SafeAgentRunDiagnostic = workers.SafeAgentRunDiagnostic
type AgentRunToolDiagnostic = workers.AgentRunToolDiagnostic
type AgentRunTranscriptEntry = workers.AgentRunTranscriptEntry

const (
	AgentRunExecutionBehavior         = workers.AgentRunExecutionBehavior
	AgentRunMetadataExecutionBehavior = workers.AgentRunMetadataExecutionBehavior
	AgentRunMetadataFailureClass      = workers.AgentRunMetadataFailureClass
	AgentRunMetadataRecoveryAction    = workers.AgentRunMetadataRecoveryAction
	AgentRunMetadataToolPolicy        = workers.AgentRunMetadataToolPolicy
	AgentRunMetadataToolCallCount     = workers.AgentRunMetadataToolCallCount
	AgentRunMetadataToolDiagnostics   = workers.AgentRunMetadataToolDiagnostics
)

var CloneSafeWorkDiagnostics = workers.CloneSafeWorkDiagnostics
var SafeWorkDiagnosticsFromWorkDiagnostics = workers.SafeWorkDiagnosticsFromWorkDiagnostics
var WorkDiagnosticsFromSafeEventPayload = workers.WorkDiagnosticsFromSafeEventPayload
var SafeWorkDiagnosticsFromEventPayload = workers.SafeWorkDiagnosticsFromEventPayload
var SafeWorkDiagnosticsEventPayload = workers.SafeWorkDiagnosticsEventPayload
var WorkDiagnosticsFromSafeWorkDiagnostics = workers.WorkDiagnosticsFromSafeWorkDiagnostics
var SafeAgentRunDiagnosticFromWorkDiagnostics = workers.SafeAgentRunDiagnosticFromWorkDiagnostics
