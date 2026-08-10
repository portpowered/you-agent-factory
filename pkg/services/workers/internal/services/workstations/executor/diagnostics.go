package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
)

func workDiagnosticsForInferenceRequest(req workerexecution.ProviderInferenceRequest) *workerexecution.WorkDiagnostics {
	requestMetadata := map[string]string{
		"dispatch_id":       req.Dispatch.DispatchID,
		"worker_type":       firstNonEmpty(req.WorkerType, req.Dispatch.WorkerType),
		"workstation_type":  req.WorkstationType,
		"worktree":          req.Worktree,
		"working_directory": req.WorkingDirectory,
		"session_id":        req.SessionID,
		"output_schema":     req.OutputSchema,
	}
	return &workerexecution.WorkDiagnostics{
		RenderedPrompt: &workerexecution.RenderedPromptDiagnostic{
			SystemPromptHash: hashText(req.SystemPrompt),
			UserMessageHash:  hashText(req.UserMessage),
		},
		Provider: &workerexecution.ProviderDiagnostic{
			Provider:        req.ModelProvider,
			Model:           req.Model,
			RequestMetadata: requestMetadata,
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func withInferenceResponseDiagnostics(base *workerexecution.WorkDiagnostics, resp workerexecution.InferenceResponse, retryCount int) *workerexecution.WorkDiagnostics {
	diagnostics := workerexecution.CloneWorkDiagnostics(base)
	diagnostics = mergeWorkDiagnostics(diagnostics, resp.Diagnostics)
	if diagnostics == nil {
		diagnostics = &workerexecution.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &workerexecution.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string)
	}
	diagnostics.Provider.ResponseMetadata["content_bytes"] = fmt.Sprintf("%d", len(resp.Content))
	diagnostics.Provider.ResponseMetadata["retry_count"] = fmt.Sprintf("%d", retryCount)
	if resp.ProviderSession != nil {
		diagnostics.Provider.ResponseMetadata["provider_session_provider"] = workerexecution.CanonicalProviderSessionProvider(resp.ProviderSession.Provider)
		diagnostics.Provider.ResponseMetadata["provider_session_kind"] = resp.ProviderSession.Kind
		diagnostics.Provider.ResponseMetadata["provider_session_id"] = resp.ProviderSession.ID
	}
	return diagnostics
}

func withInferenceErrorDiagnostics(base *workerexecution.WorkDiagnostics, err error, retryCount int) *workerexecution.WorkDiagnostics {
	diagnostics := workerexecution.CloneWorkDiagnostics(base)
	if diagnostics == nil {
		diagnostics = &workerexecution.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &workerexecution.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string)
	}
	setSafeFailureMetadata(diagnostics, err)
	diagnostics.Provider.ResponseMetadata["retry_count"] = fmt.Sprintf("%d", retryCount)
	return diagnostics
}

func completionValidationDiagnostics(base *workerexecution.WorkDiagnostics, classification string) *workerexecution.WorkDiagnostics {
	diagnostics := workerexecution.CloneWorkDiagnostics(base)
	if diagnostics == nil {
		diagnostics = &workerexecution.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &workerexecution.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string)
	}
	diagnostics.Provider.ResponseMetadata[workerexecution.ProviderResponseMetadataFailureFamily] = string(workerexecution.WorkFailureFamilyTerminal)
	diagnostics.Provider.ResponseMetadata[workerexecution.ProviderResponseMetadataFailureType] = string(workerexecution.WorkFailureTypeUnknown)
	diagnostics.Provider.ResponseMetadata[workerexecution.ProviderResponseMetadataFailureOperation] = "completion_validation"
	diagnostics.Provider.ResponseMetadata[workerexecution.ProviderResponseMetadataFailureClassification] = classification
	return diagnostics
}

func setSafeFailureMetadata(diagnostics *workerexecution.WorkDiagnostics, err error) {
	if diagnostics == nil || diagnostics.Provider == nil {
		return
	}
	metadata := diagnostics.Provider.ResponseMetadata
	metadata[workerexecution.ProviderResponseMetadataFailureOperation] = "provider_inference"
	providerErr := workerexecution.NormalizeProviderExecutionError(err)
	if providerErr != nil {
		metadata[workerexecution.ProviderResponseMetadataFailureFamily] = string(providerErr.Family)
		metadata[workerexecution.ProviderResponseMetadataFailureType] = string(providerErr.Type)
		return
	}
	metadata[workerexecution.ProviderResponseMetadataFailureFamily] = string(workerexecution.WorkFailureFamilyTerminal)
	if errors.Is(err, context.DeadlineExceeded) {
		metadata[workerexecution.ProviderResponseMetadataFailureFamily] = string(workerexecution.WorkFailureFamilyRetryable)
		metadata[workerexecution.ProviderResponseMetadataFailureType] = string(workerexecution.WorkFailureTypeTimeout)
	} else if errors.Is(err, context.Canceled) {
		metadata[workerexecution.ProviderResponseMetadataFailureType] = string(workerexecution.WorkFailureTypeUnknown)
	} else {
		metadata[workerexecution.ProviderResponseMetadataFailureType] = string(workerexecution.WorkFailureTypeUnknown)
	}
}

func mergeWorkDiagnostics(base, overlay *workerexecution.WorkDiagnostics) *workerexecution.WorkDiagnostics {
	if base == nil {
		return workerexecution.CloneWorkDiagnostics(overlay)
	}
	if overlay == nil {
		return base
	}
	overlay = workerexecution.CloneWorkDiagnostics(overlay)
	if overlay.RenderedPrompt != nil {
		base.RenderedPrompt = overlay.RenderedPrompt
	}
	if overlay.Provider != nil {
		base.Provider = mergeProviderDiagnostic(base.Provider, overlay.Provider)
	}
	if overlay.Invocation != nil {
		base.Invocation = workerexecution.CloneWorkDiagnostics(&workerexecution.WorkDiagnostics{Invocation: overlay.Invocation}).Invocation
	}
	if overlay.Command != nil {
		base.Command = overlay.Command
	}
	if overlay.Panic != nil {
		base.Panic = overlay.Panic
	}
	if len(overlay.Metadata) > 0 {
		if base.Metadata == nil {
			base.Metadata = make(map[string]string, len(overlay.Metadata))
		}
		for k, v := range overlay.Metadata {
			base.Metadata[k] = v
		}
	}
	return base
}

func mergeProviderDiagnostic(base, overlay *workerexecution.ProviderDiagnostic) *workerexecution.ProviderDiagnostic {
	if base == nil {
		return overlay
	}
	if overlay.Provider != "" {
		base.Provider = overlay.Provider
	}
	if overlay.Model != "" {
		base.Model = overlay.Model
	}
	if len(overlay.RequestMetadata) > 0 {
		if base.RequestMetadata == nil {
			base.RequestMetadata = make(map[string]string, len(overlay.RequestMetadata))
		}
		for k, v := range overlay.RequestMetadata {
			base.RequestMetadata[k] = v
		}
	}
	if len(overlay.ResponseMetadata) > 0 {
		if base.ResponseMetadata == nil {
			base.ResponseMetadata = make(map[string]string, len(overlay.ResponseMetadata))
		}
		for k, v := range overlay.ResponseMetadata {
			base.ResponseMetadata[k] = v
		}
	}
	return base
}

func hashText(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

const (
	unsafeArtifactPatternReport       = "<invalid>"
	unrenderableArtifactPatternReport = "<unrenderable>"
)

func (we *WorkstationExecutor) verifyExpectedArtifacts(
	request workerexecution.WorkstationExecutionRequest,
	workstation *interfaces.FactoryWorkstationConfig,
	result workerexecution.WorkResult,
) workerexecution.WorkResult {
	if result.Outcome != workerexecution.OutcomeAccepted {
		return result
	}
	inputTokens := executionRequestInputTokens(request)
	declarations := we.expectedArtifactDeclarations(inputTokens, workstation)
	if len(declarations) == 0 {
		return result
	}
	entries := verifyExpectedArtifactDeclarations(
		request.WorkingDirectory,
		inputTokens,
		expectedArtifactContext(request),
		declarations,
		we.expectedArtifactFileSystem(),
	)
	if len(entries) == 0 {
		return result
	}
	verification := &workerexecution.ExpectedArtifactVerification{
		Code:    workerexecution.WorkFailureTypeExpectedArtifactsUnsatisfied,
		Entries: entries,
	}
	result.Outcome = workerexecution.OutcomeFailed
	result.ArtifactVerification = verification
	result.FailureMetadata = &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyTerminal,
		Type:   verification.Code,
	}
	result.Error = expectedArtifactVerificationError(verification)
	return result
}

func verifyExpectedArtifactDeclarations(
	workspace string,
	tokens []workerexecution.Token,
	context *workerexecution.Context,
	declarations []interfaces.ExpectedArtifactConfig,
	fileSystem platformfilesystem.GlobInspector,
) []workerexecution.ExpectedArtifactVerificationEntry {
	workspace = strings.TrimSpace(workspace)
	entries := make([]workerexecution.ExpectedArtifactVerificationEntry, 0, len(declarations))
	for declarationIndex, declaration := range declarations {
		pattern, renderErr := renderExpectedArtifactPattern(declaration.Pattern, tokens, context)
		if renderErr != nil {
			entries = append(entries, expectedArtifactFailureEntry(
				declarationIndex,
				declaration.Name,
				unrenderableArtifactPatternReport,
				workerexecution.ExpectedArtifactVerificationReasonMissing,
			))
			continue
		}
		pattern, safe := safeExpectedArtifactPattern(pattern)
		if !safe {
			entries = append(entries, expectedArtifactFailureEntry(
				declarationIndex,
				declaration.Name,
				unsafeArtifactPatternReport,
				workerexecution.ExpectedArtifactVerificationReasonMissing,
			))
			continue
		}
		reason, satisfied := expectedArtifactStatus(
			workspace,
			pattern,
			declaration.NonEmpty,
			fileSystem,
		)
		if !satisfied {
			entries = append(entries, expectedArtifactFailureEntry(declarationIndex, declaration.Name, pattern, reason))
		}
	}
	return entries
}

func expectedArtifactContext(request workerexecution.WorkstationExecutionRequest) *workerexecution.Context {
	if recorded := request.Dispatch.ExpectedArtifactContext; recorded != nil {
		return &workerexecution.Context{
			ProjectID: recorded.Project,
			SessionID: recorded.SessionID,
		}
	}
	return &workerexecution.Context{
		ProjectID: request.ProjectID,
		SessionID: request.FactorySessionID,
	}
}

func expectedArtifactFailureEntry(
	declarationIndex int,
	name string,
	pattern string,
	reason workerexecution.ExpectedArtifactVerificationReason,
) workerexecution.ExpectedArtifactVerificationEntry {
	return workerexecution.ExpectedArtifactVerificationEntry{
		DeclarationIndex: declarationIndex + 1,
		Name:             strings.TrimSpace(name),
		Pattern:          pattern,
		Reason:           reason,
	}
}

func renderExpectedArtifactPattern(
	pattern string,
	tokens []workerexecution.Token,
	context *workerexecution.Context,
) (string, error) {
	parsed, err := template.New("expected_artifact").Option("missingkey=error").Parse(pattern)
	if err != nil {
		return "", err
	}
	promptData := workerprompting.BuildPromptData(tokens, context)
	data := struct {
		Inputs  []workerprompting.TokenData
		Context struct {
			Project   string
			SessionID string
		}
	}{
		Inputs: promptData.Inputs,
		Context: struct {
			Project   string
			SessionID string
		}{
			Project:   promptData.Context.Project,
			SessionID: promptData.Context.SessionID,
		},
	}
	var rendered bytes.Buffer
	if err := parsed.Execute(&rendered, data); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func safeExpectedArtifactPattern(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false
	}
	portable := strings.ReplaceAll(trimmed, `\`, "/")
	if pathpkg.IsAbs(portable) || strings.HasPrefix(portable, "/") {
		return "", false
	}
	if len(portable) >= 2 && portable[1] == ':' && isASCIIAlpha(portable[0]) {
		return "", false
	}
	for _, segment := range strings.Split(portable, "/") {
		if segment == ".." {
			return "", false
		}
	}
	if _, err := pathpkg.Match(portable, ""); err != nil {
		return "", false
	}
	return portable, true
}

func expectedArtifactStatus(
	workspace string,
	pattern string,
	nonEmpty bool,
	fileSystem platformfilesystem.GlobInspector,
) (workerexecution.ExpectedArtifactVerificationReason, bool) {
	if fileSystem == nil || workspace == "" {
		return workerexecution.ExpectedArtifactVerificationReasonMissing, false
	}
	if !strings.ContainsAny(pattern, "*?[") {
		return expectedArtifactLiteralStatus(workspace, pattern, nonEmpty, fileSystem)
	}
	return expectedArtifactGlobStatus(workspace, pattern, nonEmpty, fileSystem)
}

func expectedArtifactLiteralStatus(
	workspace string,
	pattern string,
	nonEmpty bool,
	fileSystem platformfilesystem.GlobInspector,
) (workerexecution.ExpectedArtifactVerificationReason, bool) {
	candidate := filepath.Join(workspace, filepath.FromSlash(pattern))
	if !expectedArtifactPathWithinWorkspace(workspace, candidate, fileSystem) {
		return workerexecution.ExpectedArtifactVerificationReasonMissing, false
	}
	info, err := fileSystem.Stat(candidate)
	if err != nil || info == nil || !info.Mode().IsRegular() {
		return workerexecution.ExpectedArtifactVerificationReasonMissing, false
	}
	if nonEmpty && info.Size() == 0 {
		return workerexecution.ExpectedArtifactVerificationReasonEmpty, false
	}
	return "", true
}

func expectedArtifactGlobStatus(
	workspace string,
	pattern string,
	nonEmpty bool,
	fileSystem platformfilesystem.GlobInspector,
) (workerexecution.ExpectedArtifactVerificationReason, bool) {
	matches, err := fileSystem.Glob(filepath.Join(workspace, filepath.FromSlash(pattern)))
	if err != nil {
		return workerexecution.ExpectedArtifactVerificationReasonMissing, false
	}
	sort.Strings(matches)
	regularFiles := 0
	for _, match := range matches {
		if !expectedArtifactPathWithinWorkspace(workspace, match, fileSystem) {
			continue
		}
		info, statErr := fileSystem.Stat(match)
		if statErr != nil || info == nil || !info.Mode().IsRegular() {
			continue
		}
		regularFiles++
		if nonEmpty && info.Size() == 0 {
			return workerexecution.ExpectedArtifactVerificationReasonEmpty, false
		}
	}
	if regularFiles == 0 {
		return workerexecution.ExpectedArtifactVerificationReasonMissing, false
	}
	return "", true
}

func expectedArtifactPathWithinWorkspace(
	workspace string,
	candidate string,
	fileSystem platformfilesystem.GlobInspector,
) bool {
	if fileSystem == nil {
		return false
	}
	resolvedWorkspace, err := fileSystem.EvalSymlinks(workspace)
	if err != nil {
		return false
	}
	resolvedCandidate, err := fileSystem.EvalSymlinks(candidate)
	if err != nil {
		return false
	}
	return pathWithinWorkspace(filepath.Clean(resolvedWorkspace), filepath.Clean(resolvedCandidate))
}

func pathWithinWorkspace(workspace, candidate string) bool {
	relative, err := filepath.Rel(workspace, candidate)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func expectedArtifactVerificationError(
	verification *workerexecution.ExpectedArtifactVerification,
) string {
	if verification == nil || len(verification.Entries) == 0 {
		return string(workerexecution.WorkFailureTypeExpectedArtifactsUnsatisfied)
	}
	details := make([]string, 0, len(verification.Entries))
	for _, entry := range verification.Entries {
		details = append(details, fmt.Sprintf("%s=%s (%s)", entry.Name, entry.Pattern, entry.Reason))
	}
	return fmt.Sprintf("%s: %s", verification.Code, strings.Join(details, "; "))
}

func isASCIIAlpha(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}
