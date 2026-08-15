package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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
	if session := providers.SessionMetadataFromContinuation(resp.Continuation); session != nil {
		diagnostics.Provider.ResponseMetadata["provider_session_provider"] = providers.CanonicalProviderSessionProvider(session.Provider)
		diagnostics.Provider.ResponseMetadata["provider_session_kind"] = session.Kind
		diagnostics.Provider.ResponseMetadata["provider_session_id"] = session.ID
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

func validateWorkstationCompletion(
	request workerexecution.WorkstationExecutionRequest,
	result workerexecution.WorkResult,
) workerexecution.WorkResult {
	if result.Outcome == workerexecution.OutcomeFailed && result.FailureMetadata != nil &&
		result.FailureMetadata.Type == workerexecution.WorkFailureTypeStructuredOutputSchemaViolation {
		result.Diagnostics = completionValidationDiagnostics(result.Diagnostics, "missing_required_output")
	}
	if result.Outcome != workerexecution.OutcomeAccepted || request.OutputContract == "" {
		return result
	}
	if contractErr := validateOutputContract(result.Output, request.OutputContract); contractErr != nil {
		result.Outcome = workerexecution.OutcomeFailed
		result.Error = "output contract failed: " + contractErr.Error()
		result.FailureMetadata = structuredOutputFailureMetadata()
		result.Diagnostics = completionValidationDiagnostics(result.Diagnostics, "missing_required_output")
	}
	return result
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
	context *work.ExpectedArtifactTemplateContext,
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

func expectedArtifactContext(request workerexecution.WorkstationExecutionRequest) *work.ExpectedArtifactTemplateContext {
	if recorded := request.Dispatch.ExpectedArtifactContext; recorded != nil {
		return recorded.Clone()
	}
	return &work.ExpectedArtifactTemplateContext{
		Project:   request.ProjectID,
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
	context *work.ExpectedArtifactTemplateContext,
) (string, error) {
	templateContext := work.ExpectedArtifactTemplateContext{}
	if context != nil {
		templateContext = *context
	}
	return templateContext.Render(pattern, expectedArtifactInputs(tokens, templateContext.Project))
}

func expectedArtifactInputs(tokens []workerexecution.Token, inputProject string) []work.ExpectedArtifactInput {
	if len(tokens) == 0 {
		return nil
	}
	inputProject = workerexecution.ResolveProjectID(inputProject)
	inputs := make([]work.ExpectedArtifactInput, 0, len(tokens))
	for _, token := range tokens {
		project := strings.TrimSpace(token.Color.Tags[workerexecution.ProjectTagKey])
		if project == "" {
			project = inputProject
		} else {
			project = workerexecution.ResolveProjectID(project)
		}
		inputs = append(inputs, work.ExpectedArtifactInput{
			Name:       token.Color.Name,
			WorkID:     token.Color.WorkID,
			WorkTypeID: token.Color.WorkTypeID,
			DataType:   string(token.Color.DataType),
			TraceID:    token.Color.TraceID,
			ParentID:   token.Color.ParentID,
			Project:    project,
			Tags:       work.CloneTags(token.Color.Tags),
			Payload:    string(token.Color.Payload),
		})
	}
	return inputs
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

func (we *WorkstationExecutor) expectedArtifactDeclarations(
	tokens []workerexecution.Token,
	workstation *interfaces.FactoryWorkstationConfig,
) []interfaces.ExpectedArtifactConfig {
	if workstation == nil {
		return nil
	}
	var declarations []interfaces.ExpectedArtifactConfig
	if we != nil && we.RuntimeConfig != nil {
		if factory := we.RuntimeConfig.FactoryConfig(); factory != nil {
			seenWorkTypes := make(map[string]struct{})
			for _, token := range tokens {
				if token.Color.DataType == workerexecution.DataTypeResource {
					continue
				}
				workTypeID := strings.TrimSpace(token.Color.WorkTypeID)
				if workTypeID == "" {
					continue
				}
				if _, seen := seenWorkTypes[workTypeID]; seen {
					continue
				}
				seenWorkTypes[workTypeID] = struct{}{}
				for _, workType := range factory.WorkTypes {
					if workType.ID == workTypeID || workType.Name == workTypeID {
						declarations = append(declarations, workType.ExpectedArtifacts...)
						break
					}
				}
			}
		}
	}
	declarations = append(declarations, workstation.ExpectedArtifacts...)
	return interfaces.NormalizeExpectedArtifactConfigs(declarations)
}

func (we *WorkstationExecutor) expectedArtifactFileSystem() platformfilesystem.GlobInspector {
	if we == nil {
		return nil
	}
	if we.ArtifactFileSystem != nil {
		return we.ArtifactFileSystem
	}
	inspector, _ := we.FileSystem.(platformfilesystem.GlobInspector)
	return inspector
}

func workstationWorkerName(workstationDef *interfaces.FactoryWorkstationConfig, dispatch work.WorkDispatch) string {
	if workstationDef != nil && workstationDef.WorkerTypeName != "" {
		return workstationDef.WorkerTypeName
	}
	return dispatch.WorkerType
}

func workstationLookupKey(dispatch work.WorkDispatch) string {
	return dispatch.WorkstationName
}
