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

// ContinuationService extends ordinary execution with the private exact-session
// continuation seam used only by the Providers root.
type ContinuationService interface {
	Service
	Continue(context.Context, ContinuationRequest) (providers.ExecuteResult, error)
}

// Attempt is the private adapter seam for one normalized provider invocation.
// It deliberately carries no retry, fallback, scheduling, or throttle policy.
type Attempt func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error)

// ContinuationRequest is the parent-private adapter input for a continued
// attempt. It intentionally embeds the public ordinary ExecuteRequest while
// keeping the prior Provider Session reference inside Providers internals.
type ContinuationRequest struct {
	providers.ExecuteRequest
	ResumeSession *providers.SessionRef
}

// Clone returns a detached continuation-attempt input.
func (request ContinuationRequest) Clone() ContinuationRequest {
	cloned := request
	cloned.ExecuteRequest = request.ExecuteRequest.Clone()
	if request.ResumeSession != nil {
		resume := request.ResumeSession.Clone()
		cloned.ResumeSession = &resume
	}
	return cloned
}

// Validate checks the public attempt fields and the Providers-private exact
// session reference before an adapter is invoked.
func (request ContinuationRequest) Validate() error {
	if err := request.ExecuteRequest.Validate(); err != nil {
		return err
	}
	if request.ResumeSession == nil {
		return providers.ErrInvalidSessionRef
	}
	return request.ResumeSession.Validate()
}

// ContinuationAttempt is the private adapter seam for one exact-session
// continuation. It deliberately carries no retry, fallback, scheduling, or
// throttle policy.
type ContinuationAttempt func(context.Context, ContinuationRequest) (providers.ExecuteResult, error)

// AttemptFailure carries the competing lifecycle facts and optional detached
// session identity from one private adapter attempt. Execution applies one
// deterministic precedence rule and never exposes these native errors to
// peers.
type AttemptFailure struct {
	Declared   *providers.ExecuteFailure
	SessionRef *providers.SessionRef
	// Diagnostics carries bounded adapter facts such as inspection limits and
	// progress truncation through execution normalization. It intentionally
	// contains no native error text or raw provider payload.
	Diagnostics     *providers.ExecuteDiagnostics
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
	Continue ContinuationAttempt
}
