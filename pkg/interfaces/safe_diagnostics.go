package interfaces

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// SafeWorkDiagnostics carries the canonical dashboard-safe execution
// diagnostics surface used by event history, replay, and selected-tick
// projections.
type SafeWorkDiagnostics struct {
	RenderedPrompt *SafeRenderedPromptDiagnostic `json:"rendered_prompt,omitempty"`
	Provider       *SafeProviderDiagnostic       `json:"provider,omitempty"`
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
	}
	if out.RenderedPrompt == nil && out.Provider == nil {
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
	}
	if out.RenderedPrompt == nil && out.Provider == nil {
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
	}
	if out.RenderedPrompt == nil && out.Provider == nil {
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

// GeneratedProviderFailureMetadata converts canonical provider-failure metadata
// into the generated event contract.
func GeneratedProviderFailureMetadata(failure *ProviderFailureMetadata) *factoryapi.ProviderFailureMetadata {
	if failure == nil {
		return nil
	}
	return &factoryapi.ProviderFailureMetadata{
		Family: safeDiagnosticsStringPtrIfNotEmpty(string(failure.Family)),
		Type:   safeDiagnosticsStringPtrIfNotEmpty(string(failure.Type)),
	}
}

// ProviderFailureMetadataFromGenerated converts the generated provider-failure
// contract into canonical provider-failure metadata.
func ProviderFailureMetadataFromGenerated(failure *factoryapi.ProviderFailureMetadata) *ProviderFailureMetadata {
	if failure == nil {
		return nil
	}
	return &ProviderFailureMetadata{
		Family: ProviderErrorFamily(safeDiagnosticsStringValue(failure.Family)),
		Type:   ProviderErrorType(safeDiagnosticsStringValue(failure.Type)),
	}
}

// GeneratedProviderSessionMetadata converts canonical provider-session
// metadata into the generated event contract.
func GeneratedProviderSessionMetadata(session *ProviderSessionMetadata) *factoryapi.ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	return &factoryapi.ProviderSessionMetadata{
		Provider: safeDiagnosticsStringPtrIfNotEmpty(session.Provider),
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
		Provider: safeDiagnosticsStringValue(session.Provider),
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
		"output_schema",
		"prompt_source",
		"provider_session_id",
		"provider_session_kind",
		"provider_session_provider",
		"request_id",
		"retry_count",
		"session_id",
		"source",
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
	converted := factoryapi.StringMap(cloneSafeDiagnosticsStringMap(values))
	return &converted
}

func safeDiagnosticsStringMapValue(values *factoryapi.StringMap) map[string]string {
	if values == nil || len(*values) == 0 {
		return nil
	}
	out := make(map[string]string, len(*values))
	for key, value := range *values {
		out[key] = value
	}
	return out
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

func cloneSafeDiagnosticsStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	clone := make(map[string]string, len(input))
	for key, value := range input {
		clone[key] = value
	}
	return clone
}
