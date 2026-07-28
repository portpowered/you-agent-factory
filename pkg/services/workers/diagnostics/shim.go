// Package diagnostics is a transitional compile shim that re-exports safe
// diagnostics helpers from the private workers/internal destination. Peers
// should construct through workers/wire; baseline deletion of this path is
// owned by DEL-WRK.
package diagnostics

import (
	private "github.com/portpowered/infinite-you/pkg/services/workers/internal/diagnostics"
)

type (
	SafeWorkDiagnostics          = private.SafeWorkDiagnostics
	SafeRenderedPromptDiagnostic = private.SafeRenderedPromptDiagnostic
	SafeProviderDiagnostic       = private.SafeProviderDiagnostic
	SafeAgentRunDiagnostic       = private.SafeAgentRunDiagnostic
	AgentRunToolDiagnostic       = private.AgentRunToolDiagnostic
	AgentRunTranscriptEntry      = private.AgentRunTranscriptEntry
)

const (
	AgentRunExecutionBehavior         = private.AgentRunExecutionBehavior
	AgentRunMetadataExecutionBehavior = private.AgentRunMetadataExecutionBehavior
	AgentRunMetadataFailureClass      = private.AgentRunMetadataFailureClass
	AgentRunMetadataRecoveryAction    = private.AgentRunMetadataRecoveryAction
	AgentRunMetadataToolPolicy        = private.AgentRunMetadataToolPolicy
	AgentRunMetadataToolCallCount     = private.AgentRunMetadataToolCallCount
	AgentRunMetadataToolDiagnostics   = private.AgentRunMetadataToolDiagnostics
)

var (
	CloneSafeWorkDiagnostics               = private.CloneSafeWorkDiagnostics
	SafeWorkDiagnosticsFromWorkDiagnostics = private.SafeWorkDiagnosticsFromWorkDiagnostics
	WorkDiagnosticsFromSafeEventPayload    = private.WorkDiagnosticsFromSafeEventPayload
	SafeWorkDiagnosticsFromEventPayload    = private.SafeWorkDiagnosticsFromEventPayload
	SafeWorkDiagnosticsEventPayload        = private.SafeWorkDiagnosticsEventPayload
	WorkDiagnosticsFromSafeWorkDiagnostics = private.WorkDiagnosticsFromSafeWorkDiagnostics
	SafeAgentRunDiagnosticFromWorkDiagnostics = private.SafeAgentRunDiagnosticFromWorkDiagnostics
)
