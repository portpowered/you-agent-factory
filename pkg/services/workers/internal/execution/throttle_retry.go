package workerexecution

import (
	"context"
	"math/rand"
	"time"

	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	// ThrottleRetryInitialDelay is the first wait after a provider reports a
	// capacity or rate-limit failure. Each later wait doubles.
	ThrottleRetryInitialDelay = 30 * time.Second
	// ThrottleRetryMaxDelay caps a single wait between throttled attempts.
	ThrottleRetryMaxDelay = 5 * time.Minute
	// ThrottleRetryWindow bounds the total time one provider attempt keeps
	// retrying a throttled provider, measured from the first throttled failure.
	ThrottleRetryWindow = 30 * time.Minute

	// throttleRetryJitterSpread keeps each wait within +/-20% of its nominal
	// value so concurrent workers do not retry in lockstep.
	throttleRetryJitterSpread = 0.2
)

// ThrottleRetryPolicy is the provider-attempt policy for throttled (capacity
// or rate-limit) failures. Provider capacity is an infrastructure condition,
// so it is retried with exponential backoff and jitter inside a long window
// instead of the short attempt budget used for other retryable failures.
type ThrottleRetryPolicy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Window       time.Duration
	// Now reads the clock that measures the window. Nil means time.Now.
	Now func() time.Time
	// Jitter returns a value in [0,1). Nil means math/rand.
	Jitter func() float64
}

// DefaultThrottleRetryPolicy returns the production throttle retry policy.
func DefaultThrottleRetryPolicy(now func() time.Time) ThrottleRetryPolicy {
	return ThrottleRetryPolicy{
		InitialDelay: ThrottleRetryInitialDelay,
		MaxDelay:     ThrottleRetryMaxDelay,
		Window:       ThrottleRetryWindow,
		Now:          now,
	}
}

// IsThrottleFailure reports whether the provider error is a throttled
// (capacity or rate-limit) failure that the throttle retry policy owns.
func IsThrottleFailure(providerErr *workers.ProviderError) bool {
	if providerErr == nil {
		return false
	}
	return workers.WorkFailureDecisionFromProviderError(providerErr).TriggersThrottlePause
}

// ThrottleRetry tracks one provider attempt's progress through the throttle
// retry window. It is request-scoped and must not be shared.
type ThrottleRetry struct {
	policy  ThrottleRetryPolicy
	started time.Time
	begun   bool
	waits   int
}

// NewThrottleRetry starts a request-scoped throttle retry tracker.
func NewThrottleRetry(policy ThrottleRetryPolicy) *ThrottleRetry {
	return &ThrottleRetry{policy: policy}
}

func (r *ThrottleRetry) now() time.Time {
	if r.policy.Now != nil {
		return r.policy.Now()
	}
	return time.Now()
}

// Wait sleeps before the next throttled attempt using sleep. It reports false
// without sleeping when the retry window is already exhausted. The final wait
// is shortened so the last attempt lands at the end of the window.
func (r *ThrottleRetry) Wait(
	ctx context.Context,
	sleep func(context.Context, time.Duration) error,
) (bool, error) {
	now := r.now()
	if !r.begun {
		r.begun = true
		r.started = now
	}
	remaining := r.policy.Window - now.Sub(r.started)
	if remaining <= 0 {
		return false, nil
	}
	delay := r.nextDelay()
	if delay > remaining {
		delay = remaining
	}
	if err := sleep(ctx, delay); err != nil {
		return false, err
	}
	return true, nil
}

func (r *ThrottleRetry) nextDelay() time.Duration {
	delay := r.policy.InitialDelay
	for i := 0; i < r.waits && delay < r.policy.MaxDelay; i++ {
		delay *= 2
	}
	r.waits++
	if delay > r.policy.MaxDelay {
		delay = r.policy.MaxDelay
	}
	jitter := rand.Float64
	if r.policy.Jitter != nil {
		jitter = r.policy.Jitter
	}
	scaled := time.Duration(float64(delay) * (1 - throttleRetryJitterSpread + 2*throttleRetryJitterSpread*jitter()))
	if scaled > r.policy.MaxDelay {
		scaled = r.policy.MaxDelay
	}
	return scaled
}
