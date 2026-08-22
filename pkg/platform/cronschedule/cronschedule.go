// Package cronschedule is the policy-free cron expression and timing-field
// grammar shared by the services that author and the services that execute
// cron schedules.
//
// It owns exactly the syntax layer: whether a cron expression parses and what
// its next fire instant is, and whether an authored jitter or expiry-window
// duration string is well formed. It holds no schedule state, no clock, and no
// product policy, so both the Factory Definitions validation path (which
// rejects malformed authored cron configuration) and the Automations cron
// service (which evaluates and fires schedules) can depend on one definition of
// the grammar without either service depending on the other.
//
// Callers pass the authored field values as plain strings. Product types that
// carry those fields, and the decision of when a field is required, stay with
// the owning service.
package cronschedule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	cronparser "github.com/robfig/cron/v3"
)

var (
	// ErrInvalidSchedule reports that a cron schedule string could not be parsed.
	ErrInvalidSchedule = errors.New("cron: invalid schedule")
	// ErrInvalidJitter reports negative or otherwise invalid jitter configuration.
	ErrInvalidJitter = errors.New("cron: invalid jitter")
	// ErrInvalidExpiryWindow reports non-positive or otherwise invalid expiry configuration.
	ErrInvalidExpiryWindow = errors.New("cron: invalid expiry window")
)

// ValidateSchedule reports whether a cron schedule expression is well formed.
// It is the syntax check an authoring path runs before a schedule is ever
// scheduled, and it is defined as "a next fire time can be computed", which is
// the same question the executing path asks.
func ValidateSchedule(schedule string) error {
	_, err := NextFire(schedule, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
	return err
}

// NextFire returns the first instant strictly after `after` at which schedule
// fires, in UTC. A schedule without an explicit timezone prefix is interpreted
// in UTC rather than in the host's local zone, so the same authored expression
// resolves identically on every machine.
func NextFire(schedule string, after time.Time) (time.Time, error) {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return time.Time{}, fmt.Errorf("%w: schedule is required", ErrInvalidSchedule)
	}
	after = after.UTC()
	parseInput := schedule
	if !strings.HasPrefix(schedule, "TZ=") && !strings.HasPrefix(schedule, "CRON_TZ=") {
		parseInput = "CRON_TZ=UTC " + schedule
	}
	parsed, err := cronparser.ParseStandard(parseInput)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid cron schedule %q: %v", ErrInvalidSchedule, schedule, err)
	}
	next := parsed.Next(after)
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("%w: cron schedule %q produced no next fire", ErrInvalidSchedule, schedule)
	}
	return next.UTC(), nil
}

// ParseJitter parses an authored jitter duration. An empty value means no
// jitter rather than a malformed one, so the zero duration is returned without
// an error. A negative duration is rejected.
func ParseJitter(jitter string) (time.Duration, error) {
	if jitter == "" {
		return 0, nil
	}
	parsed, err := time.ParseDuration(jitter)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidJitter, err)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("%w: duration must be non-negative", ErrInvalidJitter)
	}
	return parsed, nil
}

// ParseExpiryWindow parses an authored expiry-window duration, falling back to
// defaultWindow when the field is unset. Unlike jitter an expiry window must be
// strictly positive, and an unset field with a non-positive default is a caller
// error rather than a silently unbounded window.
func ParseExpiryWindow(expiryWindow string, defaultWindow time.Duration) (time.Duration, error) {
	if expiryWindow == "" {
		if defaultWindow <= 0 {
			return 0, fmt.Errorf("%w: schedule window default must be positive", ErrInvalidExpiryWindow)
		}
		return defaultWindow, nil
	}
	parsed, err := time.ParseDuration(expiryWindow)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidExpiryWindow, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%w: duration must be positive", ErrInvalidExpiryWindow)
	}
	return parsed, nil
}
