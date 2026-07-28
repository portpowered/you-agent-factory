// Package opencode owns the parent-private OpenCode execution adapter.
package opencode

import (
	"context"
	"errors"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

// Mode is the selected OpenCode output protocol for one attempt.
type Mode string

const (
	ModeStructured Mode = "structured"
	ModeFinalOnly  Mode = "final_only"
)

// Effect is the invocation-scoped native OpenCode boundary. Implementations
// emit stdout chunks in arrival order and return only allowlisted execution facts.
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

// CommandFacts retain bounded subprocess facts for one native attempt.
type CommandFacts struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	RunError error
}

// EffectResult contains safe native execution facts that are not derived from
// the structured JSONL protocol.
type EffectResult struct {
	DurationMillis int64
	Metadata       map[string]string
	Command        CommandFacts
}

// NewRegistration binds one OpenCode effect to the canonical OpenCode identity
// using structured JSONL decoding.
func NewRegistration(effect Effect) execution.Registration {
	return NewRegistrationWithMode(effect, ModeStructured)
}

// RegistrationOptions configure one OpenCode registration.
type RegistrationOptions struct {
	RequireStructuredStream bool
}

// NewRegistrationWithMode binds one OpenCode effect with an explicit output mode.
func NewRegistrationWithMode(effect Effect, mode Mode) execution.Registration {
	return NewRegistrationWithOptions(effect, mode, RegistrationOptions{})
}

// NewRegistrationWithOptions binds one OpenCode effect with explicit mode and
// fallback policy.
func NewRegistrationWithOptions(
	effect Effect,
	mode Mode,
	options RegistrationOptions,
) execution.Registration {
	if mode == "" {
		mode = ModeStructured
	}
	return execution.Registration{
		Provider: providers.IDOpenCode,
		Attempt:  newAttempt(effect, mode, options),
	}
}

func newAttempt(effect Effect, mode Mode, options RegistrationOptions) execution.Attempt {
	if effect == nil {
		return unavailableAttempt
	}
	return func(
		ctx context.Context,
		request providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		attemptMode := mode
		if mode == ModeStructured {
			if negotiator, ok := effect.(*negotiatingEffect); ok {
				attemptMode = negotiator.negotiatedMode()
			}
		}
		if attemptMode == ModeFinalOnly {
			outcome := runAttempt(ctx, request, effectForMode(effect, ModeFinalOnly), ModeFinalOnly)
			return outcome.successResult(nil)
		}

		outcome := runAttempt(ctx, request, effectForMode(effect, ModeStructured), ModeStructured)
		if !plansStructuredFallback(ctx, request, outcome, options.RequireStructuredStream) {
			return outcome.successResult(nil)
		}

		version := safeVersionContext(string(outcome.effectResult.Command.Stderr))
		if negotiator, ok := effect.(*negotiatingEffect); ok {
			negotiator.downgrade(version)
			version = negotiator.versionContext()
		}
		diagnostic := degradationDiagnostic(version)
		fallbackOutcome := runAttempt(
			ctx,
			request,
			effectForMode(effect, ModeFinalOnly),
			ModeFinalOnly,
		)
		return fallbackOutcome.successResult(&diagnostic)
	}
}

func runAttempt(
	ctx context.Context,
	request providers.ExecuteRequest,
	effect Effect,
	mode Mode,
) attemptOutcome {
	decoder := newDecoder(mode)
	effectResult, effectErr := effect.Execute(ctx, request, decoder.observe)
	flushErr := decoder.flush()
	_, _, finalErr := decoder.final()
	return attemptOutcome{
		mode:         mode,
		effectResult: effectResult,
		effectErr:    effectErr,
		flushErr:     flushErr,
		finalErr:     finalErr,
		decoder:      decoder,
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

func unavailableAttempt(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindDependency,
		Message: "OpenCode native execution is unavailable",
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
