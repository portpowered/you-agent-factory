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
		decoder := newDecoder()
		effectResult, effectErr := effect.Execute(ctx, request, decoder.observe)
		if effectErr != nil {
			var attemptFailure execution.AttemptFailure
			if errors.As(effectErr, &attemptFailure) {
				return providers.ExecuteResult{}, effectErr
			}
			return providers.ExecuteResult{}, execution.AttemptFailure{
				NativeError: effectErr,
			}
		}
		if flushErr := decoder.flush(); flushErr != nil {
			return providers.ExecuteResult{}, execution.AttemptFailure{
				FlushError: flushErr,
			}
		}
		content, session, finalErr := decoder.final()
		if finalErr != nil {
			return providers.ExecuteResult{}, execution.AttemptFailure{
				FinalParseError: finalErr,
			}
		}
		return providers.ExecuteResult{
			Content:    content,
			SessionRef: session,
			Diagnostics: &providers.ExecuteDiagnostics{
				DurationMillis: effectResult.DurationMillis,
				Progress:       decoder.progressFacts(),
				Metadata:       cloneMetadata(effectResult.Metadata),
			},
		}, nil
	}
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
