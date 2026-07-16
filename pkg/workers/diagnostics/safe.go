// Package diagnostics owns customer-safe worker diagnostic projection.
package diagnostics

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// SafeWorkDiagnostics carries the canonical dashboard-safe execution
// diagnostics surface used by event history, replay, and selected-tick
// projections.
type SafeWorkDiagnostics struct {
	RenderedPrompt *SafeRenderedPromptDiagnostic         `json:"rendered_prompt,omitempty"`
	Provider       *SafeProviderDiagnostic               `json:"provider,omitempty"`
	AgentRun       *SafeAgentRunDiagnostic               `json:"agent_run,omitempty"`
	Invocation     *workerexecution.InvocationDiagnostic `json:"invocation,omitempty"`
}

// SafeRenderedPromptDiagnostic carries prompt hashes and allowlisted variables.
type SafeRenderedPromptDiagnostic struct {
	SystemPromptHash string            `json:"system_prompt_hash,omitempty"`
	UserMessageHash  string            `json:"user_message_hash,omitempty"`
	Variables        map[string]string `json:"variables,omitempty"`
}

// SafeProviderDiagnostic carries allowlisted provider execution metadata.
type SafeProviderDiagnostic struct {
	Provider         string            `json:"provider,omitempty"`
	Model            string            `json:"model,omitempty"`
	RequestMetadata  map[string]string `json:"request_metadata,omitempty"`
	ResponseMetadata map[string]string `json:"response_metadata,omitempty"`
}

// CloneSafeWorkDiagnostics returns a detached safe diagnostics snapshot.
func CloneSafeWorkDiagnostics(diagnostics *SafeWorkDiagnostics) *SafeWorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	return &SafeWorkDiagnostics{
		RenderedPrompt: cloneSafeRenderedPromptDiagnostic(diagnostics.RenderedPrompt),
		Provider:       cloneSafeProviderDiagnostic(diagnostics.Provider),
		AgentRun:       cloneSafeAgentRunDiagnostic(diagnostics.AgentRun),
		Invocation:     workerexecution.CloneInvocationDiagnostic(diagnostics.Invocation),
	}
}

func cloneSafeRenderedPromptDiagnostic(diagnostic *SafeRenderedPromptDiagnostic) *SafeRenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeRenderedPromptDiagnostic{SystemPromptHash: diagnostic.SystemPromptHash, UserMessageHash: diagnostic.UserMessageHash, Variables: cloneStringMap(diagnostic.Variables)}
}

func cloneSafeProviderDiagnostic(diagnostic *SafeProviderDiagnostic) *SafeProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeProviderDiagnostic{Provider: diagnostic.Provider, Model: diagnostic.Model, RequestMetadata: cloneStringMap(diagnostic.RequestMetadata), ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata)}
}

// SafeWorkDiagnosticsFromWorkDiagnostics projects worker-internal diagnostics
// onto the canonical safe diagnostics boundary.
func SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics *workerexecution.WorkDiagnostics) *SafeWorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	out := &SafeWorkDiagnostics{
		RenderedPrompt: safeRenderedPromptDiagnosticFromWorkDiagnostics(diagnostics.RenderedPrompt),
		Provider:       safeProviderDiagnosticFromWorkDiagnostics(diagnostics.Provider),
		AgentRun:       SafeAgentRunDiagnosticFromWorkDiagnostics(diagnostics),
		Invocation:     cloneInvocationDiagnostic(diagnostics.Invocation),
	}
	if out.RenderedPrompt == nil && out.Provider == nil && out.AgentRun == nil && out.Invocation == nil {
		return nil
	}
	return out
}

// WorkDiagnosticsFromSafeEventPayload decodes the public camel-case event shape
// into canonical worker diagnostics without involving generated transport types.
func WorkDiagnosticsFromSafeEventPayload(payload json.RawMessage) (*workerexecution.WorkDiagnostics, error) {
	safe, err := SafeWorkDiagnosticsFromEventPayload(payload)
	if err != nil {
		return nil, err
	}
	return WorkDiagnosticsFromSafeWorkDiagnostics(safe), nil
}

// SafeWorkDiagnosticsFromEventPayload decodes the public camel-case event
// shape into the worker-owned safe diagnostics contract.
func SafeWorkDiagnosticsFromEventPayload(payload json.RawMessage) (*SafeWorkDiagnostics, error) {
	if len(payload) == 0 || string(payload) == "null" {
		return nil, nil
	}
	var eventFields any
	if err := json.Unmarshal(payload, &eventFields); err != nil {
		return nil, fmt.Errorf("decode safe inference diagnostics: %w", err)
	}
	normalizeSafeEventFieldNames(eventFields)
	normalized, err := json.Marshal(eventFields)
	if err != nil {
		return nil, fmt.Errorf("normalize safe inference diagnostics: %w", err)
	}
	var safe SafeWorkDiagnostics
	if err := json.Unmarshal(normalized, &safe); err != nil {
		return nil, fmt.Errorf("decode normalized safe inference diagnostics: %w", err)
	}
	return &safe, nil
}

// SafeWorkDiagnosticsEventPayload encodes canonical safe diagnostics with the
// camel-case field names retained by Factory event and public API payloads.
// Metadata map keys are caller-owned and are not rewritten.
func SafeWorkDiagnosticsEventPayload(diagnostics *SafeWorkDiagnostics) (json.RawMessage, error) {
	if diagnostics == nil {
		return json.RawMessage("null"), nil
	}
	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		return nil, fmt.Errorf("encode safe inference diagnostics: %w", err)
	}
	var eventFields any
	if err := json.Unmarshal(encoded, &eventFields); err != nil {
		return nil, fmt.Errorf("decode encoded safe inference diagnostics: %w", err)
	}
	normalizeSafeCanonicalFieldNames(eventFields)
	encoded, err = json.Marshal(eventFields)
	if err != nil {
		return nil, fmt.Errorf("encode safe inference event diagnostics: %w", err)
	}
	return encoded, nil
}

func normalizeSafeEventFieldNames(value any) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			normalizeSafeEventFieldNames(item)
		}
	case map[string]any:
		for key, item := range typed {
			canonical := key
			if target := canonicalSafeEventFieldName(key); target != key {
				delete(typed, key)
				typed[target] = item
				canonical = target
			}
			if canonical == "variables" || canonical == "request_metadata" || canonical == "response_metadata" {
				continue
			}
			normalizeSafeEventFieldNames(item)
		}
	}
}

func normalizeSafeCanonicalFieldNames(value any) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			normalizeSafeCanonicalFieldNames(item)
		}
	case map[string]any:
		for key, item := range typed {
			eventName := key
			if target := eventSafeFieldName(key); target != key {
				delete(typed, key)
				typed[target] = item
				eventName = target
			}
			if eventName == "variables" || eventName == "requestMetadata" || eventName == "responseMetadata" {
				continue
			}
			normalizeSafeCanonicalFieldNames(item)
		}
	}
}

func eventSafeFieldName(field string) string {
	for eventName, canonicalName := range safeEventFieldNames() {
		if canonicalName == field {
			return eventName
		}
	}
	return field
}

func canonicalSafeEventFieldName(field string) string {
	if canonical, ok := safeEventFieldNames()[field]; ok {
		return canonical
	}
	return field
}

func safeEventFieldNames() map[string]string {
	return map[string]string{
		"agentRun":          "agent_run",
		"executionBehavior": "execution_behavior",
		"failureClass":      "failure_class",
		"recoveryAction":    "recovery_action",
		"renderedPrompt":    "rendered_prompt",
		"requestMetadata":   "request_metadata",
		"responseMetadata":  "response_metadata",
		"signatureHash":     "signature_hash",
		"sourceKinds":       "source_kinds",
		"systemPromptHash":  "system_prompt_hash",
		"toolCallCount":     "tool_call_count",
		"toolDiagnostics":   "tool_diagnostics",
		"toolName":          "tool_name",
		"toolPolicy":        "tool_policy",
		"userMessageHash":   "user_message_hash",
		"valueCount":        "value_count",
	}
}

// WorkDiagnosticsFromSafeWorkDiagnostics rehydrates worker-facing diagnostics
// from the canonical safe diagnostics boundary.
func WorkDiagnosticsFromSafeWorkDiagnostics(diagnostics *SafeWorkDiagnostics) *workerexecution.WorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	out := &workerexecution.WorkDiagnostics{
		RenderedPrompt: renderedPromptDiagnosticFromSafeWorkDiagnostics(diagnostics.RenderedPrompt),
		Provider:       providerDiagnosticFromSafeWorkDiagnostics(diagnostics.Provider),
		Invocation:     cloneInvocationDiagnostic(diagnostics.Invocation),
	}
	if agentRun := diagnostics.AgentRun; agentRun != nil {
		out.Metadata = agentRunMetadataFromSafeDiagnostic(agentRun)
	}
	if out.RenderedPrompt == nil && out.Provider == nil && out.Invocation == nil && len(out.Metadata) == 0 {
		return nil
	}
	return out
}

func safeRenderedPromptDiagnosticFromWorkDiagnostics(diagnostic *workerexecution.RenderedPromptDiagnostic) *SafeRenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeRenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        safeRenderedPromptVariables(diagnostic.Variables),
	}
}

func renderedPromptDiagnosticFromSafeWorkDiagnostics(diagnostic *SafeRenderedPromptDiagnostic) *workerexecution.RenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &workerexecution.RenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        cloneStringMap(diagnostic.Variables),
	}
}

func safeProviderDiagnosticFromWorkDiagnostics(diagnostic *workerexecution.ProviderDiagnostic) *SafeProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeProviderDiagnostic{
		Provider:         diagnostic.Provider,
		Model:            diagnostic.Model,
		RequestMetadata:  safeDiagnosticMetadata(diagnostic.RequestMetadata),
		ResponseMetadata: safeDiagnosticMetadata(diagnostic.ResponseMetadata),
	}
}

func providerDiagnosticFromSafeWorkDiagnostics(diagnostic *SafeProviderDiagnostic) *workerexecution.ProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &workerexecution.ProviderDiagnostic{
		Provider:         diagnostic.Provider,
		Model:            diagnostic.Model,
		RequestMetadata:  cloneStringMap(diagnostic.RequestMetadata),
		ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata),
	}
}

func safeRenderedPromptVariables(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		switch strings.ToLower(key) {
		case "prompt_source", "promptsource",
			"request_id", "requestid",
			"trace_id", "traceid",
			"work_id", "workid",
			"work_type", "worktype",
			"work_type_id", "worktypeid",
			"work_type_name", "worktypename",
			"worker_type", "workertype",
			"workstation_type", "workstationtype":
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func safeDiagnosticMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		if isSafeProviderMetadataKey(key) {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isSafeProviderMetadataKey(key string) bool {
	switch strings.ToLower(key) {
	case "content_bytes",
		"opencode_agent",
		"output_schema",
		"prompt_source",
		"provider_session_id",
		"provider_session_kind",
		"provider_session_provider",
		"request_id",
		"retry_count",
		"session_id",
		"source",
		"stderr_excerpt",
		"stdout_excerpt",
		"worker_type",
		"workstation_type",
		"working_directory",
		"worktree":
		return true
	default:
		return false
	}
}

func cloneInvocationDiagnostic(diagnostic *workerexecution.InvocationDiagnostic) *workerexecution.InvocationDiagnostic {
	if diagnostic == nil {
		return nil
	}
	clone := &workerexecution.InvocationDiagnostic{
		SignatureHash: diagnostic.SignatureHash,
	}
	if len(diagnostic.Parameters) > 0 {
		clone.Parameters = make([]workerexecution.InvocationParameterDiagnostic, len(diagnostic.Parameters))
		for i, parameter := range diagnostic.Parameters {
			clone.Parameters[i] = workerexecution.InvocationParameterDiagnostic{
				Name:        parameter.Name,
				SourceKinds: cloneStringSlice(parameter.SourceKinds),
				ValueCount:  parameter.ValueCount,
				Redacted:    parameter.Redacted,
			}
		}
	}
	return clone
}

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
	ExecutionBehavior string                    `json:"execution_behavior,omitempty"`
	FailureClass      string                    `json:"failure_class,omitempty"`
	RecoveryAction    string                    `json:"recovery_action,omitempty"`
	ToolPolicy        string                    `json:"tool_policy,omitempty"`
	ToolCallCount     int                       `json:"tool_call_count,omitempty"`
	ToolDiagnostics   []AgentRunToolDiagnostic  `json:"tool_diagnostics,omitempty"`
	Transcript        []AgentRunTranscriptEntry `json:"transcript,omitempty"`
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
func SafeAgentRunDiagnosticFromWorkDiagnostics(diagnostics *workerexecution.WorkDiagnostics) *SafeAgentRunDiagnostic {
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

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
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
