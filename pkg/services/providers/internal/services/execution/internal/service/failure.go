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

func normalizeValidationFailure(request providers.ExecuteRequest) error {
	return normalizeDeclaredFailure(providers.ExecuteFailure{
		Kind: providers.ExecuteFailureKindInvalidRequest,
	}, request)
}

func normalizeContextFailure(
	ctx context.Context,
	request providers.ExecuteRequest,
) error {
	if err := ctx.Err(); err != nil {
		return normalizeAttemptFailure(ctx, err, request)
	}
	return nil
}

func normalizeAttemptFailure(
	ctx context.Context,
	attemptErr error,
	request providers.ExecuteRequest,
) (normalized error) {
	lifecycle, hasLifecycle := attemptFailureAs(attemptErr)
	defer func() {
		if !hasLifecycle || lifecycle.SessionRef == nil || normalized == nil {
			return
		}
		if failure, ok := executeFailureAs(normalized); ok {
			session := lifecycle.SessionRef.Clone()
			failure.SessionRef = &session
			normalized = failure
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
		}, request)
	}
	if containsError(signals, context.Canceled) {
		return normalizeDeclaredFailure(providers.ExecuteFailure{
			Kind: providers.ExecuteFailureKindCanceled,
		}, request)
	}
	if hasLifecycle && lifecycle.Declared != nil {
		return normalizeDeclaredFailure(lifecycle.Declared.Clone(), request)
	}
	if declared, ok := executeFailureAs(attemptErr); ok {
		return normalizeDeclaredFailure(declared, request)
	}
	if errors.Is(attemptErr, context.DeadlineExceeded) ||
		errors.Is(attemptErr, providers.ErrExecuteTimeout) {
		return normalizeDeclaredFailure(providers.ExecuteFailure{
			Kind: providers.ExecuteFailureKindTimeout,
		}, request)
	}
	if errors.Is(attemptErr, context.Canceled) ||
		errors.Is(attemptErr, providers.ErrExecuteCancelled) {
		return normalizeDeclaredFailure(providers.ExecuteFailure{
			Kind: providers.ExecuteFailureKindCanceled,
		}, request)
	}
	// Stream lifecycle failures describe an unusable provider response, not a
	// caller validation failure. Classify them as dependency failures so the
	// bounded worker retry policy can recover transient provider output faults.
	if hasLifecycle && (lifecycle.FinalParseError != nil ||
		lifecycle.FlushError != nil || lifecycle.DecodeError != nil) {
		return normalizeDeclaredFailure(providers.ExecuteFailure{
			Kind:        providers.ExecuteFailureKindDependency,
			Diagnostics: lifecycleStageDiagnostics(lifecycle, hasLifecycle),
		}, request)
	}
	return normalizeDeclaredFailure(providers.ExecuteFailure{
		Kind:        providers.ExecuteFailureKindUnknown,
		Diagnostics: lifecycleStageDiagnostics(lifecycle, hasLifecycle),
	}, request)
}

func normalizeDeclaredFailure(
	failure providers.ExecuteFailure,
	request providers.ExecuteRequest,
) providers.ExecuteFailure {
	failure = failure.Clone()
	if !knownFailureKind(failure.Kind) {
		failure.Kind = providers.ExecuteFailureKindUnknown
	}
	failure.Message = sanitizeDiagnosticText(
		failure.Message,
		maxDiagnosticRunes,
		request,
	)
	if failure.Message == "" {
		failure.Message = defaultFailureMessage(failure.Kind)
	}
	if failure.Diagnostics != nil {
		diagnostics := normalizeDiagnostics(*failure.Diagnostics, request)
		failure.Diagnostics = &diagnostics
	}
	return failure
}

func lifecycleStageDiagnostics(
	failure execution.AttemptFailure,
	ok bool,
) *providers.ExecuteDiagnostics {
	if !ok {
		return nil
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
	if stage == "" {
		return nil
	}
	return &providers.ExecuteDiagnostics{
		Metadata: map[string]string{"failure_stage": stage},
	}
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
		providers.ExecuteFailureKindUnknown:
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
	default:
		return "provider execution failed"
	}
}
