// Package pi owns the parent-private Pi execution adapter.
package pi

import (
	"context"
	"errors"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

// Effect is the invocation-scoped native Pi boundary. Implementations emit
// stdout chunks in arrival order and return only allowlisted execution facts.
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
// the structured JSONL protocol.
type EffectResult struct {
	DurationMillis int64
	Metadata       map[string]string
	Command        CommandFacts
}

// NewRegistration binds one Pi effect to the canonical Pi identity.
func NewRegistration(effect Effect) execution.Registration {
	return execution.Registration{
		Provider: providers.IDPi,
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
		decoder := newDecoder(request.AttemptID)
		effectResult, effectErr := effect.Execute(ctx, request, decoder.observe)
		flushErr := decoder.flush()
		if failure, failed := collectFailure(decoder, effectResult, effectErr, flushErr); failed {
			sessionRef := decoder.sessionRef()
			progress := decoder.progressFacts()
			failure = attachFailureObservation(failure, sessionRef, progress)
			return providers.ExecuteResult{SessionRef: sessionRef}, failure
		}
		parsed, parseErr := parseFinalOutput(effectResult.Command.Stdout)
		if parseErr != nil {
			sessionRef := decoder.sessionRef()
			progress := decoder.progressFacts()
			failure := execution.AttemptFailure{
				Declared: &providers.ExecuteFailure{
					Kind:    providers.ExecuteFailureKindUnknown,
					Message: parseErr.Error(),
					Diagnostics: &providers.ExecuteDiagnostics{
						Progress: progress,
						Metadata: map[string]string{"failure_stage": "final_parse"},
					},
					SessionRef: sessionRef,
				},
			}
			return providers.ExecuteResult{SessionRef: sessionRef}, failure
		}
		sessionRef := sessionRefFromParsed(parsed, decoder.sessionRef())
		return providers.ExecuteResult{
			Content:    parsed.Content,
			SessionRef: sessionRef,
			Diagnostics: &providers.ExecuteDiagnostics{
				DurationMillis: effectResult.DurationMillis,
				Progress:       decoder.progressFacts(),
				Metadata:       cloneMetadata(effectResult.Metadata),
			},
		}, nil
	}
}

func collectFailure(
	decoder *decoder,
	effectResult EffectResult,
	effectErr error,
	flushErr error,
) (execution.AttemptFailure, bool) {
	failure, failed := nativeFailure(effectErr)
	if decoder.declaredFailure != nil {
		declared := decoder.declaredFailure.Clone()
		if failure.Declared == nil ||
			declared.Kind != providers.ExecuteFailureKindUnknown ||
			failure.Declared.Kind == providers.ExecuteFailureKindUnknown {
			failure.Declared = &declared
		}
		failure.NativeError = nil
		failed = true
	}
	if decoder.decodeErr != nil {
		failure.DecodeError = decoder.decodeErr
		failed = true
	}
	if flushErr != nil {
		failure.FlushError = flushErr
		failed = true
	}
	if !failed && effectResult.Command.ExitCode != 0 {
		if declared, ok := declaredFailureFromCommandOutput(effectResult.Command.Stdout); ok {
			failure.Declared = &declared
			failed = true
		} else if retry := piRetryFailure(effectResult.Command.Stdout); retry != nil {
			failure.Declared = retry
			failed = true
		} else {
			failure.Declared = &providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindUnknown,
				Message: piInvocationFailureMessage(),
			}
			failed = true
		}
	}
	return failure, failed
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
		Message: "Pi native execution is unavailable",
	}
}

const failureStageDecode = "decode"

func attachFailureObservation(
	failure execution.AttemptFailure,
	sessionRef *providers.SessionRef,
	progress []providers.ExecuteProgress,
) execution.AttemptFailure {
	if sessionRef == nil && len(progress) == 0 {
		return failure
	}
	declared := providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown}
	if failure.Declared != nil {
		declared = failure.Declared.Clone()
	} else if failure.DecodeError != nil {
		declared.Kind = providers.ExecuteFailureKindInvalidRequest
		declared.Message = piDeclaredFailureMessage(providers.ExecuteFailureKindUnknown)
		declared.Diagnostics = &providers.ExecuteDiagnostics{
			Metadata: map[string]string{"failure_stage": failureStageDecode},
		}
	}
	if sessionRef != nil {
		declared.SessionRef = sessionRef
	}
	if len(progress) > 0 {
		if declared.Diagnostics == nil {
			declared.Diagnostics = &providers.ExecuteDiagnostics{}
		}
		declared.Diagnostics.Progress = progress
	}
	failure.Declared = &declared
	failure.NativeError = nil
	failure.DecodeError = nil
	return failure
}

func sessionRefFromParsed(parsed FinalResult, fallback *providers.SessionRef) *providers.SessionRef {
	if parsed.SessionID != "" {
		return &providers.SessionRef{
			Provider: providers.IDPi,
			Kind:     providers.SessionIDKind,
			ID:       parsed.SessionID,
		}
	}
	return cloneSessionRef(fallback)
}

func cloneSessionRef(sessionRef *providers.SessionRef) *providers.SessionRef {
	if sessionRef == nil {
		return nil
	}
	cloned := sessionRef.Clone()
	return &cloned
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
