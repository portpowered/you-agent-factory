// Package workerdiagnostics maps worker-owned safe diagnostics to and from the
// generated HTTP contract.
package workerdiagnostics

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	domaindiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// SafeWorkDiagnosticsFromGenerated converts generated safe diagnostics to the
// canonical worker-owned boundary.
func SafeWorkDiagnosticsFromGenerated(diagnostics *factoryapi.SafeWorkDiagnostics) *domaindiagnostics.SafeWorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	out := &domaindiagnostics.SafeWorkDiagnostics{
		RenderedPrompt: renderedPromptFromGenerated(diagnostics.RenderedPrompt),
		Provider:       providerFromGenerated(diagnostics.Provider),
		AgentRun:       SafeAgentRunDiagnosticFromGenerated(diagnostics.AgentRun),
		Invocation:     invocationFromGenerated(diagnostics.Invocation),
	}
	if out.RenderedPrompt == nil && out.Provider == nil && out.AgentRun == nil && out.Invocation == nil {
		return nil
	}
	return out
}

// GeneratedSafeWorkDiagnostics converts canonical worker diagnostics to the
// generated event contract.
func GeneratedSafeWorkDiagnostics(diagnostics *domaindiagnostics.SafeWorkDiagnostics) *factoryapi.SafeWorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	out := &factoryapi.SafeWorkDiagnostics{
		RenderedPrompt: generatedRenderedPrompt(diagnostics.RenderedPrompt),
		Provider:       generatedProvider(diagnostics.Provider),
		AgentRun:       GeneratedSafeAgentRunDiagnostic(diagnostics.AgentRun),
		Invocation:     generatedInvocation(diagnostics.Invocation),
	}
	if out.RenderedPrompt == nil && out.Provider == nil && out.AgentRun == nil && out.Invocation == nil {
		return nil
	}
	return out
}

// GeneratedSafeWorkDiagnosticsFromWorkDiagnostics projects worker-internal
// diagnostics through the canonical safe boundary into the generated contract.
func GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(diagnostics *workerexecution.WorkDiagnostics) *factoryapi.SafeWorkDiagnostics {
	return GeneratedSafeWorkDiagnostics(domaindiagnostics.SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics))
}

// GeneratedWorkFailureMetadata maps canonical failure metadata to the public contract.
func GeneratedWorkFailureMetadata(failure *workerexecution.WorkFailureMetadata) *factoryapi.ProviderFailureMetadata {
	if failure == nil {
		return nil
	}
	family := factoryapi.WorkFailureFamily(failure.Family)
	failureType := factoryapi.WorkFailureType(failure.Type)
	return &factoryapi.ProviderFailureMetadata{Family: &family, Type: &failureType}
}

// WorkFailureMetadataFromGenerated maps public failure metadata to the canonical contract.
func WorkFailureMetadataFromGenerated(failure *factoryapi.ProviderFailureMetadata) *workerexecution.WorkFailureMetadata {
	if failure == nil {
		return nil
	}
	return &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamily(enumStringValue(failure.Family)),
		Type:   workerexecution.WorkFailureType(enumStringValue(failure.Type)),
	}
}

// GeneratedProviderSessionMetadata maps canonical provider-session metadata to the public contract.
func GeneratedProviderSessionMetadata(session *workerexecution.ProviderSessionMetadata) *factoryapi.ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	return &factoryapi.ProviderSessionMetadata{
		Provider: stringPtrIfNotEmpty(workerexecution.CanonicalProviderSessionProvider(session.Provider)),
		Kind:     stringPtrIfNotEmpty(session.Kind),
		Id:       stringPtrIfNotEmpty(session.ID),
	}
}

// ProviderSessionMetadataFromGenerated maps public provider-session metadata to the canonical contract.
func ProviderSessionMetadataFromGenerated(session *factoryapi.ProviderSessionMetadata) *workerexecution.ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	return &workerexecution.ProviderSessionMetadata{
		Provider: workerexecution.CanonicalProviderSessionProvider(stringValue(session.Provider)),
		Kind:     stringValue(session.Kind),
		ID:       stringValue(session.Id),
	}
}

func renderedPromptFromGenerated(diagnostic *factoryapi.RenderedPromptDiagnostic) *domaindiagnostics.SafeRenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &domaindiagnostics.SafeRenderedPromptDiagnostic{
		SystemPromptHash: stringValue(diagnostic.SystemPromptHash),
		UserMessageHash:  stringValue(diagnostic.UserMessageHash),
		Variables:        stringMapValue(diagnostic.Variables),
	}
}

func generatedRenderedPrompt(diagnostic *domaindiagnostics.SafeRenderedPromptDiagnostic) *factoryapi.RenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &factoryapi.RenderedPromptDiagnostic{
		SystemPromptHash: stringPtrIfNotEmpty(diagnostic.SystemPromptHash),
		UserMessageHash:  stringPtrIfNotEmpty(diagnostic.UserMessageHash),
		Variables:        stringMapPtr(diagnostic.Variables),
	}
}

func providerFromGenerated(diagnostic *factoryapi.ProviderDiagnostic) *domaindiagnostics.SafeProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &domaindiagnostics.SafeProviderDiagnostic{
		Provider:         stringValue(diagnostic.Provider),
		Model:            stringValue(diagnostic.Model),
		RequestMetadata:  stringMapValue(diagnostic.RequestMetadata),
		ResponseMetadata: stringMapValue(diagnostic.ResponseMetadata),
	}
}

func generatedProvider(diagnostic *domaindiagnostics.SafeProviderDiagnostic) *factoryapi.ProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &factoryapi.ProviderDiagnostic{
		Provider:         stringPtrIfNotEmpty(diagnostic.Provider),
		Model:            stringPtrIfNotEmpty(diagnostic.Model),
		RequestMetadata:  stringMapPtr(diagnostic.RequestMetadata),
		ResponseMetadata: stringMapPtr(diagnostic.ResponseMetadata),
	}
}

func invocationFromGenerated(diagnostic *factoryapi.InvocationDiagnostic) *workerexecution.InvocationDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &workerexecution.InvocationDiagnostic{SignatureHash: stringValue(diagnostic.SignatureHash)}
	if diagnostic.Parameters != nil && len(*diagnostic.Parameters) > 0 {
		out.Parameters = make([]workerexecution.InvocationParameterDiagnostic, 0, len(*diagnostic.Parameters))
		for _, parameter := range *diagnostic.Parameters {
			out.Parameters = append(out.Parameters, workerexecution.InvocationParameterDiagnostic{
				Name:        stringValue(parameter.Name),
				SourceKinds: append([]string(nil), stringSliceValue(parameter.SourceKinds)...),
				ValueCount:  int(int64Value(parameter.ValueCount)),
				Redacted:    boolValue(parameter.Redacted),
			})
		}
	}
	if out.SignatureHash == "" && len(out.Parameters) == 0 {
		return nil
	}
	return out
}

func generatedInvocation(diagnostic *workerexecution.InvocationDiagnostic) *factoryapi.InvocationDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &factoryapi.InvocationDiagnostic{SignatureHash: stringPtrIfNotEmpty(diagnostic.SignatureHash)}
	if len(diagnostic.Parameters) > 0 {
		parameters := make([]factoryapi.InvocationParameterDiagnostic, 0, len(diagnostic.Parameters))
		for _, parameter := range diagnostic.Parameters {
			parameters = append(parameters, factoryapi.InvocationParameterDiagnostic{
				Name:        stringPtrIfNotEmpty(parameter.Name),
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

// GeneratedSafeAgentRunDiagnostic maps canonical agent-run diagnostics to the public contract.
func GeneratedSafeAgentRunDiagnostic(diagnostic *domaindiagnostics.SafeAgentRunDiagnostic) *factoryapi.SafeAgentRunDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &factoryapi.SafeAgentRunDiagnostic{
		FailureClass:   stringPtrIfNotEmpty(diagnostic.FailureClass),
		RecoveryAction: stringPtrIfNotEmpty(diagnostic.RecoveryAction),
		ToolPolicy:     stringPtrIfNotEmpty(diagnostic.ToolPolicy),
	}
	if diagnostic.ExecutionBehavior != "" {
		behavior := factoryapi.SafeAgentRunDiagnosticExecutionBehavior(diagnostic.ExecutionBehavior)
		out.ExecutionBehavior = &behavior
	}
	if diagnostic.ToolCallCount > 0 {
		count := int32(diagnostic.ToolCallCount)
		out.ToolCallCount = &count
	}
	out.ToolDiagnostics = generatedToolDiagnostics(diagnostic.ToolDiagnostics)
	out.Transcript = generatedTranscript(diagnostic.Transcript)
	if out.ExecutionBehavior == nil && out.FailureClass == nil && out.RecoveryAction == nil &&
		out.ToolPolicy == nil && out.ToolCallCount == nil && out.ToolDiagnostics == nil && out.Transcript == nil {
		return nil
	}
	return out
}

// SafeAgentRunDiagnosticFromGenerated maps public agent-run diagnostics to the canonical contract.
func SafeAgentRunDiagnosticFromGenerated(diagnostic *factoryapi.SafeAgentRunDiagnostic) *domaindiagnostics.SafeAgentRunDiagnostic {
	if diagnostic == nil {
		return nil
	}
	out := &domaindiagnostics.SafeAgentRunDiagnostic{
		ExecutionBehavior: enumStringValue(diagnostic.ExecutionBehavior),
		FailureClass:      stringValue(diagnostic.FailureClass),
		RecoveryAction:    stringValue(diagnostic.RecoveryAction),
		ToolPolicy:        stringValue(diagnostic.ToolPolicy),
	}
	if diagnostic.ToolCallCount != nil {
		out.ToolCallCount = int(*diagnostic.ToolCallCount)
	}
	if diagnostic.ToolDiagnostics != nil {
		out.ToolDiagnostics = toolDiagnosticsFromGenerated(*diagnostic.ToolDiagnostics)
	}
	if diagnostic.Transcript != nil {
		out.Transcript = transcriptFromGenerated(*diagnostic.Transcript)
	}
	if out.ExecutionBehavior == "" && out.FailureClass == "" && out.RecoveryAction == "" &&
		out.ToolPolicy == "" && out.ToolCallCount == 0 && len(out.ToolDiagnostics) == 0 && len(out.Transcript) == 0 {
		return nil
	}
	return out
}

// GeneratedFactoryWorldAgentRunInspectionView maps canonical agent-run diagnostics to the world-view contract.
func GeneratedFactoryWorldAgentRunInspectionView(diagnostic *domaindiagnostics.SafeAgentRunDiagnostic) *factoryapi.FactoryWorldAgentRunInspectionView {
	if diagnostic == nil {
		return nil
	}
	out := &factoryapi.FactoryWorldAgentRunInspectionView{
		ExecutionBehavior: stringPtrIfNotEmpty(diagnostic.ExecutionBehavior),
		FailureClass:      stringPtrIfNotEmpty(diagnostic.FailureClass),
		RecoveryAction:    stringPtrIfNotEmpty(diagnostic.RecoveryAction),
		ToolPolicy:        stringPtrIfNotEmpty(diagnostic.ToolPolicy),
	}
	if diagnostic.ToolCallCount > 0 {
		count := int32(diagnostic.ToolCallCount)
		out.ToolCallCount = &count
	}
	out.ToolDiagnostics = generatedToolDiagnostics(diagnostic.ToolDiagnostics)
	out.Transcript = generatedTranscript(diagnostic.Transcript)
	if out.ExecutionBehavior == nil && out.FailureClass == nil && out.RecoveryAction == nil &&
		out.ToolPolicy == nil && out.ToolCallCount == nil && out.ToolDiagnostics == nil && out.Transcript == nil {
		return nil
	}
	return out
}

func generatedToolDiagnostics(entries []domaindiagnostics.AgentRunToolDiagnostic) *[]factoryapi.AgentRunToolDiagnosticEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]factoryapi.AgentRunToolDiagnosticEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, factoryapi.AgentRunToolDiagnosticEntry{ToolName: stringPtrIfNotEmpty(entry.ToolName), Phase: stringPtrIfNotEmpty(entry.Phase), Detail: stringPtrIfNotEmpty(entry.Detail)})
	}
	return &out
}

func generatedTranscript(entries []domaindiagnostics.AgentRunTranscriptEntry) *[]factoryapi.AgentRunTranscriptEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]factoryapi.AgentRunTranscriptEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, factoryapi.AgentRunTranscriptEntry{Role: stringPtrIfNotEmpty(entry.Role), Summary: stringPtrIfNotEmpty(entry.Summary)})
	}
	return &out
}

func toolDiagnosticsFromGenerated(entries []factoryapi.AgentRunToolDiagnosticEntry) []domaindiagnostics.AgentRunToolDiagnostic {
	if len(entries) == 0 {
		return nil
	}
	out := make([]domaindiagnostics.AgentRunToolDiagnostic, 0, len(entries))
	for _, entry := range entries {
		out = append(out, domaindiagnostics.AgentRunToolDiagnostic{ToolName: stringValue(entry.ToolName), Phase: stringValue(entry.Phase), Detail: stringValue(entry.Detail)})
	}
	return out
}

func transcriptFromGenerated(entries []factoryapi.AgentRunTranscriptEntry) []domaindiagnostics.AgentRunTranscriptEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]domaindiagnostics.AgentRunTranscriptEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, domaindiagnostics.AgentRunTranscriptEntry{Role: stringValue(entry.Role), Summary: stringValue(entry.Summary)})
	}
	return out
}

func stringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	out := factoryapi.StringMap(cloneStringMap(values))
	return &out
}

func stringMapValue(values *factoryapi.StringMap) map[string]string {
	if values == nil || len(*values) == 0 {
		return nil
	}
	return cloneStringMap(map[string]string(*values))
}

func cloneStringMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func enumStringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func boolPtr(value bool) *bool { return &value }
func boolValue(value *bool) bool {
	return value != nil && *value
}
func int64Ptr(value int64) *int64 { return &value }
func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
func stringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	return &out
}
func stringSliceValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}
