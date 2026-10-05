package clockview

import (
	"sync"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
)

type generation struct {
	timer    platformclock.Timer
	deadline time.Time
	cancel   chan struct{}
	done     chan struct{}
}

type timer struct {
	mu        sync.Mutex
	scheduler platformclock.TimerSource
	current   *generation
	channel   chan time.Time
	callback  func()
	interval  time.Duration
}

func newTimer(scheduler platformclock.TimerSource, d, interval time.Duration, callback func()) *timer {
	t := &timer{scheduler: scheduler, channel: make(chan time.Time, 1), callback: callback, interval: interval}
	t.replace(d, interval)
	return t
}

func (t *timer) Chan() <-chan time.Time     { return t.channel }
func (t *timer) Reset(d time.Duration) bool { return t.replace(d, 0) }

// Stop retires and joins the waiting bridge, but cannot join an admitted callback.
// The source owner joins callbacks as part of its own shutdown protocol.
func (t *timer) Stop() bool {
	t.mu.Lock()
	old, active := t.retireLocked()
	t.mu.Unlock()
	join(old)
	return active
}

func (t *timer) retireLocked() (*generation, bool) {
	old := t.current
	if old == nil {
		return nil, false
	}
	t.current = nil
	old.timer.Stop()
	close(old.cancel)
	// The external timer expires at bridge delivery, not at the source signal.
	// Until then we can still prevent channel delivery or callback admission.
	return old, true
}

func join(g *generation) {
	if g != nil {
		<-g.done
	}
}

func (t *timer) replace(d, interval time.Duration) bool {
	t.mu.Lock()
	old, active := t.retireLocked()
	t.interval = interval
	g := &generation{timer: t.scheduler.NewTimer(d), deadline: t.scheduler.Now().Add(d), cancel: make(chan struct{}), done: make(chan struct{})}
	t.current = g
	go t.wait(g)
	t.mu.Unlock()
	join(old)
	return active
}

func (t *timer) wait(g *generation) {
	defer close(g.done)
	for {
		select {
		case <-g.cancel:
			return
		case instant := <-g.timer.C():
			if !t.deliver(g, instant) {
				return
			}
		}
	}
}

func (t *timer) deliver(g *generation, instant time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current != g {
		return false
	}
	if t.interval == 0 {
		t.current = nil
	}
	if t.callback != nil {
		go t.callback()
	} else {
		select {
		case t.channel <- instant:
		default:
		}
	}
	if t.interval != 0 {
		// Coalesce missed ticks; never create a catch-up loop after a large advance.
		now := t.scheduler.Now()
		next := g.deadline.Add(t.interval)
		if !next.After(now) {
			next = next.Add((now.Sub(next)/t.interval + 1) * t.interval)
		}
		g.deadline = next
		g.timer = t.scheduler.NewTimer(next.Sub(now))
	}
	return t.interval != 0
}
