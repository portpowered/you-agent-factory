package replay

import (
	"sort"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const defaultLogicalTickDuration = time.Millisecond

// DeterministicClock maps engine ticks to deterministic timestamps for replay.
type DeterministicClock struct {
	mu           sync.Mutex
	base         time.Time
	tickDuration time.Duration
	tick         int
	tickTimes    map[int]time.Time
	knownTicks   []int
}

var _ factory.LogicalClock = (*DeterministicClock)(nil)

// NewDeterministicClock returns a replay clock whose Now value is derived from
// the current logical tick instead of host wall-clock time.
func NewDeterministicClock(base time.Time, tickDuration time.Duration) *DeterministicClock {
	if base.IsZero() {
		base = time.Unix(0, 0).UTC()
	}
	if tickDuration <= 0 {
		tickDuration = defaultLogicalTickDuration
	}
	return &DeterministicClock{
		base:         base.UTC(),
		tickDuration: tickDuration,
	}
}

// NewArtifactClock returns a replay clock aligned to recorded event times when
// the artifact includes wall-clock timestamps for specific ticks.
func NewArtifactClock(artifact *interfaces.ReplayArtifact) *DeterministicClock {
	if artifact == nil {
		return NewDeterministicClock(time.Time{}, 0)
	}
	clock := NewDeterministicClock(artifact.RecordedAt, 0)
	clock.tickTimes, clock.knownTicks = recordedTickTimes(artifact)
	return clock
}

// Now returns the deterministic timestamp for the current logical tick.
func (c *DeterministicClock) Now() time.Time {
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

// SetTick updates the logical tick used by Now.
func (c *DeterministicClock) SetTick(tick int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if tick < 0 {
		tick = 0
	}
	c.tick = tick
}

func (c *DeterministicClock) previousTickTimeLocked(tick int) (int, time.Time, bool) {
	index := sort.Search(len(c.knownTicks), func(i int) bool {
		return c.knownTicks[i] > tick
	}) - 1
	if index < 0 {
		return 0, time.Time{}, false
	}
	knownTick := c.knownTicks[index]
	return knownTick, c.tickTimes[knownTick], true
}

func (c *DeterministicClock) nextTickTimeLocked(tick int) (int, time.Time, bool) {
	index := sort.Search(len(c.knownTicks), func(i int) bool {
		return c.knownTicks[i] > tick
	})
	if index >= len(c.knownTicks) {
		return 0, time.Time{}, false
	}
	knownTick := c.knownTicks[index]
	return knownTick, c.tickTimes[knownTick], true
}

func recordedTickTimes(artifact *interfaces.ReplayArtifact) (map[int]time.Time, []int) {
	if artifact == nil {
		return nil, nil
	}
	tickTimes := make(map[int]time.Time)
	recordTime := artifact.RecordedAt.UTC()
	if !recordTime.IsZero() {
		tickTimes[0] = recordTime
	}
	for _, event := range artifact.Events {
		eventTime := event.Context.EventTime.UTC()
		if eventTime.IsZero() {
			continue
		}
		tick := event.Context.Tick
		existing, exists := tickTimes[tick]
		if !exists || eventTime.Before(existing) {
			tickTimes[tick] = eventTime
		}
	}
	if len(tickTimes) == 0 {
		return nil, nil
	}
	knownTicks := make([]int, 0, len(tickTimes))
	for tick := range tickTimes {
		knownTicks = append(knownTicks, tick)
	}
	sort.Ints(knownTicks)
	return tickTimes, knownTicks
}
