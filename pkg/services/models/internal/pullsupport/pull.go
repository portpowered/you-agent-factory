// Package pullsupport contains implementation helpers for managed model pulls.
// The public Models package owns the value and error contracts; operational
// callers use this package so those helpers do not become a second service API.
package pullsupport

import (
	"errors"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// WrapPullStage attaches a stage without replacing a more specific stage
// already present in the error chain.
func WrapPullStage(
	stage models.PullStage,
	modelName, operation, artifact string,
	cause error,
) error {
	if cause == nil {
		return nil
	}
	var existing *models.PullStageError
	if errors.As(cause, &existing) {
		return cause
	}
	return &models.PullStageError{
		Stage: stage, ModelName: strings.TrimSpace(modelName),
		Operation: strings.TrimSpace(operation), Artifact: strings.TrimSpace(artifact),
		Cause: cause,
	}
}

// PullStageForError returns the most specific stage encoded by a pull error,
// or infers one from the stable Models pull sentinels.
func PullStageForError(err error) models.PullStage {
	if err == nil {
		return ""
	}
	var stageError *models.PullStageError
	if errors.As(err, &stageError) && stageError != nil {
		return stageError.Stage
	}
	switch {
	case errors.Is(err, models.ErrSourceFetchFailed):
		return models.PullStageSourceFetch
	case errors.Is(err, models.ErrModelReferenceInvalid),
		errors.Is(err, models.ErrModelReferenceUnknown),
		errors.Is(err, models.ErrModelRevisionUnresolved),
		errors.Is(err, models.ErrModelConfigurationInvalid),
		errors.Is(err, models.ErrAssetSourceMissing),
		errors.Is(err, models.ErrAssetSourceUnsupported),
		errors.Is(err, models.ErrAssetOffline):
		return models.PullStageSourceResolution
	case errors.Is(err, models.ErrAssetIntegrityFailed):
		return models.PullStageIntegrityVerification
	case errors.Is(err, models.ErrAssetPreparationInterrupted):
		return models.PullStageAssembly
	default:
		return ""
	}
}

// MergePullDiagnostics keeps the primary diagnostic's non-empty facts and
// fills gaps from the fallback.
func MergePullDiagnostics(
	primary, fallback models.PullDiagnostics,
) models.PullDiagnostics {
	primary = primary.Normalize()
	fallback = fallback.Normalize()
	if primary.ModelName == "" {
		primary.ModelName = fallback.ModelName
	}
	if primary.ResolvedRepository == "" {
		primary.ResolvedRepository = fallback.ResolvedRepository
	}
	if primary.Revision == "" {
		primary.Revision = fallback.Revision
	}
	if primary.File == "" {
		primary.File = fallback.File
	}
	if primary.Operation == "" {
		primary.Operation = fallback.Operation
	}
	if primary.RequestURL == "" {
		primary.RequestURL = fallback.RequestURL
	}
	if primary.UpstreamStatusCode == 0 {
		primary.UpstreamStatusCode = fallback.UpstreamStatusCode
	}
	return primary.Normalize()
}

// NewPullDiagnosticsError creates the safe presentation wrapper for a raw
// pull cause. The raw cause remains available through typed matching but is
// never rendered by the wrapper itself.
func NewPullDiagnosticsError(
	diagnostics models.PullDiagnostics,
	cause error,
) error {
	if cause == nil && !diagnostics.Normalize().HasDetails() {
		return nil
	}
	return &models.PullDiagnosticsError{
		Diagnostics: diagnostics.Normalize(), Cause: cause,
	}
}

// WrapPullDiagnostics attaches safe facts to a cause without replacing an
// already-present diagnostic wrapper.
func WrapPullDiagnostics(
	diagnostics models.PullDiagnostics,
	cause error,
) error {
	if cause == nil {
		return nil
	}
	diagnostics = diagnostics.Normalize()
	var existing *models.PullDiagnosticsError
	if errors.As(cause, &existing) && existing != nil {
		merged := MergePullDiagnostics(existing.Diagnostics, diagnostics)
		if merged == existing.Diagnostics.Normalize() {
			return cause
		}
		return &models.PullDiagnosticsError{Diagnostics: merged, Cause: cause}
	}
	return &models.PullDiagnosticsError{Diagnostics: diagnostics, Cause: cause}
}

// PullDiagnosticsFromError recovers explicit diagnostics or derives safe
// stage facts from a classified pull error. It never formats a raw cause.
func PullDiagnosticsFromError(err error) models.PullDiagnostics {
	if err == nil {
		return models.PullDiagnostics{}
	}
	var pullError *models.PullError
	if errors.As(err, &pullError) && pullError != nil {
		if diagnostics := pullError.Result.PullDiagnostics.Normalize(); diagnostics.HasDetails() {
			return diagnostics
		}
	}
	var diagnosticError *models.PullDiagnosticsError
	if errors.As(err, &diagnosticError) && diagnosticError != nil {
		if diagnostics := diagnosticError.Diagnostics.Normalize(); diagnostics.HasDetails() {
			return diagnostics
		}
	}
	var stageError *models.PullStageError
	if errors.As(err, &stageError) && stageError != nil {
		return models.PullDiagnostics{
			ModelName: stageError.ModelName,
			File:      stageError.Artifact,
			Operation: stageError.Operation,
		}.Normalize()
	}
	return models.PullDiagnostics{}
}
