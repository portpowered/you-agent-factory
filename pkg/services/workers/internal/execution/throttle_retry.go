package workerexecution

import (
	"context"
	"hash/fnv"
	"strconv"
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
	// Now reads the injected clock that measures the window. Nil measures the
	// window by the total time spent waiting instead.
	Now func() time.Time
	// JitterSeed (typically the dispatch ID) deterministically spreads waits
	// across concurrent workers without a hidden random source. An empty seed
	// means no jitter.
	JitterSeed string
	// Jitter overrides the seeded jitter with a value in [0,1) for the given
	// zero-based wait index. Tests use it for exact waits.
	Jitter func(wait int) float64
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
	waited  time.Duration
}

// NewThrottleRetry starts a request-scoped throttle retry tracker.
func NewThrottleRetry(policy ThrottleRetryPolicy) *ThrottleRetry {
	return &ThrottleRetry{policy: policy}
}

// elapsed reports how much of the window has been used. With a clock it is
// the time since the first throttled failure; without one it is the total time
// spent waiting, which keeps the window bounded with no ambient clock.
func (r *ThrottleRetry) elapsed() time.Duration {
	if r.policy.Now == nil {
		return r.waited
	}
	now := r.policy.Now()
	if !r.begun {
		r.begun = true
		r.started = now
	}
	return now.Sub(r.started)
}

// Wait sleeps before the next throttled attempt using sleep. It reports false
// without sleeping when the retry window is already exhausted. The final wait
// is shortened so the last attempt lands at the end of the window.
func (r *ThrottleRetry) Wait(
	ctx context.Context,
	sleep func(context.Context, time.Duration) error,
) (bool, error) {
	remaining := r.policy.Window - r.elapsed()
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
	r.waited += delay
	return true, nil
}

func (r *ThrottleRetry) nextDelay() time.Duration {
	delay := r.policy.InitialDelay
	for i := 0; i < r.waits && delay < r.policy.MaxDelay; i++ {
		delay *= 2
	}
	wait := r.waits
	r.waits++
	if delay > r.policy.MaxDelay {
		delay = r.policy.MaxDelay
	}
	scaled := time.Duration(float64(delay) * (1 - throttleRetryJitterSpread + 2*throttleRetryJitterSpread*r.jitter(wait)))
	if scaled > r.policy.MaxDelay {
		scaled = r.policy.MaxDelay
	}
	return scaled
}

// jitter returns a fraction in [0,1) for the given wait index. It is stable
// for one seed and wait, and differs across seeds so concurrent workers that
// hit the same capacity event do not retry in lockstep.
func (r *ThrottleRetry) jitter(wait int) float64 {
	if r.policy.Jitter != nil {
		return r.policy.Jitter(wait)
	}
	if r.policy.JitterSeed == "" {
		return 0.5
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(r.policy.JitterSeed + "/" + strconv.Itoa(wait)))
	return float64(hash.Sum32()) / (1 << 32)
}
