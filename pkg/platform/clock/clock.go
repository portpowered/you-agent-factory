// Package clock provides policy-free wall and deterministic time sources for
// domain-owned clock contracts.
package clock

import (
	"sort"
	"sync"
	"time"
)

const defaultLogicalTickDuration = time.Millisecond

// Source is the minimal clock contract implemented by platform clocks.
// Domain packages should define their own consuming contracts with the same
// method and receive these implementations through composition.
type Source interface {
	Now() time.Time
}

// Real reads the host wall clock.
type Real struct{}

var _ Source = Real{}

// Now returns the current wall-clock time.
func (Real) Now() time.Time {
	return time.Now()
}

// Ensure returns a real clock when the supplied source is nil.
func Ensure(source Source) Source {
	if source == nil {
		return Real{}
	}
	return source
}

// Deterministic maps logical ticks to stable timestamps.
type Deterministic struct {
	mu           sync.Mutex
	base         time.Time
	tickDuration time.Duration
	tick         int
	tickTimes    map[int]time.Time
	knownTicks   []int
}

var _ Source = (*Deterministic)(nil)

// NewDeterministic returns a clock whose time advances from a fixed base by
// one duration per logical tick.
func NewDeterministic(base time.Time, tickDuration time.Duration) *Deterministic {
	if base.IsZero() {
		base = time.Unix(0, 0).UTC()
	}
	if tickDuration <= 0 {
		tickDuration = defaultLogicalTickDuration
	}
	return &Deterministic{
		base:         base.UTC(),
		tickDuration: tickDuration,
	}
}

// NewRecordedDeterministic returns a deterministic clock aligned to explicit
// logical-tick timestamps. The input map is cloned so callers retain ownership.
func NewRecordedDeterministic(
	base time.Time,
	tickDuration time.Duration,
	tickTimes map[int]time.Time,
) *Deterministic {
	clock := NewDeterministic(base, tickDuration)
	if len(tickTimes) == 0 {
		return clock
	}
	clock.tickTimes = make(map[int]time.Time, len(tickTimes))
	clock.knownTicks = make([]int, 0, len(tickTimes))
	for tick, tickTime := range tickTimes {
		clock.tickTimes[tick] = tickTime.UTC()
		clock.knownTicks = append(clock.knownTicks, tick)
	}
	sort.Ints(clock.knownTicks)
	return clock
}

// Now returns the deterministic timestamp for the current logical tick.
func (c *Deterministic) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.knownTicks) > 0 {
		if exact, ok := c.tickTimes[c.tick]; ok {
			return exact
		}
		prevTick, prevTime, hasPrev := c.previousTickTimeLocked(c.tick)
		nextTick, nextTime, hasNext := c.nextTickTimeLocked(c.tick)
		switch {
		case hasPrev && hasNext && nextTick > prevTick:
			perTick := nextTime.Sub(prevTime) / time.Duration(nextTick-prevTick)
			if perTick <= 0 {
				perTick = c.tickDuration
			}
			return prevTime.Add(time.Duration(c.tick-prevTick) * perTick)
		case hasPrev:
			return prevTime.Add(time.Duration(c.tick-prevTick) * c.tickDuration)
		}
	}
	return c.base.Add(time.Duration(c.tick) * c.tickDuration)
}

// SetTick updates the logical tick used by Now. Negative ticks clamp to zero.
func (c *Deterministic) SetTick(tick int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if tick < 0 {
		tick = 0
	}
	c.tick = tick
}

func (c *Deterministic) previousTickTimeLocked(tick int) (int, time.Time, bool) {
	index := sort.Search(len(c.knownTicks), func(i int) bool {
		return c.knownTicks[i] > tick
	}) - 1
	if index < 0 {
		return 0, time.Time{}, false
	}
	knownTick := c.knownTicks[index]
	return knownTick, c.tickTimes[knownTick], true
}

func (c *Deterministic) nextTickTimeLocked(tick int) (int, time.Time, bool) {
	index := sort.Search(len(c.knownTicks), func(i int) bool {
		return c.knownTicks[i] > tick
	})
	if index >= len(c.knownTicks) {
		return 0, time.Time{}, false
	}
	knownTick := c.knownTicks[index]
	return knownTick, c.tickTimes[knownTick], true
}
