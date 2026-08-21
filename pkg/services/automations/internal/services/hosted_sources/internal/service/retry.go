package service

import (
	"errors"
	"time"

	hostedlinear "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/internal/linear"
)

const (
	restartBackoffMin = 25 * time.Millisecond
	restartBackoffMax = 60 * time.Second
	retryDelayFloor   = 1 * time.Millisecond
	// Local retry jitter is bounded to +/-20% of the computed delay, then
	// clamped to the positive floor and 60-second ceiling below.
	retryJitterRatio = 20
)

// retryJitter is the replaceable randomness boundary for local retry delays.
// It receives the bounded exponential delay and returns a jittered candidate;
// jitteredRetryBackoff clamps that candidate before it reaches the clock.
type retryJitter func(time.Duration) time.Duration

// RetryRandomSource supplies bounded entropy to the hosted retry policy. Wire
// selects the platform implementation; tests can replace it deterministically.
type RetryRandomSource interface {
	Int63n(upperBound int64) (int64, error)
}

func retryDelay(consecutiveFailures int, runErr error, jitter retryJitter) (time.Duration, string) {
	var rateLimitErr *hostedlinear.RateLimitError
	if errors.As(runErr, &rateLimitErr) {
		if providerDelay, ok := rateLimitErr.RetryDelay(); ok {
			return providerDelay, "provider"
		}
	}
	return jitteredRetryBackoff(consecutiveFailures, jitter), "computed"
}

func restartBackoff(consecutiveFailures int) time.Duration {
	if consecutiveFailures <= 1 {
		return restartBackoffMin
	}
	backoff := restartBackoffMin
	for failure := 1; failure < consecutiveFailures; failure++ {
		if backoff >= restartBackoffMax/2 {
			return restartBackoffMax
		}
		backoff *= 2
	}
	return backoff
}

func jitteredRetryBackoff(consecutiveFailures int, jitter retryJitter) time.Duration {
	base := restartBackoff(consecutiveFailures)
	if jitter == nil {
		return clampRetryDelay(base)
	}
	return clampRetryDelay(jitter(base))
}

func retryJitterFromRandomSource(source RetryRandomSource) retryJitter {
	if source == nil {
		return nil
	}
	return func(base time.Duration) time.Duration {
		return defaultRetryJitter(source, base)
	}
}

func defaultRetryJitter(source RetryRandomSource, base time.Duration) time.Duration {
	base = clampRetryDelay(base)
	if source == nil {
		return base
	}
	maxJitter := base * retryJitterRatio / 100
	minimum := clampRetryDelay(base - maxJitter)
	maximum := clampRetryDelay(base + maxJitter)
	if maximum < minimum {
		maximum = minimum
	}

	span := maximum - minimum
	if span == 0 {
		return minimum
	}
	sample, err := source.Int63n(int64(span) + 1)
	if err != nil || sample < 0 || sample > int64(span) {
		return base
	}
	return minimum + time.Duration(sample)
}

func clampRetryDelay(delay time.Duration) time.Duration {
	switch {
	case delay < retryDelayFloor:
		return retryDelayFloor
	case delay > restartBackoffMax:
		return restartBackoffMax
	default:
		return delay
	}
}
