package factory

import (
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
)

// Clock is the runtime time source used by replay-sensitive factory paths.
type Clock interface {
	Now() time.Time
}

// LogicalClock is a clock that can align itself to the current engine tick.
type LogicalClock interface {
	Clock
	SetTick(tick int)
}

// EnsureClock returns a real clock when the supplied clock is nil.
func EnsureClock(clock Clock) Clock {
	return platformclock.Ensure(clock)
}
