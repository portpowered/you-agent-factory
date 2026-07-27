package cron

import "errors"

var (
	// ErrInvalidSchedule reports that a cron schedule string could not be parsed.
	ErrInvalidSchedule = errors.New("cron: invalid schedule")
	// ErrInvalidEvaluationWindow reports inverted or otherwise invalid evaluation bounds.
	ErrInvalidEvaluationWindow = errors.New("cron: invalid evaluation window")
	// ErrInvalidJitter reports negative or otherwise invalid jitter configuration.
	ErrInvalidJitter = errors.New("cron: invalid jitter")
	// ErrInvalidExpiryWindow reports non-positive or otherwise invalid expiry configuration.
	ErrInvalidExpiryWindow = errors.New("cron: invalid expiry window")
)
