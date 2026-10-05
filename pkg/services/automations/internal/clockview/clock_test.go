package clockview

import (
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
)

type observedSource struct {
	*platformclock.Deterministic
	registered chan time.Duration
	stopped    atomic.Int32
}

type observedTimer struct {
	platformclock.Timer
	owner *observedSource
}

func (t observedTimer) Stop() bool {
	if t.Timer.Stop() {
		t.owner.stopped.Add(1)
		return true
	}
	return false
}

func (s *observedSource) NewTimer(d time.Duration) platformclock.Timer {
	t := s.Deterministic.NewTimer(d)
	s.registered <- d
	return observedTimer{Timer: t, owner: s}
}

func newObservedSource() *observedSource {
	return &observedSource{Deterministic: platformclock.NewDeterministic(time.Unix(42, 0), time.Second), registered: make(chan time.Duration, 32)}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(30 * time.Second):
		t.Fatal("owned effect did not acknowledge")
		var zero T
		return zero
	}
}

func absent[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case value := <-ch:
		t.Fatalf("unexpected delivery: %v", value)
	default:
	}
}

func TestClockViewFactsAndDirectWaits(t *testing.T) {
	t.Parallel()
	facts := platformclock.NewDeterministic(time.Unix(100, 0), time.Second)
	s := newObservedSource()
	v := New(facts, s)
	absent(t, s.registered)
	if v.Now() != facts.Now() || v.Since(facts.Now().Add(-time.Hour)) != time.Hour || v.Until(facts.Now().Add(time.Hour)) != time.Hour {
		t.Fatal("fact origin changed")
	}
	after := v.After(time.Second)
	facts.SetTick(10)
	absent(t, after)
	s.SetTick(1)
	if receive(t, after) != s.Now() {
		t.Fatal("After used wrong timestamp")
	}

}

type sleepSource struct{ *observedSource }

func (s sleepSource) After(d time.Duration) <-chan time.Time { return s.NewTimer(d).C() }

func TestClockViewSleepAndImmediateDurations(t *testing.T) {
	t.Parallel()
	s := sleepSource{newObservedSource()}
	v := New(s, s)
	done := make(chan struct{})
	go func() { v.Sleep(time.Second); close(done) }()
	receive(t, s.registered)
	absent(t, done)
	s.SetTick(1)
	receive(t, done)
	for _, d := range []time.Duration{0, -time.Second} {
		timer := v.NewTimer(d)
		receive(t, s.registered)
		if receive(t, timer.Chan()) != s.Now() || timer.Stop() {
			t.Fatal("immediate timer semantics changed")
		}
	}
}

func TestClockViewTimerStopResetAndStableChannel(t *testing.T) {
	t.Parallel()
	s := newObservedSource()
	v := New(s, s)
	timer := v.NewTimer(2 * time.Second)
	receive(t, s.registered)
	channel := timer.Chan()
	if !timer.Reset(3 * time.Second) {
		t.Fatal("active Reset returned false")
	}
	receive(t, s.registered)
	s.SetTick(2)
	absent(t, channel)
	s.SetTick(3)
	if receive(t, channel) != s.Now() {
		t.Fatal("reset timestamp changed")
	}
	if timer.Reset(time.Second) {
		t.Fatal("expired Reset returned true")
	}
	receive(t, s.registered)
	if timer.Chan() != channel {
		t.Fatal("Reset replaced the published channel")
	}
	if !timer.Stop() || timer.Stop() {
		t.Fatal("Stop active/repeat semantics changed")
	}
	s.SetTick(4)
	absent(t, channel)
	if timer.Reset(time.Second) {
		t.Fatal("stopped Reset returned true")
	}
	receive(t, s.registered)
	s.SetTick(5)
	receive(t, channel)
	if s.stopped.Load() != 2 {
		t.Fatalf("retired source timers = %d, want 2", s.stopped.Load())
	}
}

func TestClockViewCallbackStopResetAndRunningSemantics(t *testing.T) {
	t.Parallel()
	s := newObservedSource()
	v := New(s, s)
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	timer := v.AfterFunc(time.Second, func() { close(started); <-release; close(finished) })
	receive(t, s.registered)
	if !timer.Stop() || timer.Stop() {
		t.Fatal("pending callback did not stop")
	}
	s.SetTick(1)
	absent(t, started)
	if timer.Reset(2 * time.Second) {
		t.Fatal("stopped callback Reset returned true")
	}
	receive(t, s.registered)
	if !timer.Reset(time.Second) {
		t.Fatal("active callback Reset returned false")
	}
	receive(t, s.registered)
	s.SetTick(2)
	receive(t, started)
	if timer.Stop() {
		t.Fatal("Stop promised to stop a running callback")
	}
	absent(t, timer.Chan())
	close(release)
	receive(t, finished)
}

func TestClockViewTickerCoalescesResetsAndStops(t *testing.T) {
	t.Parallel()
	s := newObservedSource()
	v := New(s, s)
	ticker := v.NewTicker(time.Second)
	receive(t, s.registered)
	s.SetTick(1)
	receive(t, s.registered)
	s.SetTick(10)
	receive(t, s.registered)
	// A slow reader receives one buffered observation, with no catch-up queue.
	receive(t, ticker.Chan())
	absent(t, ticker.Chan())
	ticker.Reset(2 * time.Second)
	receive(t, s.registered)
	s.SetTick(11)
	absent(t, ticker.Chan())
	s.SetTick(12)
	receive(t, s.registered)
	receive(t, ticker.Chan())
	ticker.Stop()
	s.SetTick(20)
	absent(t, ticker.Chan())
	absent(t, s.registered)
	for _, d := range []time.Duration{0, -time.Second} {
		t.Run(d.String(), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid ticker interval did not panic")
				}
			}()
			v.NewTicker(d)
		})
	}
}

func TestClockViewCallbackResetWhileRunning(t *testing.T) {
	t.Parallel()
	s := newObservedSource()
	v := New(s, s)
	started, finished := make(chan struct{}, 2), make(chan struct{}, 2)
	release := make(chan struct{})
	timer := v.AfterFunc(time.Second, func() { started <- struct{}{}; <-release; finished <- struct{}{} })
	receive(t, s.registered)
	s.SetTick(1)
	receive(t, started)
	if timer.Reset(time.Second) {
		t.Fatal("running callback Reset returned active")
	}
	receive(t, s.registered)
	s.SetTick(2)
	receive(t, started)
	close(release)
	receive(t, finished)
	receive(t, finished)
}

func TestClockViewTickerKeepsCadenceAcrossLargeAdvance(t *testing.T) {
	t.Parallel()
	s := newObservedSource()
	ticker := New(s, s).NewTicker(3 * time.Second)
	defer ticker.Stop()
	receive(t, s.registered)
	s.SetTick(10)
	if d := receive(t, s.registered); d != 2*time.Second {
		t.Fatalf("next wait=%s, want 2s to original cadence", d)
	}
	receive(t, ticker.Chan())
	s.SetTick(11)
	absent(t, ticker.Chan())
	s.SetTick(12)
	receive(t, s.registered)
	receive(t, ticker.Chan())
	for _, d := range []time.Duration{0, -time.Second} {
		t.Run(d.String(), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid ticker Reset did not panic")
				}
			}()
			ticker.Reset(d)
		})
	}
}
