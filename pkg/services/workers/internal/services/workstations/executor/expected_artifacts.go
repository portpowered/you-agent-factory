package executor

import (
	"bytes"
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
