package cron

import "errors"

var (
	// ErrInvalidSchedule reports that a cron schedule string could not be parsed.
	ErrInvalidSchedule = errors.New("cron: invalid schedule")
	// ErrInvalidEvaluationWindow reports inverted or otherwise invalid evaluation bounds.
	ErrInvalidEvaluationWindow = errors.New("cron: invalid evaluation window")
)
