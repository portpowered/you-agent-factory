package cron

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/platform/cronschedule"
)

var (
	// ErrInvalidSchedule reports that a cron schedule string could not be parsed.
	// The identity is owned by the shared cron grammar so a schedule rejected
	// while a Factory is authored and the same schedule rejected while it runs
	// compare equal under errors.Is.
	ErrInvalidSchedule = cronschedule.ErrInvalidSchedule
	// ErrInvalidEvaluationWindow reports inverted or otherwise invalid evaluation bounds.
	ErrInvalidEvaluationWindow = errors.New("cron: invalid evaluation window")
	// ErrInvalidJitter reports negative or otherwise invalid jitter configuration.
	ErrInvalidJitter = cronschedule.ErrInvalidJitter
	// ErrInvalidExpiryWindow reports non-positive or otherwise invalid expiry configuration.
	ErrInvalidExpiryWindow = cronschedule.ErrInvalidExpiryWindow
	// ErrInvalidResumeFacts reports malformed committed schedule facts supplied on resume.
	ErrInvalidResumeFacts = errors.New("cron: invalid resume facts")
	// ErrStaleResumeFacts reports resume facts that are foreign, stale, or contradict authoritative state.
	ErrStaleResumeFacts = errors.New("cron: stale resume facts")
)
