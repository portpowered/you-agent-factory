package watch_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var watchProfileRoot string
var workWatchProcess support.ApplicationProcess
var selectedWatchProcess support.ApplicationProcess
var observationWatchProcess support.ApplicationProcess
var observationCommands observationCommandRouter
var observationRecordingFailures sync.Map
var observationRecordings recordings.Service
var legacyWatchSource watchObservationSource
var selectedWatchSource watchObservationSource
var selectedWatchScheduler = &watchSelectedScheduler{
	clock:   platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond),
	created: make(chan time.Duration, 128),
	startup: make(chan *watchReadinessTimer, 128),
}
var watchHostListeners sync.Map

// The source intentionally has no timer capability. The two immutable cohorts
// exercise omitted scheduler normalization and an explicit scheduler separately.
type watchObservationSource struct{ millis atomic.Int64 }

func (s *watchObservationSource) Now() time.Time {
	return time.Date(2026, time.October, 4, 0, 0, 0, 0, time.UTC).Add(time.Duration(s.millis.Load()) * time.Millisecond)
}

type watchSelectedScheduler struct {
	clock   *platformclock.Deterministic
	created chan time.Duration
	startup chan *watchReadinessTimer
	mu      sync.Mutex
	tick    int
}

func (s *watchSelectedScheduler) Now() time.Time { return s.clock.Now() }
func (s *watchSelectedScheduler) After(delay time.Duration) <-chan time.Time {
	return s.NewTimer(delay).C()
}
func (s *watchSelectedScheduler) NewTimer(delay time.Duration) platformclock.Timer {
	// Runtime readiness polls receive an explicit wakeup without moving the
	// watch clock or consuming its unrelated startup/request deadlines.
	if delay == 10*time.Millisecond {
		timer := &watchReadinessTimer{channel: make(chan time.Time, 1)}
		s.startup <- timer
		return timer
	}
	timer := s.clock.NewTimer(delay)
	if delay == 100*time.Millisecond {
		s.created <- delay
	}
	return timer
}

type watchReadinessTimer struct {
	channel chan time.Time
	stopped atomic.Bool
}

func (timer *watchReadinessTimer) C() <-chan time.Time { return timer.channel }
func (timer *watchReadinessTimer) Stop() bool          { return !timer.stopped.Swap(true) }
func (timer *watchReadinessTimer) wake(now time.Time) {
	if !timer.stopped.Swap(true) {
		timer.channel <- now
	}
}

func (s *watchSelectedScheduler) advance(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tick += delta
	s.clock.SetTick(s.tick)
}

type watchHostListener struct {
	listener net.Listener
	ready    chan struct{}
}

func watchAPIStarter(ctx context.Context, request platformhttpserver.StartRequest) error {
	value, ok := watchHostListeners.Load(request.Port)
	if !ok {
		return fmt.Errorf("no owned listener for port %d", request.Port)
	}
	host := value.(*watchHostListener)
	if request.OnBound != nil {
		request.OnBound(platformhttpserver.Binding{Host: "127.0.0.1", Port: request.Port})
	}
	close(host.ready)
	return platformhttpserver.Serve(ctx, request.Handler, host.listener, request.Logger)
}

var watchFixtureOnce sync.Once
var watchFixtureErr error

func ensureWatchFixture(t *testing.T) {
	t.Helper()
	watchFixtureOnce.Do(func() { watchFixtureErr = initializeWatchProcesses() })
	if watchFixtureErr != nil {
		t.Fatalf("initialize Work watch fixture: %v", watchFixtureErr)
	}
}

func initializeWatchProcesses() error {
	var err error
	watchProfileRoot, err = os.MkdirTemp("", "work-watch-profiles-")
	if err != nil {
		return err
	}
	workWatchProcess, err = support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		Clock: &legacyWatchSource, APIServerStarter: watchAPIStarter,
	})
	if err != nil {
		return fmt.Errorf("build work watch process: %w", err)
	}
	selectedWatchProcess, err = support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		Clock: &selectedWatchSource, ProcessScheduler: selectedWatchScheduler, APIServerStarter: watchAPIStarter,
	})
	if err != nil {
		return err
	}
	// This cohort exercises actual provider execution through the command edge;
	// the existing timing cohorts deliberately contain no workers.
	observationWatchProcess, err = support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: &observationCommands, APIServerStarter: watchAPIStarter,
		RecordingsRootObserver: func(service recordings.Service) { observationRecordings = service },
		RecordingWriteFile: func(target string, content []byte) error {
			if cause, ok := observationRecordingFailures.Load(filepath.Clean(target)); ok {
				return cause.(error)
			}
			return os.WriteFile(target, content, 0o600)
		},
	})
	return err
}

func closeWatchProcesses() error {
	//nolint:testsleep // Failure ceiling for real process teardown after every customer command has joined.
	closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var errs []error
	for _, process := range []support.ApplicationProcess{workWatchProcess, selectedWatchProcess, observationWatchProcess} {
		if process != nil {
			errs = append(errs, process.Close(closeContext))
		}
	}
	if watchProfileRoot != "" {
		errs = append(errs, os.RemoveAll(watchProfileRoot))
	}
	return errors.Join(errs...)
}

func TestMain(m *testing.M) {
	exitCode := m.Run()
	if err := closeWatchProcesses(); err != nil {
		fmt.Fprintf(os.Stderr, "close Work watch fixture: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

// FunctionalMonolithCleanup follows all customer children, matching TestMain.
func FunctionalMonolithCleanup(t *testing.T) {
	t.Helper()
	if err := closeWatchProcesses(); err != nil {
		t.Errorf("close Work watch fixture: %v", err)
	}
}
