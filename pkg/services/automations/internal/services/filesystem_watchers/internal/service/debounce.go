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
	timers map[string]clockwork.Timer
}

func newDebounceScheduler(clock debounceClock, window time.Duration) *debounceScheduler {
	if clock == nil {
		clock = clockwork.NewRealClock()
	}
	if window <= 0 {
		window = defaultDebounceWindow
	}
	return &debounceScheduler{
		clock:  clock,
		window: window,
		timers: make(map[string]clockwork.Timer),
	}
}

func (s *debounceScheduler) schedule(key string, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.timers[key]; ok {
		existing.Stop()
	}
	timer := s.clock.AfterFunc(s.window, func() {
		s.mu.Lock()
		delete(s.timers, key)
		s.mu.Unlock()
		fn()
	})
	s.timers[key] = timer
}

func (s *debounceScheduler) cancelAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, timer := range s.timers {
		timer.Stop()
		delete(s.timers, key)
	}
}
