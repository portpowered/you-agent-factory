package watch_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var watchProfileRoot string
var workWatchProcess support.ApplicationProcess
var selectedWatchProcess support.ApplicationProcess
var legacyWatchSource watchObservationSource
var selectedWatchSource watchObservationSource
var selectedWatchScheduler = &watchSelectedScheduler{
	clock:   platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond),
	created: make(chan time.Duration, 128),
	startup: make(chan struct{}, 128),
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
	startup chan struct{}
	mu      sync.Mutex
	tick    int
}

func (s *watchSelectedScheduler) Now() time.Time { return s.clock.Now() }
func (s *watchSelectedScheduler) After(delay time.Duration) <-chan time.Time {
	return s.NewTimer(delay).C()
}
func (s *watchSelectedScheduler) NewTimer(delay time.Duration) platformclock.Timer {
	timer := s.clock.NewTimer(delay)
	if delay == 100*time.Millisecond {
		s.created <- delay
	}
	if delay == 10*time.Millisecond {
		s.startup <- struct{}{}
	}
	return timer
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

func TestMain(m *testing.M) {
	var err error
	watchProfileRoot, err = os.MkdirTemp("", "work-watch-profiles-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		Clock: &legacyWatchSource, APIServerStarter: watchAPIStarter,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build work watch process: %v\n", err)
		os.Exit(1)
	}
	workWatchProcess = process
	selectedWatchProcess, err = support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		Clock: &selectedWatchSource, ProcessScheduler: selectedWatchScheduler, APIServerStarter: watchAPIStarter,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build selected watch process: %v\n", err)
		os.Exit(1)
	}
	exitCode := m.Run()
	closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := process.Close(closeContext); err != nil {
		fmt.Fprintf(os.Stderr, "close work watch process: %v\n", err)
		exitCode = 1
	}
	if err := selectedWatchProcess.Close(closeContext); err != nil {
		fmt.Fprintf(os.Stderr, "close selected watch process: %v\n", err)
		exitCode = 1
	}
	if err := os.RemoveAll(watchProfileRoot); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitCode = 1
	}
	os.Exit(exitCode)
}
