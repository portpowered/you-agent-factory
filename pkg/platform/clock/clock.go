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

// Timer is the cancellable timer seam used by services that need an
// attempt-scoped deadline without coupling their policy to the host clock.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

// TimerSource optionally supplies timers that advance with the same logical
// source as Now. Sources that only implement Source remain valid and receive
// a real host timer at the consuming service boundary.
type TimerSource interface {
	Source
	NewTimer(time.Duration) Timer
}

// Real reads the host wall clock.
type Real struct{}

var _ Source = Real{}
var _ TimerSource = Real{}

// Now returns the current wall-clock time.
func (Real) Now() time.Time {
	return time.Now()
}

// NewTimer returns a cancellable host timer.
func (Real) NewTimer(duration time.Duration) Timer {
	return realTimer{timer: time.NewTimer(duration)}
}

// After reports the passage of a duration using the host wall clock.
func (Real) After(duration time.Duration) <-chan time.Time {
	return time.After(duration)
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
	timers       map[*deterministicTimer]struct{}
}

var _ Source = (*Deterministic)(nil)
var _ TimerSource = (*Deterministic)(nil)

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
	return c.nowLocked()
}

func (c *Deterministic) nowLocked() time.Time {
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

// NewTimer returns a deterministic timer whose delivery is driven by
// SetTick. It makes controlled-time supervision tests deterministic without
// sleeps or host-time padding.
func (c *Deterministic) NewTimer(duration time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timers == nil {
		c.timers = make(map[*deterministicTimer]struct{})
	}
	timer := &deterministicTimer{
		clock:    c,
		deadline: c.nowLocked().Add(duration),
		channel:  make(chan time.Time, 1),
	}
	c.timers[timer] = struct{}{}
	c.fireDueTimersLocked(c.nowLocked())
	return timer
}

// SetTick updates the logical tick used by Now. Negative ticks clamp to zero.
func (c *Deterministic) SetTick(tick int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if tick < 0 {
		tick = 0
	}
	c.tick = tick
	c.fireDueTimersLocked(c.nowLocked())
}

func (c *Deterministic) fireDueTimersLocked(now time.Time) {
	for timer := range c.timers {
		if now.Before(timer.deadline) {
			continue
		}
		timer.fired = true
		delete(c.timers, timer)
		timer.channel <- now
	}
}

type realTimer struct {
	timer *time.Timer
}

func (timer realTimer) C() <-chan time.Time { return timer.timer.C }

func (timer realTimer) Stop() bool { return timer.timer.Stop() }

type deterministicTimer struct {
	clock    *Deterministic
	deadline time.Time
	channel  chan time.Time
	fired    bool
	stopped  bool
}

func (timer *deterministicTimer) C() <-chan time.Time { return timer.channel }

func (timer *deterministicTimer) Stop() bool {
	if timer == nil || timer.clock == nil {
		return false
	}
	timer.clock.mu.Lock()
	defer timer.clock.mu.Unlock()
	if timer.fired || timer.stopped {
		return false
	}
	timer.stopped = true
	delete(timer.clock.timers, timer)
	return true
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
