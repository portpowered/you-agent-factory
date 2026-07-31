// Package execution defines the parent-private Providers execution service.
package execution

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Service performs one provider-neutral adapter attempt for an explicit
// Providers-owned request.
type Service interface {
	Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error)
}

// Attempt is the private adapter seam for one normalized provider invocation.
// It deliberately carries no retry, fallback, scheduling, or throttle policy.
type Attempt func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error)

// AttemptFailure carries the competing lifecycle facts from one private
// adapter attempt. Execution applies one deterministic precedence rule and
// never exposes these native errors to peers.
type AttemptFailure struct {
	Declared        *providers.ExecuteFailure
	SessionRef      *providers.SessionRef
	NativeError     error
	DecodeError     error
	FlushError      error
	FinalParseError error
}

func (failure AttemptFailure) Error() string {
	return "private provider adapter attempt failed"
}

func (failure AttemptFailure) Unwrap() []error {
	causes := make([]error, 0, 4)
	for _, cause := range []error{
		failure.FinalParseError, failure.FlushError,
		failure.DecodeError, failure.NativeError,
	} {
		if cause != nil {
			causes = append(causes, cause)
		}
	}
	return causes
}

// Registration binds one canonical Providers identity to one private adapter
// attempt.
type Registration struct {
	Provider providers.ID
	Attempt  Attempt
}
