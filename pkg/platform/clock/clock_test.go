package clock_test

import (
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
)

func TestRealReadsCurrentWallClock(t *testing.T) {
	before := time.Now()
	got := (platformclock.Real{}).Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("Now() = %s, want time between %s and %s", got, before, after)
	}
}

func TestEnsureUsesRealClockOnlyForMissingSource(t *testing.T) {
	fixed := platformclock.NewDeterministic(time.Unix(42, 0), time.Second)
	if got := platformclock.Ensure(fixed); got != fixed {
		t.Fatal("Ensure replaced the supplied clock")
	}
	if got := platformclock.Ensure(nil); got == nil {
		t.Fatal("Ensure(nil) returned nil")
	}
}

func TestDeterministicAdvancesFromLogicalTick(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	clock := platformclock.NewDeterministic(base, 10*time.Millisecond)

	if got := clock.Now(); !got.Equal(base) {
		t.Fatalf("initial Now() = %s, want %s", got, base)
	}

	clock.SetTick(3)
	want := base.Add(30 * time.Millisecond)
	if got := clock.Now(); !got.Equal(want) {
		t.Fatalf("tick 3 Now() = %s, want %s", got, want)
	}

	clock.SetTick(-1)
	if got := clock.Now(); !got.Equal(base) {
		t.Fatalf("negative tick Now() = %s, want clamped base %s", got, base)
	}
}

func TestRecordedDeterministicClonesAndInterpolatesTickTimes(t *testing.T) {
	base := time.Date(2026, time.April, 25, 20, 59, 3, 0, time.UTC)
	tickFour := base.Add(time.Minute)
	tickEight := tickFour.Add(40 * time.Second)
	tickTimes := map[int]time.Time{4: tickFour, 8: tickEight}
	clock := platformclock.NewRecordedDeterministic(base, time.Millisecond, tickTimes)
	delete(tickTimes, 4)

	clock.SetTick(4)
	if got := clock.Now(); !got.Equal(tickFour) {
		t.Fatalf("tick 4 Now() = %s, want cloned time %s", got, tickFour)
	}

	clock.SetTick(6)
	want := tickFour.Add(20 * time.Second)
	if got := clock.Now(); !got.Equal(want) {
		t.Fatalf("tick 6 Now() = %s, want interpolated time %s", got, want)
	}
}
