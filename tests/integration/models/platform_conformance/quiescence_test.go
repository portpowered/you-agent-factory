package platform_conformance

import (
	"errors"
	"time"
)

const controlledCleanupCeilingMultiplier = 4

type controlledCleanupCeilingError struct{}

func (controlledCleanupCeilingError) Error() string {
	return "controlled owned resources did not reach quiescence before cleanup ceiling"
}

func (runner ControlledRunner) observeControlledQuiescence(spec RunSpec, attempt *controlledAttempt, listenerObserved bool) {
	sample := func() ReleaseEvidence {
		return runner.releaseEvidence(
			spec, attempt.process, attempt.tree, attempt.treeAttached, attempt.waitCompleted,
			attempt.cleanupErr, listenerObserved,
		)
	}
	attempt.release = sample()
	if controlledReleaseResourcesClosed(attempt.release) {
		return
	}
	if runner.waitDelay <= 0 {
		attempt.cleanupErr = errors.Join(attempt.cleanupErr, controlledCleanupCeilingError{})
		return
	}

	// Direct Wait only covers the root process. The attached PGID and declared
	// listener have no completion channel, so bounded observation is required;
	// the first sample above remains immediate and the timer is only a safety
	// ceiling, not a fixed synchronization delay.
	deadline := time.NewTimer(controlledCleanupCeiling(runner.waitDelay))
	defer deadline.Stop()
	interval := time.NewTicker(runner.waitDelay)
	defer interval.Stop()
	for {
		select {
		case <-interval.C:
			attempt.release = sample()
			if controlledReleaseResourcesClosed(attempt.release) {
				return
			}
		case <-deadline.C:
			attempt.release = sample()
			if controlledReleaseResourcesClosed(attempt.release) {
				return
			}
			attempt.cleanupErr = errors.Join(attempt.cleanupErr, controlledCleanupCeilingError{})
			return
		}
	}
}

func controlledCleanupCeiling(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	return delay * controlledCleanupCeilingMultiplier
}

func controlledReleaseResourcesClosed(release ReleaseEvidence) bool {
	return release.OwnedProcesses == 0 && release.OwnedListeners == 0
}

func isControlledCleanupCeiling(err error) bool {
	var ceiling controlledCleanupCeilingError
	return errors.As(err, &ceiling)
}
