package interfaces

import (
	"strconv"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

const (
	AgentRunExecutionBehavior = "agent_run"

	AgentRunMetadataExecutionBehavior = "execution_behavior"
	AgentRunMetadataFailureClass      = "failure_class"
	AgentRunMetadataRecoveryAction    = "recovery_action"
	AgentRunMetadataToolPolicy        = "tool_policy"
	AgentRunMetadataToolCallCount     = "tool_call_count"
	AgentRunMetadataToolDiagnostics   = "tool_diagnostics"
)

// SafeAgentRunDiagnostic carries dashboard-safe agent-run inspection metadata.
type SafeAgentRunDiagnostic struct {
	ExecutionBehavior string                      `json:"execution_behavior,omitempty"`
	FailureClass      string                      `json:"failure_class,omitempty"`
	RecoveryAction    string                      `json:"recovery_action,omitempty"`
	ToolPolicy        string                      `json:"tool_policy,omitempty"`
	ToolCallCount     int                         `json:"tool_call_count,omitempty"`
	ToolDiagnostics   []AgentRunToolDiagnostic    `json:"tool_diagnostics,omitempty"`
	Transcript        []AgentRunTranscriptEntry   `json:"transcript,omitempty"`
}

// AgentRunToolDiagnostic records one bounded tool lifecycle summary.
type AgentRunToolDiagnostic struct {
	ToolName string `json:"tool_name,omitempty"`
	Phase    string `json:"phase,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// AgentRunTranscriptEntry records bounded transcript metadata for one message.
type AgentRunTranscriptEntry struct {
	Role    string `json:"role,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// SafeAgentRunDiagnosticFromWorkDiagnostics projects agent-run metadata from
// worker-facing diagnostics onto the canonical safe boundary.
func SafeAgentRunDiagnosticFromWorkDiagnostics(diagnostics *WorkDiagnostics) *SafeAgentRunDiagnostic {
	if diagnostics == nil || len(diagnostics.Metadata) == 0 {
		return nil
	}
	behavior := strings.TrimSpace(diagnostics.Metadata[AgentRunMetadataExecutionBehavior])
	if behavior != AgentRunExecutionBehavior {
		return nil
	}
	out := &SafeAgentRunDiagnostic{
		ExecutionBehavior: behavior,
		FailureClass:      strings.TrimSpace(diagnostics.Metadata[AgentRunMetadataFailureClass]),
		RecoveryAction:    strings.TrimSpace(diagnostics.Metadata[AgentRunMetadataRecoveryAction]),
		ToolPolicy:        strings.TrimSpace(diagnostics.Metadata[AgentRunMetadataToolPolicy]),
	}
	if count := strings.TrimSpace(diagnostics.Metadata[AgentRunMetadataToolCallCount]); count != "" {
		if parsed, err := strconv.Atoi(count); err == nil && parsed >= 0 {
			out.ToolCallCount = parsed
		}
	}
	if entries := parseAgentRunToolDiagnostics(diagnostics.Metadata[AgentRunMetadataToolDiagnostics]); len(entries) > 0 {
		out.ToolDiagnostics = entries
	}
	if out.FailureClass == "" && out.RecoveryAction == "" && out.ToolPolicy == "" &&
		out.ToolCallCount == 0 && len(out.ToolDiagnostics) == 0 {
		return &SafeAgentRunDiagnostic{ExecutionBehavior: behavior}
	}
	return out
}

func parseAgentRunToolDiagnostics(raw string) []AgentRunToolDiagnostic {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	out := make([]AgentRunToolDiagnostic, 0, len(parts))
	for _, part := range parts {
		entry := parseAgentRunToolDiagnosticEntry(part)
		if entry.ToolName == "" && entry.Phase == "" && entry.Detail == "" {
			continue
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseAgentRunToolDiagnosticEntry(raw string) AgentRunToolDiagnostic {
	segments := strings.Split(strings.TrimSpace(raw), ":")
	switch len(segments) {
	case 0:
		return AgentRunToolDiagnostic{}
	case 1:
		return AgentRunToolDiagnostic{ToolName: segments[0]}
	case 2:
		return AgentRunToolDiagnostic{ToolName: segments[0], Phase: segments[1]}
	default:
		return AgentRunToolDiagnostic{
			ToolName: segments[0],
			Phase:    segments[1],
			Detail:   strings.Join(segments[2:], ":"),
		}
	}
}

// GeneratedSafeAgentRunDiagnostic converts the canonical safe agent-run boundary
// into the generated event contract.
func GeneratedSafeAgentRunDiagnostic(diagnostic *SafeAgentRunDiagnostic) *factoryapi.SafeAgentRunDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &factoryapi.SafeAgentRunDiagnostic{
		FailureClass:   safeDiagnosticsStringPtrIfNotEmpty(diagnostic.FailureClass),
		RecoveryAction: safeDiagnosticsStringPtrIfNotEmpty(diagnostic.RecoveryAction),
		ToolPolicy:     safeDiagnosticsStringPtrIfNotEmpty(diagnostic.ToolPolicy),
	}
	if diagnostic.ExecutionBehavior != "" {
		behavior := factoryapi.SafeAgentRunDiagnosticExecutionBehavior(diagnostic.ExecutionBehavior)
		out.ExecutionBehavior = &behavior
	}
	if diagnostic.ToolCallCount > 0 {
		count := int32(diagnostic.ToolCallCount)
		out.ToolCallCount = &count
	}
	if entries := generatedAgentRunToolDiagnostics(diagnostic.ToolDiagnostics); entries != nil {
		out.ToolDiagnostics = entries
	}
	if entries := generatedAgentRunTranscript(diagnostic.Transcript); entries != nil {
		out.Transcript = entries
	}
	if out.ExecutionBehavior == nil && out.FailureClass == nil && out.RecoveryAction == nil &&
		out.ToolPolicy == nil && out.ToolCallCount == nil && out.ToolDiagnostics == nil && out.Transcript == nil {
		return nil
	}
	return out
}

// SafeAgentRunDiagnosticFromGenerated converts generated agent-run diagnostics
// into the canonical safe boundary.
func SafeAgentRunDiagnosticFromGenerated(diagnostic *factoryapi.SafeAgentRunDiagnostic) *SafeAgentRunDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &SafeAgentRunDiagnostic{
		ExecutionBehavior: safeDiagnosticsEnumStringValue(diagnostic.ExecutionBehavior),
		FailureClass:      safeDiagnosticsStringValue(diagnostic.FailureClass),
		RecoveryAction:    safeDiagnosticsStringValue(diagnostic.RecoveryAction),
		ToolPolicy:        safeDiagnosticsStringValue(diagnostic.ToolPolicy),
	}
	if diagnostic.ToolCallCount != nil {
		out.ToolCallCount = int(*diagnostic.ToolCallCount)
	}
	if diagnostic.ToolDiagnostics != nil {
		out.ToolDiagnostics = safeAgentRunToolDiagnosticsFromGenerated(*diagnostic.ToolDiagnostics)
	}
	if diagnostic.Transcript != nil {
		out.Transcript = safeAgentRunTranscriptFromGenerated(*diagnostic.Transcript)
	}
	if out.ExecutionBehavior == "" && out.FailureClass == "" && out.RecoveryAction == "" &&
		out.ToolPolicy == "" && out.ToolCallCount == 0 && len(out.ToolDiagnostics) == 0 && len(out.Transcript) == 0 {
		return nil
	}
	return out
}

// GeneratedFactoryWorldAgentRunInspectionView converts canonical agent-run
// inspection into the factory-world workstation response contract.
func GeneratedFactoryWorldAgentRunInspectionView(diagnostic *SafeAgentRunDiagnostic) *factoryapi.FactoryWorldAgentRunInspectionView {
	if diagnostic == nil {
		return nil
	}
	out := &factoryapi.FactoryWorldAgentRunInspectionView{
		ExecutionBehavior: safeDiagnosticsStringPtrIfNotEmpty(diagnostic.ExecutionBehavior),
		FailureClass:      safeDiagnosticsStringPtrIfNotEmpty(diagnostic.FailureClass),
		RecoveryAction:    safeDiagnosticsStringPtrIfNotEmpty(diagnostic.RecoveryAction),
		ToolPolicy:        safeDiagnosticsStringPtrIfNotEmpty(diagnostic.ToolPolicy),
	}
	if diagnostic.ToolCallCount > 0 {
		count := int32(diagnostic.ToolCallCount)
		out.ToolCallCount = &count
	}
	if entries := generatedAgentRunToolDiagnostics(diagnostic.ToolDiagnostics); entries != nil {
		out.ToolDiagnostics = entries
	}
	if entries := generatedAgentRunTranscript(diagnostic.Transcript); entries != nil {
		out.Transcript = entries
	}
	if out.ExecutionBehavior == nil && out.FailureClass == nil && out.RecoveryAction == nil &&
		out.ToolPolicy == nil && out.ToolCallCount == nil && out.ToolDiagnostics == nil && out.Transcript == nil {
		return nil
	}
	return out
}

func generatedAgentRunToolDiagnostics(entries []AgentRunToolDiagnostic) *[]factoryapi.AgentRunToolDiagnosticEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]factoryapi.AgentRunToolDiagnosticEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, factoryapi.AgentRunToolDiagnosticEntry{
			ToolName: safeDiagnosticsStringPtrIfNotEmpty(entry.ToolName),
			Phase:    safeDiagnosticsStringPtrIfNotEmpty(entry.Phase),
			Detail:   safeDiagnosticsStringPtrIfNotEmpty(entry.Detail),
		})
	}
	return &out
}

func generatedAgentRunTranscript(entries []AgentRunTranscriptEntry) *[]factoryapi.AgentRunTranscriptEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]factoryapi.AgentRunTranscriptEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, factoryapi.AgentRunTranscriptEntry{
			Role:    safeDiagnosticsStringPtrIfNotEmpty(entry.Role),
			Summary: safeDiagnosticsStringPtrIfNotEmpty(entry.Summary),
		})
	}
	return &out
}

func safeAgentRunToolDiagnosticsFromGenerated(entries []factoryapi.AgentRunToolDiagnosticEntry) []AgentRunToolDiagnostic {
	if len(entries) == 0 {
		return nil
	}
	out := make([]AgentRunToolDiagnostic, 0, len(entries))
	for _, entry := range entries {
		out = append(out, AgentRunToolDiagnostic{
			ToolName: safeDiagnosticsStringValue(entry.ToolName),
			Phase:    safeDiagnosticsStringValue(entry.Phase),
			Detail:   safeDiagnosticsStringValue(entry.Detail),
		})
	}
	return out
}

func safeAgentRunTranscriptFromGenerated(entries []factoryapi.AgentRunTranscriptEntry) []AgentRunTranscriptEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]AgentRunTranscriptEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, AgentRunTranscriptEntry{
			Role:    safeDiagnosticsStringValue(entry.Role),
			Summary: safeDiagnosticsStringValue(entry.Summary),
		})
	}
	return out
}

func cloneSafeAgentRunDiagnostic(diagnostic *SafeAgentRunDiagnostic) *SafeAgentRunDiagnostic {
	if diagnostic == nil {
		return nil
	}
	clone := &SafeAgentRunDiagnostic{
		ExecutionBehavior: diagnostic.ExecutionBehavior,
		FailureClass:      diagnostic.FailureClass,
		RecoveryAction:    diagnostic.RecoveryAction,
		ToolPolicy:        diagnostic.ToolPolicy,
		ToolCallCount:     diagnostic.ToolCallCount,
	}
	if len(diagnostic.ToolDiagnostics) > 0 {
		clone.ToolDiagnostics = make([]AgentRunToolDiagnostic, len(diagnostic.ToolDiagnostics))
		copy(clone.ToolDiagnostics, diagnostic.ToolDiagnostics)
	}
	if len(diagnostic.Transcript) > 0 {
		clone.Transcript = make([]AgentRunTranscriptEntry, len(diagnostic.Transcript))
		copy(clone.Transcript, diagnostic.Transcript)
	}
	return clone
}

func agentRunMetadataFromSafeDiagnostic(diagnostic *SafeAgentRunDiagnostic) map[string]string {
	if diagnostic == nil {
		return nil
	}
	metadata := map[string]string{
		AgentRunMetadataExecutionBehavior: firstNonEmptyAgentRunValue(diagnostic.ExecutionBehavior, AgentRunExecutionBehavior),
	}
	if diagnostic.FailureClass != "" {
		metadata[AgentRunMetadataFailureClass] = diagnostic.FailureClass
	}
	if diagnostic.RecoveryAction != "" {
		metadata[AgentRunMetadataRecoveryAction] = diagnostic.RecoveryAction
	}
	if diagnostic.ToolPolicy != "" {
		metadata[AgentRunMetadataToolPolicy] = diagnostic.ToolPolicy
	}
	if diagnostic.ToolCallCount > 0 {
		metadata[AgentRunMetadataToolCallCount] = strconv.Itoa(diagnostic.ToolCallCount)
	}
	if len(diagnostic.ToolDiagnostics) > 0 {
		summaries := make([]string, 0, len(diagnostic.ToolDiagnostics))
		for _, entry := range diagnostic.ToolDiagnostics {
			summary := entry.ToolName + ":" + entry.Phase
			if entry.Detail != "" {
				summary += ":" + entry.Detail
			}
			summaries = append(summaries, summary)
		}
		metadata[AgentRunMetadataToolDiagnostics] = strings.Join(summaries, ",")
	}
	return metadata
}

func firstNonEmptyAgentRunValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
