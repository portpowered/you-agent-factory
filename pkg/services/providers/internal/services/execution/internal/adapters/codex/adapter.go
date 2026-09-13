// Package codex owns the parent-private Codex execution adapter.
package codex

import (
	"context"
	"errors"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

// Effect is the invocation-scoped native Codex boundary. Implementations emit
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
// the JSONL protocol.
type EffectResult struct {
	DurationMillis int64
	Metadata       map[string]string
}

// NewRegistration binds one Codex effect to the canonical Codex identity.
func NewRegistration(effect Effect) execution.Registration {
	return execution.Registration{
		Provider: providers.IDCodex,
		Attempt:  newAttempt(effect),
		Continue: newContinuationAttempt(effect),
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
		return newContinuationAttempt(effect)(ctx, execution.ContinuationRequest{ExecuteRequest: request})
	}
}

func newContinuationAttempt(effect Effect) execution.ContinuationAttempt {
	if effect == nil {
		return unavailableContinuationAttempt
	}
	return func(
		ctx context.Context,
		request execution.ContinuationRequest,
	) (providers.ExecuteResult, error) {
		decoder := newDecoder(request.ExecuteRequest.ObserveSession)
		effectResult, effectErr := effect.Execute(ctx, request, decoder.observe)
		flushErr := decoder.flush()
		content, session, finalErr := decoder.final()
		failure, failed := collectFailure(decoder, effectErr, flushErr)
		if finalErr != nil {
			// Preserve an earlier native, decode, flush, declared, or resource
			// classification when the same attempt also lacks a final message.
			// A final-parse failure is the primary route only when no other
			// failure was observed.
			if !failed {
				failure.FinalParseError = finalErr
				// A skipped oversized record only fails the execution when the
				// stream ends without a recoverable final agent decision. Keep the
				// pre-existing record-limit classification for that terminal case.
				if skipped := decoder.skippedRecordFailure(); skipped != nil {
					failure.Declared = skipped
					failure.Diagnostics = decoder.diagnostics()
				}
				failed = true
			}
		}
		if failure.SessionRef == nil {
			failure.SessionRef = decoder.sessionRef()
		}
		if failure.Diagnostics == nil {
			failure.Diagnostics = decoder.diagnostics()
		}
		result := providers.ExecuteResult{
			Content:    content,
			SessionRef: session,
			Diagnostics: &providers.ExecuteDiagnostics{
				DurationMillis: effectResult.DurationMillis,
				Progress:       decoder.progressFacts(),
				Metadata:       completedMetadata(effectResult.Metadata, decoder.diagnostics().Metadata),
			},
		}
		if failed {
			return result, failure
		}
		return result, nil
	}
}

func completedMetadata(native, decoded map[string]string) map[string]string {
	metadata := cloneMetadata(native)
	if metadata == nil {
		metadata = make(map[string]string, 4)
	}
	for key, value := range decoded {
		metadata[key] = value
	}
	metadata["completion_evidence"] = "agent_message"
	return metadata
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
	if resourceFailure := decoder.resourceFailure(); resourceFailure != nil {
		if failure.Declared == nil || failure.Declared.Kind == providers.ExecuteFailureKindUnknown {
			failure.Declared = resourceFailure
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
	if decoder.limit != nil || decoder.recordSkips > 0 ||
		decoder.transcriptFull || decoder.diagnosticsFull || decoder.retainedTextFull {
		failure.Diagnostics = decoder.diagnostics()
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
		Message: "Codex native execution is unavailable",
	}
}

func unavailableContinuationAttempt(
	context.Context,
	execution.ContinuationRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindDependency,
		Message: "Codex native execution is unavailable",
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
