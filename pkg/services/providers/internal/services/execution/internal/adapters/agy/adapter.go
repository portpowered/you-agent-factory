// Package agy owns the parent-private Agy execution adapter. PTY transport is
// part of this canonical Agy identity; no separate provider identity exists.
package agy

import (
	"bytes"
	"context"
	"errors"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

// Effect is the invocation-scoped native Agy boundary. Implementations drive
// PTY-backed execution, emit final stdout through observe, and return only
// allowlisted execution facts.
type Effect interface {
	Execute(context.Context, providers.ExecuteRequest, func([]byte) error) (EffectResult, error)
}

// EffectFunc adapts a function to Effect.
type EffectFunc func(
	context.Context,
	providers.ExecuteRequest,
	func([]byte) error,
) (EffectResult, error)

// Execute invokes the adapted function.
func (effect EffectFunc) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
	observe func([]byte) error,
) (EffectResult, error) {
	return effect(ctx, request, observe)
}

// EffectResult contains safe native execution facts that are not derived from
// the final-only stdout payload.
type EffectResult struct {
	DurationMillis int64
	Metadata       map[string]string
	SessionRef     *providers.SessionRef
}

// NewRegistration binds one Agy effect to the canonical Agy identity.
func NewRegistration(effect Effect) execution.Registration {
	return execution.Registration{
		Provider: providers.IDAgy,
		Attempt:  newAttempt(effect),
	}
}

func newAttempt(effect Effect) execution.Attempt {
	if effect == nil {
		return unavailableAttempt
	}
	return func(
		ctx context.Context,
		request providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		var stdout bytes.Buffer
		effectResult, effectErr := effect.Execute(ctx, request, func(chunk []byte) error {
			stdout.Write(chunk)
			return nil
		})
		if failure, failed := collectFailure(effectErr); failed {
			sessionRef := cloneSessionRef(effectResult.SessionRef)
			if sessionRef == nil {
				sessionRef = sessionRefFromRequest(request.ResumeSession)
			}
			failure = attachFailureSession(failure, sessionRef)
			return providers.ExecuteResult{SessionRef: sessionRef}, failure
		}
		content, parseFailure := parseFinalOutput(stdout.Bytes())
		if parseFailure != nil {
			sessionRef := cloneSessionRef(effectResult.SessionRef)
			if sessionRef == nil {
				sessionRef = sessionRefFromRequest(request.ResumeSession)
			}
			declared := parseFailure.Clone()
			declared.SessionRef = sessionRef
			return providers.ExecuteResult{SessionRef: sessionRef}, execution.AttemptFailure{Declared: &declared}
		}
		return providers.ExecuteResult{
			Content:    content,
			SessionRef: cloneSessionRef(effectResult.SessionRef),
			Diagnostics: &providers.ExecuteDiagnostics{
				DurationMillis: effectResult.DurationMillis,
				Metadata:       cloneMetadata(effectResult.Metadata),
			},
		}, nil
	}
}

func collectFailure(effectErr error) (execution.AttemptFailure, bool) {
	return nativeFailure(effectErr)
}

func nativeFailure(err error) (execution.AttemptFailure, bool) {
	if err == nil {
		return execution.AttemptFailure{}, false
	}
	var lifecycle execution.AttemptFailure
	if errors.As(err, &lifecycle) {
		return lifecycle, true
	}
	var lifecyclePointer *execution.AttemptFailure
	if errors.As(err, &lifecyclePointer) && lifecyclePointer != nil {
		return *lifecyclePointer, true
	}
	var declared providers.ExecuteFailure
	if errors.As(err, &declared) {
		declared = declared.Clone()
		return execution.AttemptFailure{Declared: &declared}, true
	}
	var declaredPointer *providers.ExecuteFailure
	if errors.As(err, &declaredPointer) && declaredPointer != nil {
		declared = declaredPointer.Clone()
		return execution.AttemptFailure{Declared: &declared}, true
	}
	return execution.AttemptFailure{NativeError: err}, true
}

func unavailableAttempt(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindDependency,
		Message: "Agy native execution is unavailable",
	}
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func cloneSessionRef(sessionRef *providers.SessionRef) *providers.SessionRef {
	if sessionRef == nil {
		return nil
	}
	cloned := sessionRef.Clone()
	return &cloned
}

func attachFailureSession(
	failure execution.AttemptFailure,
	sessionRef *providers.SessionRef,
) execution.AttemptFailure {
	if failure.Declared == nil && failure.NativeError != nil {
		if setup := classifySetupError(failure.NativeError); setup != nil {
			declared := setup.Clone()
			declared.SessionRef = cloneSessionRef(sessionRef)
			failure.Declared = &declared
			failure.NativeError = nil
			return failure
		}
	}
	if sessionRef == nil || failure.Declared == nil {
		return failure
	}
	declared := failure.Declared.Clone()
	declared.SessionRef = cloneSessionRef(sessionRef)
	failure.Declared = &declared
	failure.NativeError = nil
	return failure
}
