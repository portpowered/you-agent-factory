package linear

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrWorkAdmission reports that Work admission failed for a generated hosted-source
	// request. The wrapped error preserves Work-owned rejection semantics without
	// reinterpreting them inside hosted_sources.
	ErrWorkAdmission = errors.New("hosted sources: work admission failed")
	// ErrSecretResolution reports that hosted-source credential resolution failed.
	// The wrapped error preserves the underlying resolution cause without performing
	// Work admission or reporting successful observation convergence.
	ErrSecretResolution = errors.New("hosted sources: secret resolution failed")
	// ErrRateLimited classifies a provider response that asks the poller to
	// reduce its request rate.
	ErrRateLimited = errors.New("hosted linear: rate limited")
)

// RateLimitError carries only safe retry metadata from a Linear rate-limit
// response. Provider response bodies and messages are intentionally omitted.
type RateLimitError struct {
	RetryAfter    time.Duration
	HasRetryAfter bool
}

func (e *RateLimitError) Error() string {
	if e == nil || !e.HasRetryAfter {
		return ErrRateLimited.Error()
	}
	return fmt.Sprintf("%s: retry after %s", ErrRateLimited, e.RetryAfter)
}

func (e *RateLimitError) Unwrap() error {
	return ErrRateLimited
}

// RetryDelay returns provider guidance only when it is safe to use as a
// duration. A malformed or otherwise unusable value therefore falls back to
// the supervisor's local retry policy.
func (e *RateLimitError) RetryDelay() (time.Duration, bool) {
	if e == nil || !e.HasRetryAfter || e.RetryAfter < 0 {
		return 0, false
	}
	return e.RetryAfter, true
}
