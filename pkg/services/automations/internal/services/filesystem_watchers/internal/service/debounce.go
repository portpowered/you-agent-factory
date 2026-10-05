package service

import (
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
)

const defaultDebounceWindow = 100 * time.Millisecond

type debounceClock interface {
	AfterFunc(d time.Duration, f func()) clockwork.Timer
}

type debounceScheduler struct {
	clock  debounceClock
	window time.Duration
	mu     sync.Mutex
	timers map[string]*debounceCall
	closed bool
	joined sync.WaitGroup
}

type debounceCall struct{ timer clockwork.Timer }

func newDebounceScheduler(clock debounceClock, window time.Duration) *debounceScheduler {
	if window <= 0 {
		window = defaultDebounceWindow
	}
	return &debounceScheduler{
		clock:  clock,
		window: window,
		timers: make(map[string]*debounceCall),
	}
}

func (s *debounceScheduler) schedule(key string, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if existing, ok := s.timers[key]; ok {
		if existing.timer.Stop() {
			s.joined.Done()
		}
	}
	call := &debounceCall{}
	s.joined.Add(1)
	call.timer = s.clock.AfterFunc(s.window, func() {
		defer s.joined.Done()
		s.mu.Lock()
		if s.closed || s.timers[key] != call {
			s.mu.Unlock()
			return
		}
		delete(s.timers, key)
		s.mu.Unlock()
		fn()
	})
	s.timers[key] = call
}

func (s *debounceScheduler) cancelAll() {
	s.mu.Lock()
	s.closed = true
	for key, call := range s.timers {
		if call.timer.Stop() {
			s.joined.Done()
		}
		delete(s.timers, key)
	}
	s.mu.Unlock()
	// Callbacks can use scheduler state while finishing; never join under mu.
	s.joined.Wait()
}
