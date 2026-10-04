package wire

import (
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

type subprocessTimerRecorder struct {
	platformclock.TimerSource
	duration time.Duration
}

func (clock *subprocessTimerRecorder) After(duration time.Duration) <-chan time.Time {
	clock.duration = duration
	return clock.TimerSource.After(duration)
}

func selectedSubprocessClock(t *testing.T, edges serviceedges.Edges) platformprocess.Clock {
	t.Helper()
	runner, err := providePlatformProcessCommandRunner(edges)
	if err != nil {
		t.Fatal(err)
	}
	return runner.(platformprocess.ExecCommandRunner).Clock
}

func assertSubprocessTimerPending(t *testing.T, timer <-chan time.Time) {
	t.Helper()
	select {
	case at := <-timer:
		t.Fatalf("timer delivered early at %v", at)
	default:
	}
}

func TestPlatformProcessClockSelection(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"same", "distinct", "override", "legacy explicit"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			wall := platformclock.NewDeterministic(time.Unix(42, 0), time.Second)
			scheduler := platformclock.NewDeterministic(time.Unix(100, 0), time.Second)
			specialized := platformclock.NewDeterministic(time.Unix(200, 0), time.Second)
			if name == "same" {
				scheduler = wall
			}
			recorder := &subprocessTimerRecorder{TimerSource: scheduler}
			edges := serviceedges.Edges{Clock: wall, ProcessScheduler: recorder}
			selectedWall, selectedScheduler := wall, scheduler
			if name == "override" {
				recorder = &subprocessTimerRecorder{TimerSource: specialized}
				edges.PlatformProcessClock = recorder
				selectedWall, selectedScheduler = specialized, specialized
			}
			if name == "legacy explicit" {
				edges.Clock = metricsNowOnlyClock{at: wall.Now()}
			}
			clock := selectedSubprocessClock(t, edges)
			wantNow := selectedWall.Now()
			if clock.Now() != wantNow {
				t.Fatalf("Now = %v, want %v", clock.Now(), wantNow)
			}
			const grace = 10 * time.Second
			timer := clock.After(grace)
			if recorder.duration != grace {
				t.Fatalf("After duration = %v, want %v", recorder.duration, grace)
			}
			if name == "distinct" || name == "override" {
				wall.SetTick(100)
				assertSubprocessTimerPending(t, timer)
				if name == "distinct" {
					wantNow = wall.Now()
				} else {
					scheduler.SetTick(100)
					assertSubprocessTimerPending(t, timer)
				}
			}
			selectedScheduler.SetTick(9)
			assertSubprocessTimerPending(t, timer)
			selectedScheduler.SetTick(10)
			select {
			case at := <-timer:
				if at != selectedScheduler.Now() {
					t.Fatalf("delivery = %v, want %v", at, selectedScheduler.Now())
				}
			default:
				t.Fatal("timer did not deliver at grace")
			}
			if name == "same" || name == "override" {
				wantNow = selectedWall.Now()
			}
			if clock.Now() != wantNow {
				t.Fatalf("Now after scheduler advance = %v, want %v", clock.Now(), wantNow)
			}
		})
	}
}

func TestPlatformProcessClockLegacyRealScheduler(t *testing.T) {
	t.Parallel()
	wall := metricsNowOnlyClock{at: time.Unix(42, 0)}
	clock := selectedSubprocessClock(t, serviceedges.Edges{Clock: wall, ProcessScheduler: platformclock.Real{}})
	select {
	case at := <-clock.After(0):
		if at.IsZero() || clock.Now() != wall.Now() {
			t.Fatalf("delivery = %v, wall = %v", at, clock.Now())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("normalized real scheduler did not deliver")
	}
}

func TestSubprocessTimeIsolation(t *testing.T) {
	t.Parallel()
	wallA := platformclock.NewDeterministic(time.Unix(42, 0), time.Second)
	wallB := platformclock.NewDeterministic(time.Unix(100, 0), time.Second)
	schedulerA := platformclock.NewDeterministic(time.Unix(200, 0), time.Second)
	schedulerB := platformclock.NewDeterministic(time.Unix(300, 0), time.Second)
	a := selectedSubprocessClock(t, serviceedges.Edges{Clock: wallA, ProcessScheduler: schedulerA})
	b := selectedSubprocessClock(t, serviceedges.Edges{Clock: wallB, ProcessScheduler: schedulerB})
	timerA, timerB := a.After(10*time.Second), b.After(10*time.Second)
	wallA.SetTick(50)
	schedulerA.SetTick(10)
	if a.Now() != wallA.Now() || b.Now() != time.Unix(100, 0).UTC() {
		t.Fatal("wall observations leaked across pairs")
	}
	assertSubprocessTimerPending(t, timerB)
	select {
	case <-timerA:
	default:
		t.Fatal("first pair did not progress")
	}
	wallB.SetTick(60)
	schedulerB.SetTick(10)
	if a.Now() != wallA.Now() || b.Now() != wallB.Now() {
		t.Fatal("second pair changed first pair's observations")
	}
	select {
	case <-timerB:
	default:
		t.Fatal("second pair did not progress")
	}
}
