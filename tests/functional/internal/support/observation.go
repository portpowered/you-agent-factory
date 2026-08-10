package support

import (
	"fmt"
	"time"
)

const observationPollInterval = 10 * time.Millisecond

// WaitForObservation evaluates an observable condition immediately and then
// at a short bounded interval until it is accepted or the deadline expires.
// Callers receive the last observation with timeout diagnostics so a failed
// functional wait explains what the public boundary most recently reported.
// These waits are for asynchronously committed public projections that do not
// expose one deterministic signal covering the requested observation; using a
// fake edge would skip the runtime behavior the functional scenario proves.
func WaitForObservation[T any](
	timeout time.Duration,
	observe func() (T, error),
	accept func(T) bool,
) (T, error) {
	return waitForObservationAtInterval(timeout, observationPollInterval, observe, accept)
}

func waitForObservationAtInterval[T any](
	timeout time.Duration,
	interval time.Duration,
	observe func() (T, error),
	accept func(T) bool,
) (T, error) {
	var last T
	var lastErr error
	if timeout <= 0 {
		return last, fmt.Errorf("observation timeout must be positive: %s", timeout)
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		value, err := observe()
		last = value
		lastErr = err
		if err == nil && accept(value) {
			return value, nil
		}

		select {
		case <-ticker.C:
		case <-deadline.C:
			return last, fmt.Errorf(
				"timed out after %s: last observation=%#v; last error=%v",
				timeout,
				last,
				lastErr,
			)
		}
	}
}
