package factory

import platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"

// Clock is the runtime time source used by replay-sensitive factory paths.
type Clock = platformclock.Source

// LogicalClock is a clock that can align itself to the current engine tick.
type LogicalClock interface {
	Clock
	SetTick(tick int)
}

// ClockResolver applies the process-selected default only when an operation
// does not supply a bounded clock override.
type ClockResolver func(Clock) Clock

// IDGenerator produces opaque runtime identities. Wire selects its production
// implementation; Factory Runtime never chooses an ambient UUID source.
type IDGenerator func() string
