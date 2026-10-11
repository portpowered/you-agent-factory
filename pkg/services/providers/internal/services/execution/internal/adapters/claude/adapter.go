// Package claude owns the parent-private Claude execution adapter.
package claude

import (
	"context"
	"errors"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

// Effect is the invocation-scoped native Claude boundary. Implementations emit
// stdout chunks in arrival order and return only allowlisted execution facts.
type Effect interface {
	Execute(context.Context, execution.ContinuationRequest, func([]byte) error) (EffectResult, error)
}

// EffectFunc adapts a function to Effect.
type EffectFunc func(
	context.Context,
	execution.ContinuationRequest,
	func([]byte) error,
) (EffectResult, error)

// Execute invokes the adapted function.
func (effect EffectFunc) Execute(
	ctx context.Context,
	request execution.ContinuationRequest,
	observe func([]byte) error,
) (EffectResult, error) {
	return effect(ctx, request, observe)
}

// EffectResult contains safe native execution facts that are not derived from
// the stream-json protocol.
type EffectResult struct {
	DurationMillis int64
	Metadata       map[string]string
}

// NewRegistration binds one Claude effect to the canonical Claude identity.
func NewRegistration(effect Effect) execution.Registration {
	continuation := newContinuationAttempt(effect)
	return execution.Registration{
		Provider: providers.IDClaude,
		Attempt:  newAttempt(continuation),
		Continue: continuation,
	}
}

func newAttempt(continuation execution.ContinuationAttempt) execution.Attempt {
	return func(
		ctx context.Context,
		request providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		return continuation(ctx, execution.ContinuationRequest{ExecuteRequest: request})
	}
}

func newContinuationAttempt(effect Effect) execution.ContinuationAttempt {
	return func(
		ctx context.Context,
		request execution.ContinuationRequest,
	) (providers.ExecuteResult, error) {
		decoder := newDecoder(request.AttemptID, request.ExecuteRequest.ObserveSession)
		observed := 0
		publish := func() {
			for observed < len(decoder.progress) {
				request.ObserveProgress(decoder.progress[observed])
				observed++
			}
		}
		effectResult, effectErr := effect.Execute(ctx, request, func(chunk []byte) error {
			err := decoder.observe(chunk)
			publish()
			return err
		})
		flushErr := decoder.flush()
		failure, failed := collectFailure(decoder, effectErr, flushErr)
		content, session, finalErr := decoder.final()
		if finalErr != nil && !failed {
			failure.FinalParseError = finalErr
			failed = true
		}
		if failed && len(decoder.progress) > 0 && decoder.progress[len(decoder.progress)-1].Phase == "run.completed" {
			decoder.progress = decoder.progress[:len(decoder.progress)-1]
		}
		publish()
		if failed {
			if failure.SessionRef == nil {
				failure.SessionRef = decoder.sessionRef()
			}
			if failure.Diagnostics == nil {
				failure.Diagnostics = &providers.ExecuteDiagnostics{}
			}
			failure.Diagnostics.DurationMillis = effectResult.DurationMillis
			failure.Diagnostics.Progress = decoder.progressFacts()
			failure.Diagnostics.ProgressAlreadyObserved = request.ProgressObserver != nil
			return providers.ExecuteResult{}, failure
		}
		metadata := cloneMetadata(effectResult.Metadata)
		if metadata == nil {
			metadata = make(map[string]string, 3)
		}
		for key, value := range decoder.reportedUsage {
			metadata[key] = value
		}
		metadata["completion_evidence"] = "agent_message"
		return providers.ExecuteResult{
			Content:    content,
			SessionRef: session,
			Diagnostics: &providers.ExecuteDiagnostics{
				DurationMillis:          effectResult.DurationMillis,
				Progress:                decoder.progressFacts(),
				ProgressAlreadyObserved: request.ProgressObserver != nil,
				Metadata:                metadata,
			},
		}, nil
	}
}

func collectFailure(
	decoder *decoder,
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
