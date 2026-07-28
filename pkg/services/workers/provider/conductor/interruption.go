package conductor

import (
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// Symbolic interruption and retry diagnostics owned by the conductor.
const (
	InvariantCanceled = "canceled"
	InvariantTimeout  = "timeout"
)

const (
	canceledFailureMessage = "provider invocation was canceled"
	timeoutFailureMessage  = "provider invocation timed out"
)

// RetryHandoff is the provider-neutral retryability signal exposed from a
// normalized conductor failure. Shared orchestration must not branch on
// concrete built-in provider names to decide retryability.
type RetryHandoff struct {
	Retryable bool
}

// RetryHandoffFromFailure projects only the provider-neutral retryability
// signal from a conductor-normalized failure outcome.
func RetryHandoffFromFailure(failure inference.Failure) RetryHandoff {
	return RetryHandoff{Retryable: failure.Retryable()}
}

func normalizeConductorFailure(failure inference.Failure) inference.Failure {
	switch failure.Kind() {
	case inference.FailureCanceled:
		return interruptionFailure(
			inference.FailureCanceled,
			false,
			InvariantCanceled,
			canceledFailureMessage,
			failure.ProviderSession(),
		)
	case inference.FailureTimeout:
		return interruptionFailure(
			inference.FailureTimeout,
			true,
			InvariantTimeout,
			timeoutFailureMessage,
			failure.ProviderSession(),
		)
	default:
		return sanitizeFailure(failure)
	}
}

func interruptionFailure(
	kind inference.FailureKind,
	retryable bool,
	invariant string,
	message string,
	providerSession *inference.ProviderSession,
) inference.Failure {
	return inference.NewFailure(inference.FailureInput{
		Kind:            kind,
		Message:         message,
		Retryable:       retryable,
		ProviderSession: providerSession,
		Diagnostics: map[string]string{
			"invariant": invariant,
		},
	})
}
