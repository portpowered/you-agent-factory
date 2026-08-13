package runtime

import (
	"fmt"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	unsafeExpectedArtifactPattern = "<invalid>"
	unrenderableExpectedArtifact  = "<unrenderable>"
)

func verifyExpectedArtifactsForDispatch(
	cfg *runtimeConfig,
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		return result
	}
	declarations := expectedArtifactDeclarationsForDispatch(cfg, request.Input.Dispatch)
	if len(declarations) == 0 {
		return result
	}
	verification := verifyRuntimeExpectedArtifactDeclarations(
		request.Target.Environment.WorkingDirectory,
		request.Input.Dispatch,
		declarations,
		platformfilesystem.Local{},
	)
	if verification == nil || len(verification.Entries) == 0 {
		return result
	}
	message := expectedArtifactVerificationMessage(verification)
	result.Outcome = workers.ExecutionOutcomeFailed
	result.ArtifactVerification = verification
	result.Failure = &workers.ExecutionFailure{
		Family:  workers.WorkFailureFamilyTerminal,
		Type:    workers.WorkFailureTypeExpectedArtifactsUnsatisfied,
		Message: message,
		Detail: &workers.FailureDetail{
			Reason:  workers.WorkFailureTypeExpectedArtifactsUnsatisfied,
			Message: message,
		},
	}
	return result
}

func expectedArtifactDeclarationsForDispatch(
	cfg *runtimeConfig,
	dispatch work.WorkDispatch,
) []work.ExpectedArtifactDeclaration {
	if cfg == nil || cfg.net == nil {
		return nil
	}
	var declarations []work.ExpectedArtifactDeclaration
	seen := make(map[work.ExpectedArtifactDeclaration]struct{})
	for _, token := range workers.WorkDispatchInputTokens(dispatch) {
		workType := cfg.net.WorkTypes[token.Color.WorkTypeID]
		if workType == nil {
			continue
		}
		for _, declaration := range workType.ExpectedArtifacts {
			if _, exists := seen[declaration]; exists {
				continue
			}
			seen[declaration] = struct{}{}
			declarations = append(declarations, declaration)
		}
	}
	if transition := cfg.net.Transitions[dispatch.TransitionID]; transition != nil {
		for _, declaration := range transition.ExpectedArtifacts {
			if _, exists := seen[declaration]; exists {
				continue
			}
			seen[declaration] = struct{}{}
			declarations = append(declarations, declaration)
		}
	}
	return declarations
}

func verifyRuntimeExpectedArtifactDeclarations(
	workspace string,
	dispatch work.WorkDispatch,
	declarations []work.ExpectedArtifactDeclaration,
	fileSystem platformfilesystem.GlobInspector,
) *workers.ExpectedArtifactVerification {
	workspace = strings.TrimSpace(workspace)
	context := dispatch.ExpectedArtifactContext
	if context == nil {
		context = &work.ExpectedArtifactTemplateContext{}
	}
	entries := make([]workers.ExpectedArtifactVerificationEntry, 0, len(declarations))
	for index, declaration := range declarations {
		pattern, err := context.Render(declaration.Pattern, nil)
		if err != nil {
			entries = append(entries, runtimeExpectedArtifactFailureEntry(
				index,
				declaration.Name,
				unrenderableExpectedArtifact,
				workers.ExpectedArtifactVerificationReasonMissing,
			))
			continue
		}
		pattern, safe := safeRuntimeExpectedArtifactPattern(pattern)
		if !safe {
			entries = append(entries, runtimeExpectedArtifactFailureEntry(
				index,
				declaration.Name,
				unsafeExpectedArtifactPattern,
				workers.ExpectedArtifactVerificationReasonMissing,
			))
			continue
		}
		reason, satisfied := runtimeExpectedArtifactStatus(
			workspace,
			pattern,
			declaration.NonEmpty,
			fileSystem,
		)
		if !satisfied {
			entries = append(entries, runtimeExpectedArtifactFailureEntry(index, declaration.Name, pattern, reason))
		}
	}
	if len(entries) == 0 {
		return nil
	}
	return &workers.ExpectedArtifactVerification{
		Code:    workers.WorkFailureTypeExpectedArtifactsUnsatisfied,
		Entries: entries,
	}
}

func runtimeExpectedArtifactFailureEntry(
	declarationIndex int,
	name string,
	pattern string,
	reason workers.ExpectedArtifactVerificationReason,
) workers.ExpectedArtifactVerificationEntry {
	return workers.ExpectedArtifactVerificationEntry{
		DeclarationIndex: declarationIndex + 1,
		Name:             strings.TrimSpace(name),
		Pattern:          pattern,
		Reason:           reason,
	}
}

func safeRuntimeExpectedArtifactPattern(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false
	}
	portable := filepath.ToSlash(trimmed)
	if pathpkg.IsAbs(portable) || strings.HasPrefix(portable, "/") {
		return "", false
	}
	if len(portable) >= 2 && portable[1] == ':' && isASCIIAlphaRuntime(portable[0]) {
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

func runtimeExpectedArtifactStatus(
	workspace string,
	pattern string,
	nonEmpty bool,
	fileSystem platformfilesystem.GlobInspector,
) (workers.ExpectedArtifactVerificationReason, bool) {
	if fileSystem == nil || workspace == "" {
		return workers.ExpectedArtifactVerificationReasonMissing, false
	}
	if !strings.ContainsAny(pattern, "*?[") {
		candidate := filepath.Join(workspace, filepath.FromSlash(pattern))
		if !runtimeExpectedArtifactPathWithinWorkspace(workspace, candidate, fileSystem) {
			return workers.ExpectedArtifactVerificationReasonMissing, false
		}
		info, err := fileSystem.Stat(candidate)
		if err != nil || info == nil || !info.Mode().IsRegular() {
			return workers.ExpectedArtifactVerificationReasonMissing, false
		}
		if nonEmpty && info.Size() == 0 {
			return workers.ExpectedArtifactVerificationReasonEmpty, false
		}
		return "", true
	}
	matches, err := fileSystem.Glob(filepath.Join(workspace, filepath.FromSlash(pattern)))
	if err != nil {
		return workers.ExpectedArtifactVerificationReasonMissing, false
	}
	sort.Strings(matches)
	regularFiles := 0
	for _, match := range matches {
		if !runtimeExpectedArtifactPathWithinWorkspace(workspace, match, fileSystem) {
			continue
		}
		info, statErr := fileSystem.Stat(match)
		if statErr != nil || info == nil || !info.Mode().IsRegular() {
			continue
		}
		regularFiles++
		if nonEmpty && info.Size() == 0 {
			return workers.ExpectedArtifactVerificationReasonEmpty, false
		}
	}
	if regularFiles == 0 {
		return workers.ExpectedArtifactVerificationReasonMissing, false
	}
	return "", true
}

func runtimeExpectedArtifactPathWithinWorkspace(
	workspace string,
	candidate string,
	fileSystem platformfilesystem.GlobInspector,
) bool {
	resolvedWorkspace, err := fileSystem.EvalSymlinks(workspace)
	if err != nil {
		return false
	}
	resolvedCandidate, err := fileSystem.EvalSymlinks(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(resolvedWorkspace), filepath.Clean(resolvedCandidate))
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func expectedArtifactVerificationMessage(
	verification *workers.ExpectedArtifactVerification,
) string {
	if verification == nil || len(verification.Entries) == 0 {
		return string(workers.WorkFailureTypeExpectedArtifactsUnsatisfied)
	}
	details := make([]string, 0, len(verification.Entries))
	for _, entry := range verification.Entries {
		details = append(details, fmt.Sprintf("%s=%s (%s)", entry.Name, entry.Pattern, entry.Reason))
	}
	return fmt.Sprintf("%s: %s", verification.Code, strings.Join(details, "; "))
}

func isASCIIAlphaRuntime(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}
