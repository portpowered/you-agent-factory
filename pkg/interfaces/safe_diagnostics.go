package interfaces

import (
	"strconv"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// SafeWorkDiagnostics carries the canonical dashboard-safe execution
// diagnostics surface used by event history, replay, and selected-tick
// projections.
type SafeWorkDiagnostics struct {
	RenderedPrompt *SafeRenderedPromptDiagnostic `json:"rendered_prompt,omitempty"`
	Provider       *SafeProviderDiagnostic       `json:"provider,omitempty"`
	AgentRun       *SafeAgentRunDiagnostic       `json:"agent_run,omitempty"`
	Invocation     *InvocationDiagnostic         `json:"invocation,omitempty"`
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

// SafeWorkDiagnosticsFromWorkDiagnostics projects worker-internal diagnostics
// onto the canonical safe diagnostics boundary.
func SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics *WorkDiagnostics) *SafeWorkDiagnostics {
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

// SafeWorkDiagnosticsFromGenerated converts the generated safe diagnostics
// contract into the canonical internal safe boundary.
func SafeWorkDiagnosticsFromGenerated(diagnostics *factoryapi.SafeWorkDiagnostics) *SafeWorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	out := &SafeWorkDiagnostics{
		RenderedPrompt: safeRenderedPromptDiagnosticFromGenerated(diagnostics.RenderedPrompt),
		Provider:       safeProviderDiagnosticFromGenerated(diagnostics.Provider),
		AgentRun:       SafeAgentRunDiagnosticFromGenerated(diagnostics.AgentRun),
		Invocation:     invocationDiagnosticFromGenerated(diagnostics.Invocation),
	}
	if out.RenderedPrompt == nil && out.Provider == nil && out.AgentRun == nil && out.Invocation == nil {
		return nil
	}
	return out
}

// GeneratedSafeWorkDiagnostics converts the canonical internal safe boundary
// into the generated event contract.
func GeneratedSafeWorkDiagnostics(diagnostics *SafeWorkDiagnostics) *factoryapi.SafeWorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	out := &factoryapi.SafeWorkDiagnostics{
		RenderedPrompt: generatedSafeRenderedPromptDiagnostic(diagnostics.RenderedPrompt),
		Provider:       generatedSafeProviderDiagnostic(diagnostics.Provider),
		AgentRun:       GeneratedSafeAgentRunDiagnostic(diagnostics.AgentRun),
		Invocation:     generatedInvocationDiagnostic(diagnostics.Invocation),
	}
	if out.RenderedPrompt == nil && out.Provider == nil && out.AgentRun == nil && out.Invocation == nil {
		return nil
	}
	return out
}

// GeneratedSafeWorkDiagnosticsFromWorkDiagnostics projects worker-internal
// diagnostics to the canonical safe boundary and then to the generated event
// contract.
func GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(diagnostics *WorkDiagnostics) *factoryapi.SafeWorkDiagnostics {
	return GeneratedSafeWorkDiagnostics(SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics))
}

// WorkDiagnosticsFromSafeWorkDiagnostics rehydrates worker-facing diagnostics
// from the canonical safe diagnostics boundary.
func WorkDiagnosticsFromSafeWorkDiagnostics(diagnostics *SafeWorkDiagnostics) *WorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	out := &WorkDiagnostics{
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

// GeneratedWorkFailureMetadata converts canonical work-failure metadata into
// the generated event contract.
func GeneratedWorkFailureMetadata(failure *WorkFailureMetadata) *factoryapi.ProviderFailureMetadata {
	if failure == nil {
		return nil
	}
	family := factoryapi.WorkFailureFamily(failure.Family)
	failureType := factoryapi.WorkFailureType(failure.Type)
	return &factoryapi.ProviderFailureMetadata{
		Family: &family,
		Type:   &failureType,
	}
}

// WorkFailureMetadataFromGenerated converts the generated provider-failure
// contract into canonical work-failure metadata.
func WorkFailureMetadataFromGenerated(failure *factoryapi.ProviderFailureMetadata) *WorkFailureMetadata {
	if failure == nil {
		return nil
	}
	return &WorkFailureMetadata{
		Family: WorkFailureFamily(safeDiagnosticsEnumStringValue(failure.Family)),
		Type:   WorkFailureType(safeDiagnosticsEnumStringValue(failure.Type)),
	}
}

// GeneratedProviderSessionMetadata converts canonical provider-session
// metadata into the generated event contract.
func GeneratedProviderSessionMetadata(session *ProviderSessionMetadata) *factoryapi.ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	return &factoryapi.ProviderSessionMetadata{
		Provider: safeDiagnosticsStringPtrIfNotEmpty(CanonicalProviderSessionProvider(session.Provider)),
		Kind:     safeDiagnosticsStringPtrIfNotEmpty(session.Kind),
		Id:       safeDiagnosticsStringPtrIfNotEmpty(session.ID),
	}
}

// ProviderSessionMetadataFromGenerated converts the generated provider-session
// contract into canonical provider-session metadata.
func ProviderSessionMetadataFromGenerated(session *factoryapi.ProviderSessionMetadata) *ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	return &ProviderSessionMetadata{
		Provider: CanonicalProviderSessionProvider(safeDiagnosticsStringValue(session.Provider)),
		Kind:     safeDiagnosticsStringValue(session.Kind),
		ID:       safeDiagnosticsStringValue(session.Id),
	}
}

func safeRenderedPromptDiagnosticFromWorkDiagnostics(diagnostic *RenderedPromptDiagnostic) *SafeRenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeRenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        safeRenderedPromptVariables(diagnostic.Variables),
	}
}

func safeRenderedPromptDiagnosticFromGenerated(diagnostic *factoryapi.RenderedPromptDiagnostic) *SafeRenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeRenderedPromptDiagnostic{
		SystemPromptHash: safeDiagnosticsStringValue(diagnostic.SystemPromptHash),
		UserMessageHash:  safeDiagnosticsStringValue(diagnostic.UserMessageHash),
		Variables:        safeDiagnosticsStringMapValue(diagnostic.Variables),
	}
}

func generatedSafeRenderedPromptDiagnostic(diagnostic *SafeRenderedPromptDiagnostic) *factoryapi.RenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &factoryapi.RenderedPromptDiagnostic{
		SystemPromptHash: safeDiagnosticsStringPtrIfNotEmpty(diagnostic.SystemPromptHash),
		UserMessageHash:  safeDiagnosticsStringPtrIfNotEmpty(diagnostic.UserMessageHash),
		Variables:        safeDiagnosticsStringMapPtr(diagnostic.Variables),
	}
}

func renderedPromptDiagnosticFromSafeWorkDiagnostics(diagnostic *SafeRenderedPromptDiagnostic) *RenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &RenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        cloneStringMap(diagnostic.Variables),
	}
}

func safeProviderDiagnosticFromWorkDiagnostics(diagnostic *ProviderDiagnostic) *SafeProviderDiagnostic {
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

func safeProviderDiagnosticFromGenerated(diagnostic *factoryapi.ProviderDiagnostic) *SafeProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeProviderDiagnostic{
		Provider:         safeDiagnosticsStringValue(diagnostic.Provider),
		Model:            safeDiagnosticsStringValue(diagnostic.Model),
		RequestMetadata:  safeDiagnosticsStringMapValue(diagnostic.RequestMetadata),
		ResponseMetadata: safeDiagnosticsStringMapValue(diagnostic.ResponseMetadata),
	}
}

func generatedSafeProviderDiagnostic(diagnostic *SafeProviderDiagnostic) *factoryapi.ProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &factoryapi.ProviderDiagnostic{
		Provider:         safeDiagnosticsStringPtrIfNotEmpty(diagnostic.Provider),
		Model:            safeDiagnosticsStringPtrIfNotEmpty(diagnostic.Model),
		RequestMetadata:  safeDiagnosticsStringMapPtr(diagnostic.RequestMetadata),
		ResponseMetadata: safeDiagnosticsStringMapPtr(diagnostic.ResponseMetadata),
	}
}

func providerDiagnosticFromSafeWorkDiagnostics(diagnostic *SafeProviderDiagnostic) *ProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &ProviderDiagnostic{
		Provider:         diagnostic.Provider,
		Model:            diagnostic.Model,
		RequestMetadata:  cloneStringMap(diagnostic.RequestMetadata),
		ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata),
	}
}

func invocationDiagnosticFromGenerated(diagnostic *factoryapi.InvocationDiagnostic) *InvocationDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &InvocationDiagnostic{
		SignatureHash: safeDiagnosticsStringValue(diagnostic.SignatureHash),
	}
	if diagnostic.Parameters != nil && len(*diagnostic.Parameters) > 0 {
		out.Parameters = make([]InvocationParameterDiagnostic, 0, len(*diagnostic.Parameters))
		for _, parameter := range *diagnostic.Parameters {
			out.Parameters = append(out.Parameters, InvocationParameterDiagnostic{
				Name:        safeDiagnosticsStringValue(parameter.Name),
				SourceKinds: cloneStringSlice(stringSlicePtrValue(parameter.SourceKinds)),
				ValueCount:  int(int64PtrValue(parameter.ValueCount)),
				Redacted:    boolPtrValue(parameter.Redacted),
			})
		}
	}
	if out.SignatureHash == "" && len(out.Parameters) == 0 {
		return nil
	}
	return out
}

func generatedInvocationDiagnostic(diagnostic *InvocationDiagnostic) *factoryapi.InvocationDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &factoryapi.InvocationDiagnostic{
		SignatureHash: safeDiagnosticsStringPtrIfNotEmpty(diagnostic.SignatureHash),
	}
	if len(diagnostic.Parameters) > 0 {
		parameters := make([]factoryapi.InvocationParameterDiagnostic, 0, len(diagnostic.Parameters))
		for _, parameter := range diagnostic.Parameters {
			parameters = append(parameters, factoryapi.InvocationParameterDiagnostic{
				Name:        safeDiagnosticsStringPtrIfNotEmpty(parameter.Name),
				SourceKinds: stringSlicePtr(parameter.SourceKinds),
				ValueCount:  int64Ptr(int64(parameter.ValueCount)),
				Redacted:    boolPtr(parameter.Redacted),
			})
		}
		out.Parameters = &parameters
	}
	if out.SignatureHash == nil && out.Parameters == nil {
		return nil
	}
	return out
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

func safeDiagnosticsStringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap(cloneStringMap(values))
	return &converted
}

func safeDiagnosticsStringMapValue(values *factoryapi.StringMap) map[string]string {
	if values == nil {
		return nil
	}
	cloned := cloneStringMap(map[string]string(*values))
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

func safeDiagnosticsStringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func safeDiagnosticsStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func safeDiagnosticsEnumStringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func boolPtr(value bool) *bool {
	return &value
}

func boolPtrValue(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func int64PtrValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func stringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	cloned := cloneStringSlice(values)
	return &cloned
}

func stringSlicePtrValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), (*values)...)
}

// CloneWorkDiagnostics returns a detached copy of canonical worker-facing
// diagnostics.
func CloneWorkDiagnostics(diagnostics *WorkDiagnostics) *WorkDiagnostics {
	if diagnostics == nil {
		return nil
	}

	clone := &WorkDiagnostics{
		RenderedPrompt: cloneRenderedPromptDiagnostic(diagnostics.RenderedPrompt),
		Provider:       cloneProviderDiagnostic(diagnostics.Provider),
		Invocation:     cloneInvocationDiagnostic(diagnostics.Invocation),
		Command:        cloneCommandDiagnostic(diagnostics.Command),
		Metadata:       cloneStringMap(diagnostics.Metadata),
	}
	if diagnostics.Panic != nil {
		clone.Panic = &PanicDiagnostic{
			Message: diagnostics.Panic.Message,
			Stack:   diagnostics.Panic.Stack,
		}
	}
	return clone
}

func cloneRenderedPromptDiagnostic(diagnostic *RenderedPromptDiagnostic) *RenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &RenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        cloneStringMap(diagnostic.Variables),
	}
}

func cloneProviderDiagnostic(diagnostic *ProviderDiagnostic) *ProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &ProviderDiagnostic{
		Provider:         diagnostic.Provider,
		Model:            diagnostic.Model,
		RequestMetadata:  cloneStringMap(diagnostic.RequestMetadata),
		ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata),
	}
}

func cloneInvocationDiagnostic(diagnostic *InvocationDiagnostic) *InvocationDiagnostic {
	if diagnostic == nil {
		return nil
	}
	clone := &InvocationDiagnostic{
		SignatureHash: diagnostic.SignatureHash,
	}
	if len(diagnostic.Parameters) > 0 {
		clone.Parameters = make([]InvocationParameterDiagnostic, len(diagnostic.Parameters))
		for i, parameter := range diagnostic.Parameters {
			clone.Parameters[i] = InvocationParameterDiagnostic{
				Name:        parameter.Name,
				SourceKinds: cloneStringSlice(parameter.SourceKinds),
				ValueCount:  parameter.ValueCount,
				Redacted:    parameter.Redacted,
			}
		}
	}
	return clone
}

func cloneCommandDiagnostic(diagnostic *CommandDiagnostic) *CommandDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &CommandDiagnostic{
		Command:    diagnostic.Command,
		Args:       cloneStringSlice(diagnostic.Args),
		Stdin:      diagnostic.Stdin,
		Env:        cloneStringMap(diagnostic.Env),
		Stdout:     diagnostic.Stdout,
		Stderr:     diagnostic.Stderr,
		ExitCode:   diagnostic.ExitCode,
		TimedOut:   diagnostic.TimedOut,
		Duration:   diagnostic.Duration,
		WorkingDir: diagnostic.WorkingDir,
	}
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
