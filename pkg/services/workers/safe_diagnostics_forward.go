package workers

import (
	"encoding/json"

	workerdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers/internal/diagnostics"
)

// CloneSafeWorkDiagnostics returns a detached safe diagnostics snapshot.
func CloneSafeWorkDiagnostics(diagnostics *SafeWorkDiagnostics) *SafeWorkDiagnostics {
	return cloneSafeWorkDiagnosticsFromInternal(
		workerdiagnostics.CloneSafeWorkDiagnostics(cloneSafeWorkDiagnosticsToInternal(diagnostics)),
	)
}

// SafeWorkDiagnosticsFromWorkDiagnostics projects worker-internal diagnostics onto the safe boundary.
func SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics *WorkDiagnostics) *SafeWorkDiagnostics {
	return cloneSafeWorkDiagnosticsFromInternal(
		workerdiagnostics.SafeWorkDiagnosticsFromWorkDiagnostics(cloneWorkDiagnosticsToInternal(diagnostics)),
	)
}

// WorkDiagnosticsFromSafeEventPayload decodes the public camel-case event shape into worker diagnostics.
func WorkDiagnosticsFromSafeEventPayload(payload json.RawMessage) (*WorkDiagnostics, error) {
	safe, err := SafeWorkDiagnosticsFromEventPayload(payload)
	if err != nil {
		return nil, err
	}
	return WorkDiagnosticsFromSafeWorkDiagnostics(safe), nil
}

// SafeWorkDiagnosticsFromEventPayload decodes the public camel-case event shape into safe diagnostics.
func SafeWorkDiagnosticsFromEventPayload(payload json.RawMessage) (*SafeWorkDiagnostics, error) {
	safe, err := workerdiagnostics.SafeWorkDiagnosticsFromEventPayload(payload)
	if err != nil {
		return nil, err
	}
	return cloneSafeWorkDiagnosticsFromInternal(safe), nil
}

// SafeWorkDiagnosticsEventPayload encodes canonical safe diagnostics with camel-case field names.
func SafeWorkDiagnosticsEventPayload(diagnostics *SafeWorkDiagnostics) (json.RawMessage, error) {
	return workerdiagnostics.SafeWorkDiagnosticsEventPayload(cloneSafeWorkDiagnosticsToInternal(diagnostics))
}

// WorkDiagnosticsFromSafeWorkDiagnostics rehydrates worker-facing diagnostics from the safe boundary.
func WorkDiagnosticsFromSafeWorkDiagnostics(diagnostics *SafeWorkDiagnostics) *WorkDiagnostics {
	return cloneWorkDiagnosticsFromInternal(
		workerdiagnostics.WorkDiagnosticsFromSafeWorkDiagnostics(cloneSafeWorkDiagnosticsToInternal(diagnostics)),
	)
}

// SafeAgentRunDiagnosticFromWorkDiagnostics projects agent-run metadata onto the safe boundary.
func SafeAgentRunDiagnosticFromWorkDiagnostics(diagnostics *WorkDiagnostics) *SafeAgentRunDiagnostic {
	return cloneSafeAgentRunDiagnosticFromInternal(
		workerdiagnostics.SafeAgentRunDiagnosticFromWorkDiagnostics(cloneWorkDiagnosticsToInternal(diagnostics)),
	)
}

func cloneWorkDiagnosticsToInternal(diagnostics *WorkDiagnostics) *workerdiagnostics.WorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	out := &workerdiagnostics.WorkDiagnostics{
		Metadata: cloneDiagnosticsStringMap(diagnostics.Metadata),
	}
	if diagnostics.RenderedPrompt != nil {
		out.RenderedPrompt = &workerdiagnostics.RenderedPromptDiagnostic{
			SystemPromptHash: diagnostics.RenderedPrompt.SystemPromptHash,
			UserMessageHash:  diagnostics.RenderedPrompt.UserMessageHash,
			Variables:        cloneDiagnosticsStringMap(diagnostics.RenderedPrompt.Variables),
		}
	}
	if diagnostics.Provider != nil {
		out.Provider = &workerdiagnostics.ProviderDiagnostic{
			Provider:         diagnostics.Provider.Provider,
			Model:            diagnostics.Provider.Model,
			RequestMetadata:  cloneDiagnosticsStringMap(diagnostics.Provider.RequestMetadata),
			ResponseMetadata: cloneDiagnosticsStringMap(diagnostics.Provider.ResponseMetadata),
		}
	}
	if diagnostics.Invocation != nil {
		out.Invocation = cloneInvocationDiagnosticToInternal(diagnostics.Invocation)
	}
	return out
}

func cloneWorkDiagnosticsFromInternal(diagnostics *workerdiagnostics.WorkDiagnostics) *WorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	out := &WorkDiagnostics{
		Metadata: cloneDiagnosticsStringMap(diagnostics.Metadata),
	}
	if diagnostics.RenderedPrompt != nil {
		out.RenderedPrompt = &RenderedPromptDiagnostic{
			SystemPromptHash: diagnostics.RenderedPrompt.SystemPromptHash,
			UserMessageHash:  diagnostics.RenderedPrompt.UserMessageHash,
			Variables:        cloneDiagnosticsStringMap(diagnostics.RenderedPrompt.Variables),
		}
	}
	if diagnostics.Provider != nil {
		out.Provider = &ProviderDiagnostic{
			Provider:         diagnostics.Provider.Provider,
			Model:            diagnostics.Provider.Model,
			RequestMetadata:  cloneDiagnosticsStringMap(diagnostics.Provider.RequestMetadata),
			ResponseMetadata: cloneDiagnosticsStringMap(diagnostics.Provider.ResponseMetadata),
		}
	}
	if diagnostics.Invocation != nil {
		out.Invocation = cloneInvocationDiagnosticFromInternal(diagnostics.Invocation)
	}
	return out
}

func cloneSafeWorkDiagnosticsToInternal(diagnostics *SafeWorkDiagnostics) *workerdiagnostics.SafeWorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	out := &workerdiagnostics.SafeWorkDiagnostics{
		Invocation: cloneInvocationDiagnosticToInternal(diagnostics.Invocation),
	}
	if diagnostics.RenderedPrompt != nil {
		out.RenderedPrompt = &workerdiagnostics.SafeRenderedPromptDiagnostic{
			SystemPromptHash: diagnostics.RenderedPrompt.SystemPromptHash,
			UserMessageHash:  diagnostics.RenderedPrompt.UserMessageHash,
			Variables:        cloneDiagnosticsStringMap(diagnostics.RenderedPrompt.Variables),
		}
	}
	if diagnostics.Provider != nil {
		out.Provider = &workerdiagnostics.SafeProviderDiagnostic{
			Provider:         diagnostics.Provider.Provider,
			Model:            diagnostics.Provider.Model,
			RequestMetadata:  cloneDiagnosticsStringMap(diagnostics.Provider.RequestMetadata),
			ResponseMetadata: cloneDiagnosticsStringMap(diagnostics.Provider.ResponseMetadata),
		}
	}
	if diagnostics.AgentRun != nil {
		out.AgentRun = cloneSafeAgentRunDiagnosticToInternal(diagnostics.AgentRun)
	}
	return out
}

func cloneSafeWorkDiagnosticsFromInternal(diagnostics *workerdiagnostics.SafeWorkDiagnostics) *SafeWorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	out := &SafeWorkDiagnostics{
		Invocation: cloneInvocationDiagnosticFromInternal(diagnostics.Invocation),
	}
	if diagnostics.RenderedPrompt != nil {
		out.RenderedPrompt = &SafeRenderedPromptDiagnostic{
			SystemPromptHash: diagnostics.RenderedPrompt.SystemPromptHash,
			UserMessageHash:  diagnostics.RenderedPrompt.UserMessageHash,
			Variables:        cloneDiagnosticsStringMap(diagnostics.RenderedPrompt.Variables),
		}
	}
	if diagnostics.Provider != nil {
		out.Provider = &SafeProviderDiagnostic{
			Provider:         diagnostics.Provider.Provider,
			Model:            diagnostics.Provider.Model,
			RequestMetadata:  cloneDiagnosticsStringMap(diagnostics.Provider.RequestMetadata),
			ResponseMetadata: cloneDiagnosticsStringMap(diagnostics.Provider.ResponseMetadata),
		}
	}
	if diagnostics.AgentRun != nil {
		out.AgentRun = cloneSafeAgentRunDiagnosticFromInternal(diagnostics.AgentRun)
	}
	return out
}

func cloneSafeAgentRunDiagnosticToInternal(diagnostic *SafeAgentRunDiagnostic) *workerdiagnostics.SafeAgentRunDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &workerdiagnostics.SafeAgentRunDiagnostic{
		ExecutionBehavior: diagnostic.ExecutionBehavior,
		FailureClass:      diagnostic.FailureClass,
		RecoveryAction:    diagnostic.RecoveryAction,
		ToolPolicy:        diagnostic.ToolPolicy,
		ToolCallCount:     diagnostic.ToolCallCount,
	}
	if len(diagnostic.ToolDiagnostics) > 0 {
		out.ToolDiagnostics = make([]workerdiagnostics.AgentRunToolDiagnostic, len(diagnostic.ToolDiagnostics))
		for i, entry := range diagnostic.ToolDiagnostics {
			out.ToolDiagnostics[i] = workerdiagnostics.AgentRunToolDiagnostic{
				ToolName: entry.ToolName,
				Phase:    entry.Phase,
				Detail:   entry.Detail,
			}
		}
	}
	if len(diagnostic.Transcript) > 0 {
		out.Transcript = make([]workerdiagnostics.AgentRunTranscriptEntry, len(diagnostic.Transcript))
		for i, entry := range diagnostic.Transcript {
			out.Transcript[i] = workerdiagnostics.AgentRunTranscriptEntry{
				Role:    entry.Role,
				Summary: entry.Summary,
			}
		}
	}
	return out
}

func cloneSafeAgentRunDiagnosticFromInternal(diagnostic *workerdiagnostics.SafeAgentRunDiagnostic) *SafeAgentRunDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &SafeAgentRunDiagnostic{
		ExecutionBehavior: diagnostic.ExecutionBehavior,
		FailureClass:      diagnostic.FailureClass,
		RecoveryAction:    diagnostic.RecoveryAction,
		ToolPolicy:        diagnostic.ToolPolicy,
		ToolCallCount:     diagnostic.ToolCallCount,
	}
	if len(diagnostic.ToolDiagnostics) > 0 {
		out.ToolDiagnostics = make([]AgentRunToolDiagnostic, len(diagnostic.ToolDiagnostics))
		for i, entry := range diagnostic.ToolDiagnostics {
			out.ToolDiagnostics[i] = AgentRunToolDiagnostic{
				ToolName: entry.ToolName,
				Phase:    entry.Phase,
				Detail:   entry.Detail,
			}
		}
	}
	if len(diagnostic.Transcript) > 0 {
		out.Transcript = make([]AgentRunTranscriptEntry, len(diagnostic.Transcript))
		for i, entry := range diagnostic.Transcript {
			out.Transcript[i] = AgentRunTranscriptEntry{
				Role:    entry.Role,
				Summary: entry.Summary,
			}
		}
	}
	return out
}

func cloneInvocationDiagnosticToInternal(diagnostic *InvocationDiagnostic) *workerdiagnostics.InvocationDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &workerdiagnostics.InvocationDiagnostic{SignatureHash: diagnostic.SignatureHash}
	if len(diagnostic.Parameters) > 0 {
		out.Parameters = make([]workerdiagnostics.InvocationParameterDiagnostic, len(diagnostic.Parameters))
		for i, parameter := range diagnostic.Parameters {
			out.Parameters[i] = workerdiagnostics.InvocationParameterDiagnostic{
				Name:        parameter.Name,
				SourceKinds: append([]string(nil), parameter.SourceKinds...),
				ValueCount:  parameter.ValueCount,
				Redacted:    parameter.Redacted,
			}
		}
	}
	return out
}

func cloneInvocationDiagnosticFromInternal(diagnostic *workerdiagnostics.InvocationDiagnostic) *InvocationDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &InvocationDiagnostic{SignatureHash: diagnostic.SignatureHash}
	if len(diagnostic.Parameters) > 0 {
		out.Parameters = make([]InvocationParameterDiagnostic, len(diagnostic.Parameters))
		for i, parameter := range diagnostic.Parameters {
			out.Parameters[i] = InvocationParameterDiagnostic{
				Name:        parameter.Name,
				SourceKinds: append([]string(nil), parameter.SourceKinds...),
				ValueCount:  parameter.ValueCount,
				Redacted:    parameter.Redacted,
			}
		}
	}
	return out
}

func cloneDiagnosticsStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
