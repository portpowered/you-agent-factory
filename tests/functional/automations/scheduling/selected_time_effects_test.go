package automations

import (
	"io/fs"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// These effects expose registration/read/retirement acknowledgements at Edges.
// Time is advanced only by the scenario; wall deadlines are failure ceilings.
type selectedTimeFacts struct{ platformclock.Source }

type selectedTimeScheduler struct {
	*platformclock.Deterministic
	registered chan selectedTimeWait
	readiness  chan struct{}
	tick       int
}

type selectedTimeWait struct {
	duration time.Duration
	deadline time.Time
	timer    *selectedTimeTimer
}

type selectedTimeTimer struct {
	platformclock.Timer
	stopped atomic.Bool
}

func (t *selectedTimeTimer) Stop() bool {
	t.stopped.Store(true)
	return t.Timer.Stop()
}

func (s *selectedTimeScheduler) NewTimer(d time.Duration) platformclock.Timer {
	timer := &selectedTimeTimer{Timer: s.Deterministic.NewTimer(d)}
	s.registered <- selectedTimeWait{duration: d, deadline: s.Now().Add(d), timer: timer}
	if d == 10*time.Millisecond {
		s.readiness <- struct{}{}
	}
	return timer
}

// Public runtime activation itself polls readiness on the selected scheduler.
// Advance those acknowledged 10ms waits only while an activation is in flight;
// source eligibility and all close/join steps otherwise keep time frozen.
func selectedTimeActivate[T any](t *testing.T, scheduler *selectedTimeScheduler, fn func() T) T {
	t.Helper()
	done := make(chan struct{})
	var result T
	go func() { defer close(done); result = fn() }()
	ceiling := time.After(30 * time.Second)
	for {
		select {
		case <-done:
			return result
		case <-scheduler.readiness:
			scheduler.advance(10 * time.Millisecond)
		case <-ceiling:
			t.Fatal("public activation did not complete with controlled readiness polls")
			var zero T
			return zero
		}
	}
}

func (s *selectedTimeScheduler) After(d time.Duration) <-chan time.Time { return s.NewTimer(d).C() }

func (s *selectedTimeScheduler) advance(d time.Duration) {
	s.tick += int(d / time.Millisecond)
	s.SetTick(s.tick)
}

func selectedTimeReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(30 * time.Second):
		t.Fatal("selected-time edge did not acknowledge")
		var zero T
		return zero
	}
}

func selectedTimeAbsent[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case value := <-ch:
		t.Fatalf("unexpected selected-time effect: %+v", value)
	default:
	}
}

func (s *selectedTimeScheduler) await(t *testing.T, duration time.Duration) *selectedTimeTimer {
	t.Helper()
	for {
		wait := selectedTimeReceive(t, s.registered)
		if wait.duration == duration && !wait.timer.stopped.Load() && wait.deadline.After(s.Now()) {
			return wait.timer
		}
	}
}

type selectedTimeFiles struct {
	platformfilesystem.Local
	mu         sync.Mutex
	empty      map[string]int
	reads      chan string
	walked     chan string
	admissions map[string]int
}

func (f *selectedTimeFiles) walk(path string, fn fs.WalkDirFunc) error {
	err := filepath.WalkDir(path, fn)
	f.walked <- filepath.Clean(path)
	return err
}

func (f *selectedTimeFiles) awaitWatch(t *testing.T, dir string) {
	t.Helper()
	want := filepath.Join(dir, "inputs")
	for {
		if selectedTimeReceive(t, f.walked) == want {
			return
		}
	}
}

func (f *selectedTimeFiles) admitted(record work.FactorySubmissionRecord) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.admissions[record.Request.RequestID]++
}

func (f *selectedTimeFiles) count(requestID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.admissions[requestID]
}

func (f *selectedTimeFiles) ReadFile(path string) ([]byte, error) {
	f.mu.Lock()
	remaining := f.empty[filepath.Clean(path)]
	if remaining > 0 {
		f.empty[filepath.Clean(path)] = remaining - 1
	}
	f.mu.Unlock()
	if remaining > 0 {
		f.reads <- filepath.Clean(path)
		return nil, nil
	}
	return f.Local.ReadFile(path)
}

func (f *selectedTimeFiles) hold(path string, reads int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.empty[filepath.Clean(path)] = reads
}
