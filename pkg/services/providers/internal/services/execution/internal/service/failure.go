package service

import (
	"context"
	"errors"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

const (
	failureStageNative     = "native"
	failureStageDecode     = "decode"
	failureStageFlush      = "flush"
	failureStageFinalParse = "final_parse"
)

func normalizeValidationFailure(request providers.ExecuteRequest, extraSecrets ...string) error {
	return normalizeDeclaredFailure(providers.ExecuteFailure{
		Kind: providers.ExecuteFailureKindInvalidRequest,
	}, request, extraSecrets...)
}

func normalizeContextFailure(
	ctx context.Context,
	request providers.ExecuteRequest,
	extraSecrets ...string,
) error {
	if err := ctx.Err(); err != nil {
		return normalizeAttemptFailure(ctx, err, request, extraSecrets...)
	}
	return nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func normalizeAttemptFailure(
	ctx context.Context,
	attemptErr error,
	request providers.ExecuteRequest,
	extraSecrets ...string,
) (normalized error) {
	lifecycle, hasLifecycle := attemptFailureAs(attemptErr)
	failureSession := sessionRefFromAttemptFailure(attemptErr, lifecycle, hasLifecycle)
	defer func() {
		if failureSession == nil || normalized == nil {
			return
		}
		if failure, ok := executeFailureAs(normalized); ok {
			failure.SessionRef = failureSession
			normalized = normalizeDeclaredFailure(failure, request, extraSecrets...)
		}
	}()
	signals := []error{ctx.Err()}
	if hasLifecycle {
		signals = append(signals,
			lifecycle.FinalParseError,
			lifecycle.FlushError,
			lifecycle.DecodeError,
			lifecycle.NativeError,
		)
	}
	if containsError(signals, context.DeadlineExceeded) {
		return normalizeDeclaredFailure(providers.ExecuteFailure{
			Kind: providers.ExecuteFailureKindTimeout,
		}, request, extraSecrets...)
	}
	if containsError(signals, context.Canceled) {
		return normalizeDeclaredFailure(providers.ExecuteFailure{
			Kind: providers.ExecuteFailureKindCanceled,
		}, request, extraSecrets...)
	}
	if hasLifecycle && lifecycle.Declared != nil {
		declared := lifecycle.Declared.Clone()
		declared.Diagnostics = mergeLifecycleDiagnostics(declared.Diagnostics, lifecycle.Diagnostics)
		return normalizeDeclaredFailure(declared, request, extraSecrets...)
	}
	if declared, ok := executeFailureAs(attemptErr); ok {
		return normalizeDeclaredFailure(declared, request, extraSecrets...)
	}
	if errors.Is(attemptErr, context.DeadlineExceeded) ||
		errors.Is(attemptErr, providers.ErrExecuteTimeout) {
		return normalizeDeclaredFailure(providers.ExecuteFailure{
			Kind: providers.ExecuteFailureKindTimeout,
		}, request, extraSecrets...)
	}
	if errors.Is(attemptErr, context.Canceled) ||
		errors.Is(attemptErr, providers.ErrExecuteCancelled) {
		return normalizeDeclaredFailure(providers.ExecuteFailure{
			Kind: providers.ExecuteFailureKindCanceled,
		}, request, extraSecrets...)
	}
	// Stream lifecycle failures describe an unusable provider response, not a
	// caller validation failure. Classify them as dependency failures so the
	// bounded worker retry policy can recover transient provider output faults.
	if hasLifecycle && (lifecycle.FinalParseError != nil ||
		lifecycle.FlushError != nil || lifecycle.DecodeError != nil) {
		return normalizeDeclaredFailure(providers.ExecuteFailure{
			Kind:        providers.ExecuteFailureKindDependency,
			Diagnostics: lifecycleStageDiagnostics(lifecycle, hasLifecycle),
		}, request, extraSecrets...)
	}
	return normalizeDeclaredFailure(providers.ExecuteFailure{
		Kind:        providers.ExecuteFailureKindUnknown,
		Diagnostics: lifecycleStageDiagnostics(lifecycle, hasLifecycle),
	}, request, extraSecrets...)
}

func sessionRefFromAttemptFailure(
	attemptErr error,
	lifecycle execution.AttemptFailure,
	hasLifecycle bool,
) *providers.SessionRef {
	if hasLifecycle {
		if lifecycle.SessionRef != nil {
			return lifecycle.SessionRef
		}
		if lifecycle.Declared != nil && lifecycle.Declared.SessionRef != nil {
			return lifecycle.Declared.SessionRef
		}
	}
	if failure, ok := executeFailureAs(attemptErr); ok {
		return failure.SessionRef
	}
	return nil
}

func normalizeDeclaredFailure(
	failure providers.ExecuteFailure,
	request providers.ExecuteRequest,
	extraSecrets ...string,
) error {
	failure = failure.Clone()
	if err := validateSessionRef(failure.SessionRef, request.Provider); err != nil {
		return err
	}
	if !knownFailureKind(failure.Kind) {
		failure.Kind = providers.ExecuteFailureKindUnknown
	}
	failure.Message = sanitizeDiagnosticText(
		failure.Message,
		maxDiagnosticRunes,
		request,
		extraSecrets...,
	)
	if failure.Message == "" {
		failure.Message = defaultFailureMessage(failure.Kind)
	}
	if failure.Diagnostics != nil {
		diagnostics := normalizeDiagnostics(*failure.Diagnostics, request, extraSecrets...)
		failure.Diagnostics = &diagnostics
	}
	return failure
}

func lifecycleStageDiagnostics(
	failure execution.AttemptFailure,
	ok bool,
) *providers.ExecuteDiagnostics {
	if !ok && failure.Diagnostics == nil {
		return nil
	}
	diagnostics := providers.ExecuteDiagnostics{}
	if failure.Diagnostics != nil {
		diagnostics = failure.Diagnostics.Clone()
	}
	stage := ""
	switch {
	case failure.FinalParseError != nil:
		stage = failureStageFinalParse
	case failure.FlushError != nil:
		stage = failureStageFlush
	case failure.DecodeError != nil:
		stage = failureStageDecode
	case failure.NativeError != nil:
		stage = failureStageNative
	}
	if stage != "" {
		if diagnostics.Metadata == nil {
			diagnostics.Metadata = make(map[string]string)
		}
		diagnostics.Metadata["failure_stage"] = stage
	}
	if diagnostics.Metadata == nil && len(diagnostics.Progress) == 0 && diagnostics.DurationMillis == 0 {
		return nil
	}
	return &diagnostics
}

func mergeLifecycleDiagnostics(
	declared *providers.ExecuteDiagnostics,
	lifecycle *providers.ExecuteDiagnostics,
) *providers.ExecuteDiagnostics {
	if declared == nil {
		if lifecycle == nil {
			return nil
		}
		clone := lifecycle.Clone()
		return &clone
	}
	if lifecycle == nil {
		clone := declared.Clone()
		return &clone
	}
	merged := declared.Clone()
	overlay := lifecycle.Clone()
	if len(overlay.Progress) > 0 {
		merged.Progress = append(merged.Progress, overlay.Progress...)
	}
	if merged.Metadata == nil {
		merged.Metadata = make(map[string]string)
	}
	for key, value := range overlay.Metadata {
		merged.Metadata[key] = value
	}
	if overlay.DurationMillis > merged.DurationMillis {
		merged.DurationMillis = overlay.DurationMillis
	}
	merged.ProgressAlreadyObserved = merged.ProgressAlreadyObserved || overlay.ProgressAlreadyObserved
	return &merged
}

func containsError(values []error, target error) bool {
	for _, value := range values {
		if errors.Is(value, target) {
			return true
		}
	}
	return false
}

func attemptFailureAs(err error) (execution.AttemptFailure, bool) {
	var value execution.AttemptFailure
	if errors.As(err, &value) {
		return value, true
	}
	var pointer *execution.AttemptFailure
	if errors.As(err, &pointer) && pointer != nil {
		return *pointer, true
	}
	return execution.AttemptFailure{}, false
}

func executeFailureAs(err error) (providers.ExecuteFailure, bool) {
	var value providers.ExecuteFailure
	if errors.As(err, &value) {
		return value.Clone(), true
	}
	var pointer *providers.ExecuteFailure
	if errors.As(err, &pointer) && pointer != nil {
		return pointer.Clone(), true
	}
	return providers.ExecuteFailure{}, false
}

func knownFailureKind(kind providers.ExecuteFailureKind) bool {
	switch kind {
	case providers.ExecuteFailureKindCanceled,
		providers.ExecuteFailureKindTimeout,
		providers.ExecuteFailureKindAuthentication,
		providers.ExecuteFailureKindInvalidRequest,
		providers.ExecuteFailureKindMisconfigured,
		providers.ExecuteFailureKindThrottled,
		providers.ExecuteFailureKindDependency,
		providers.ExecuteFailureKindUnknown,
		providers.ExecuteFailureKindCapabilityMismatch,
		providers.ExecuteFailureKindSessionNotFound:
		return true
	default:
		return false
	}
}

func defaultFailureMessage(kind providers.ExecuteFailureKind) string {
	switch kind {
	case providers.ExecuteFailureKindCanceled:
		return "provider execution was canceled"
	case providers.ExecuteFailureKindTimeout:
		return "provider execution timed out"
	case providers.ExecuteFailureKindAuthentication:
		return "provider authentication failed"
	case providers.ExecuteFailureKindInvalidRequest:
		return "provider execution request is invalid"
	case providers.ExecuteFailureKindMisconfigured:
		return "provider execution is misconfigured"
	case providers.ExecuteFailureKindThrottled:
		return "provider execution was throttled"
	case providers.ExecuteFailureKindDependency:
		return "provider dependency failed"
	case providers.ExecuteFailureKindCapabilityMismatch:
		return "provider does not support the requested capability"
	case providers.ExecuteFailureKindSessionNotFound:
		return "provider does not recognize the referenced Provider Session as live"
	default:
		return "provider execution failed"
	}
}
