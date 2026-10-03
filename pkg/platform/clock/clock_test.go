package clock_test

import (
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
)

func TestDeterministicAfterDeliversOnceAtLogicalDeadline(t *testing.T) {
	t.Parallel()
	base := time.Unix(42, 0).UTC()
	clock := platformclock.NewDeterministic(base, time.Second)
	after := clock.After(3 * time.Second)
	clock.SetTick(2)
	assertNoDelivery(t, after)
	clock.SetTick(3)
	assertDelivery(t, after, base.Add(3*time.Second))
	clock.SetTick(3)
	clock.SetTick(4)
	assertNoDelivery(t, after)
}

func TestDeterministicAfterImmediateDurations(t *testing.T) {
	t.Parallel()
	for _, duration := range []time.Duration{0, -time.Second} {
		t.Run(duration.String(), func(t *testing.T) {
			t.Parallel()
			clock := platformclock.NewDeterministic(time.Unix(42, 0), time.Second)
			clock.SetTick(2)
			want := clock.Now()
			after, timer := clock.After(duration), clock.NewTimer(duration)
			assertDelivery(t, after, want)
			assertDelivery(t, timer.C(), want)
			if timer.Stop() {
				t.Fatal("Stop succeeded after immediate delivery")
			}
			clock.SetTick(4)
			assertNoDelivery(t, after)
			assertNoDelivery(t, timer.C())
		})
	}
}

func TestRecordedDeterministicAfterUsesInterpolatedAndRecordedTime(t *testing.T) {
	t.Parallel()
	base := time.Unix(42, 0).UTC()
	clock := platformclock.NewRecordedDeterministic(base, time.Second, map[int]time.Time{
		0: base, 4: base.Add(8 * time.Second),
	})
	after := clock.After(3 * time.Second)
	clock.SetTick(1)
	assertNoDelivery(t, after)
	clock.SetTick(2)
	assertDelivery(t, after, base.Add(4*time.Second))
	recorded := clock.After(4 * time.Second)
	clock.SetTick(3)
	assertNoDelivery(t, recorded)
	clock.SetTick(4)
	assertDelivery(t, recorded, base.Add(8*time.Second))
	clock.SetTick(5)
	assertNoDelivery(t, after)
	assertNoDelivery(t, recorded)
}

func TestDeterministicConcurrentStopAdvanceAndAfter(t *testing.T) {
	t.Parallel()
	base := time.Unix(42, 0).UTC()
	clock := platformclock.NewDeterministic(base, time.Second)
	timer := clock.NewTimer(time.Second)
	start := make(chan struct{})
	var joined sync.WaitGroup
	joined.Add(3)
	var stopped bool
	var after <-chan time.Time
	go func() { defer joined.Done(); <-start; stopped = timer.Stop() }()
	go func() { defer joined.Done(); <-start; clock.SetTick(1) }()
	go func() { defer joined.Done(); <-start; after = clock.After(time.Second) }()
	close(start)
	joined.Wait()
	if stopped {
		assertNoDelivery(t, timer.C())
	} else {
		assertDelivery(t, timer.C(), base.Add(time.Second))
	}
	if timer.Stop() {
		t.Fatal("second Stop succeeded after stop or delivery")
	}
	// After may register before or after tick 1; tick 2 reaches either deadline.
	clock.SetTick(2)
	select {
	case got := <-after:
		if !got.Equal(base.Add(time.Second)) && !got.Equal(base.Add(2*time.Second)) {
			t.Fatalf("After delivered unexpected logical time %s", got)
		}
	default:
		t.Fatal("After did not fire by tick 2")
	}
	clock.SetTick(3)
	assertNoDelivery(t, timer.C())
	assertNoDelivery(t, after)
}

func assertDelivery(t *testing.T, channel <-chan time.Time, want time.Time) {
	t.Helper()
	select {
	case got := <-channel:
		if !got.Equal(want) {
			t.Fatalf("delivery = %s, want %s", got, want)
		}
	default:
		t.Fatal("logical timer did not deliver synchronously at its deadline")
	}
}

func assertNoDelivery(t *testing.T, channel <-chan time.Time) {
	t.Helper()
	select {
	case got := <-channel:
		t.Fatalf("unexpected timer delivery %s", got)
	default:
	}
}

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

func TestDeterministicTimerFollowsLogicalTimeAndStops(t *testing.T) {
	base := time.Date(2026, time.April, 25, 20, 59, 3, 0, time.UTC)
	clock := platformclock.NewDeterministic(base, time.Second)
	timer := clock.NewTimer(3 * time.Second)

	clock.SetTick(2)
	select {
	case <-timer.C():
		t.Fatal("timer fired before its logical deadline")
	default:
	}
	clock.SetTick(3)
	select {
	case got := <-timer.C():
		if !got.Equal(base.Add(3 * time.Second)) {
			t.Fatalf("timer time = %s, want %s", got, base.Add(3*time.Second))
		}
	case <-time.After(time.Second):
		t.Fatal("timer did not fire at its logical deadline")
	}
	if timer.Stop() {
		t.Fatal("Stop() = true after deterministic timer fired, want false")
	}

	stopped := clock.NewTimer(time.Second)
	if !stopped.Stop() {
		t.Fatal("Stop() = false for an active deterministic timer, want true")
	}
	clock.SetTick(10)
	select {
	case <-stopped.C():
		t.Fatal("stopped timer fired after logical time advanced")
	default:
	}
}
