package workersessions

import (
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// TerminalOutcome is the exact two-value W2 vocabulary for a committed
// Worker Session terminal result.
type TerminalOutcome string

const (
	// TerminalOutcomeCompleted reports that the Workers result was
	// successful.
	TerminalOutcomeCompleted TerminalOutcome = "COMPLETED"
	// TerminalOutcomeFailed reports that the Workers result, the start
	// handoff, or the adapter failed. A FAILED outcome always carries a
	// non-nil FailureCause.
	TerminalOutcomeFailed TerminalOutcome = "FAILED"
)

// Valid reports whether o is one of the two accepted terminal outcomes.
func (o TerminalOutcome) Valid() bool {
	switch o {
	case TerminalOutcomeCompleted, TerminalOutcomeFailed:
		return true
	default:
		return false
	}
}

// FailureCauseKind is the bounded W2 typed vocabulary for why a Worker
// Session terminalized as FAILED.
type FailureCauseKind string

const (
	// FailureCauseStartFailure reports that the resolved attempt could not
	// be handed off: the injected workers.WorkstationExecutionService
	// returned no Workers result at all (for example, an unavailable or
	// misconfigured workstation pool), so no attempt ever ran.
	FailureCauseStartFailure FailureCauseKind = "START_FAILURE"
	// FailureCauseWorkersExecutionFailure reports that the injected Workers
	// boundary returned a WorkResult classified as failed for an ordinary
	// business or system reason, with no executor-panic evidence.
	FailureCauseWorkersExecutionFailure FailureCauseKind = "WORKERS_EXECUTION_FAILURE"
	// FailureCauseAdapterFailure reports that the injected Workers boundary
	// returned a failed WorkResult accompanied by a non-nil adapter error
	// that is not executor-panic evidence.
	FailureCauseAdapterFailure FailureCauseKind = "ADAPTER_FAILURE"
	// FailureCauseExecutorPanic reports that the failed WorkResult, or its
	// adapter error, carries the established Workers executor-panic
	// evidence. This is authoritative from the WorkResult even when the
	// adapter error is nil.
	FailureCauseExecutorPanic FailureCauseKind = "EXECUTOR_PANIC"
	// FailureCauseEventPublicationFailure reports that the Worker Session's
	// opening SESSION/STARTED record could not be committed to its Events
	// topic before Workers invocation (W3): the attempt is never handed off,
	// and the session terminalizes FAILED without ever calling Workers.
	FailureCauseEventPublicationFailure FailureCauseKind = "EVENT_PUBLICATION_FAILURE"
)

// Valid reports whether k is one of the five bounded W2+W3 failure kinds.
func (k FailureCauseKind) Valid() bool {
	switch k {
	case FailureCauseStartFailure, FailureCauseWorkersExecutionFailure, FailureCauseAdapterFailure,
		FailureCauseExecutorPanic, FailureCauseEventPublicationFailure:
		return true
	default:
		return false
	}
}

// FailureCause is the non-nil typed reason attached to every FAILED terminal
// result. Kind always reflects the true classification, including
// EXECUTOR_PANIC. Detail is a bounded, safe diagnostic string derived only
// from Workers' own closed-vocabulary structured failure classification
// (WorkResult.FailureMetadata's Family/Type) or a fixed generic placeholder
// naming the Kind. Detail never contains raw Workers WorkResult.Error text or
// a raw adapter error message: neither source is established as free of
// payloads, credentials, environment values, prompts, or raw provider
// commands, so that free-form text is never attached to Detail in any form.
// Callers must not treat Detail as a verbatim error message.
type FailureCause struct {
	Kind   FailureCauseKind
	Detail string
	// ProviderFailureKind carries the Providers-owned operational failure
	// classification for a continuation attempt when one exists. It is empty
	// for failures that did not reach a provider continuation.
	ProviderFailureKind providers.ExecuteFailureKind
	// ProviderContinuationFailureKind retains a typed invalid, foreign, or
	// stale continuation rejection without copying provider diagnostics,
	// messages, or opaque session IDs into this customer-safe snapshot.
	ProviderContinuationFailureKind providers.ContinuationFailureKind
	// ProviderContinuationOutcome retains the closed unsupported outcome. It
	// is distinct from ProviderContinuationFailureKind because Providers
	// deliberately reports unsupported as a successful capability response.
	ProviderContinuationOutcome providers.ContinuationOutcome
}

// Validate reports whether c names one of the four bounded W2 failure kinds.
// Validate is pure and does not mutate c.
func (c FailureCause) Validate() error {
	if !c.Kind.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidFailureCause, c.Kind)
	}
	providerFailureKind, continuationFailureKind, continuationOutcome := SanitizeProviderFailureClassification(
		c.ProviderFailureKind,
		c.ProviderContinuationFailureKind,
		c.ProviderContinuationOutcome,
	)
	if providerFailureKind != c.ProviderFailureKind ||
		continuationFailureKind != c.ProviderContinuationFailureKind ||
		continuationOutcome != c.ProviderContinuationOutcome {
		return fmt.Errorf("%w: invalid provider continuation classification", ErrInvalidFailureCause)
	}
	return nil
}

// SanitizeProviderFailureClassification keeps only the closed
// Providers-owned classifications that are safe to expose in a Worker Session
// terminal result. Unknown values and an impossible simultaneous continuation
// failure/outcome are discarded rather than becoming durable diagnostics.
func SanitizeProviderFailureClassification(
	providerFailureKind providers.ExecuteFailureKind,
	continuationFailureKind providers.ContinuationFailureKind,
	continuationOutcome providers.ContinuationOutcome,
) (providers.ExecuteFailureKind, providers.ContinuationFailureKind, providers.ContinuationOutcome) {
	if !validProviderFailureKind(providerFailureKind) ||
		!validProviderContinuationFailureKind(continuationFailureKind) ||
		!validProviderContinuationOutcome(continuationOutcome) ||
		(continuationFailureKind != "" && continuationOutcome != "") {
		return "", "", ""
	}
	return providerFailureKind, continuationFailureKind, continuationOutcome
}

func validProviderFailureKind(kind providers.ExecuteFailureKind) bool {
	switch kind {
	case "",
		providers.ExecuteFailureKindCanceled,
		providers.ExecuteFailureKindTimeout,
		providers.ExecuteFailureKindAuthentication,
		providers.ExecuteFailureKindInvalidRequest,
		providers.ExecuteFailureKindMisconfigured,
		providers.ExecuteFailureKindThrottled,
		providers.ExecuteFailureKindDependency,
		providers.ExecuteFailureKindUnknown,
		providers.ExecuteFailureKindSessionNotFound:
		return true
	default:
		return false
	}
}

func validProviderContinuationFailureKind(kind providers.ContinuationFailureKind) bool {
	switch kind {
	case "",
		providers.ContinuationFailureKindInvalid,
		providers.ContinuationFailureKindForeign,
		providers.ContinuationFailureKindStale:
		return true
	default:
		return false
	}
}

func validProviderContinuationOutcome(outcome providers.ContinuationOutcome) bool {
	return outcome == "" || outcome == providers.ContinuationOutcomeUnsupported
}

// TerminalResult is the detached, immutable outcome of one Worker Session
// attempt. A COMPLETED result never carries a Cause; a FAILED result always
// carries a non-nil, valid Cause.
type TerminalResult struct {
	Outcome TerminalOutcome
	Cause   *FailureCause
}

// Validate reports whether r's Outcome/Cause combination satisfies the
// terminal invariants: FAILED requires a valid non-nil Cause, and COMPLETED
// forbids one. Validate is pure and does not mutate r.
func (r TerminalResult) Validate() error {
	if !r.Outcome.Valid() {
		return fmt.Errorf("%w: invalid outcome %q", ErrInvalidTerminalResult, r.Outcome)
	}
	switch r.Outcome {
	case TerminalOutcomeFailed:
		if r.Cause == nil {
			return fmt.Errorf("%w: FAILED requires a non-nil FailureCause", ErrInvalidTerminalResult)
		}
		if err := r.Cause.Validate(); err != nil {
			return err
		}
	case TerminalOutcomeCompleted:
		if r.Cause != nil {
			return fmt.Errorf("%w: COMPLETED must not carry a FailureCause", ErrInvalidTerminalResult)
		}
	}
	return nil
}

var (
	// ErrInvalidFailureCause reports a FailureCause naming a value outside
	// the four bounded W2 failure kinds.
	ErrInvalidFailureCause = errors.New("worker session: invalid failure cause")
	// ErrInvalidTerminalResult reports a TerminalResult, or a terminal
	// Session, whose Outcome/Cause combination violates the terminal
	// invariants.
	ErrInvalidTerminalResult = errors.New("worker session: invalid terminal result")
)
