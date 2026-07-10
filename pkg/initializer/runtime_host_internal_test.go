package initializer

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	initializerdashboard "github.com/portpowered/infinite-you/pkg/initializer/dashboard"
)

func TestSessionRuntimeHostPropagatesDashboardStartupFailureAndCancelsRuntime(t *testing.T) {
	t.Parallel()

	sidecar, err := initializerdashboard.NewDashboardSidecar(initializerdashboard.DashboardSidecarConfig{
		Reader:   testDashboardReader{},
		Renderer: testDashboardRenderer{},
		Timing:   nilTickerTiming{},
	})
	if err != nil {
		t.Fatalf("NewDashboardSidecar: %v", err)
	}
	runtimeCanceled := make(chan struct{})
	host := &SessionRuntimeHost{dashboard: sidecar}
	err = host.runWithDashboard(context.Background(), func(ctx context.Context) error {
		<-ctx.Done()
		close(runtimeCanceled)
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "nil ticker") {
		t.Fatalf("runWithDashboard() error = %v, want dashboard startup failure", err)
	}
	<-runtimeCanceled
}

func TestSessionRuntimeHostCancelsAndJoinsDashboardBeforeReturning(t *testing.T) {
	t.Parallel()

	ticks := make(chan time.Time)
	timing := &lifecycleDashboardTiming{
		started: make(chan struct{}),
		ticker:  &lifecycleDashboardTicker{ticks: ticks, stopped: make(chan struct{})},
	}
	renderStarted := make(chan struct{})
	releaseRender := make(chan struct{})
	sidecar, err := initializerdashboard.NewDashboardSidecar(initializerdashboard.DashboardSidecarConfig{
		Reader: testDashboardReader{},
		Renderer: &blockingDashboardRenderer{
			started: renderStarted,
			release: releaseRender,
		},
		Timing: timing,
	})
	if err != nil {
		t.Fatalf("NewDashboardSidecar: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runtimeCanceled := make(chan struct{})
	runReturned := make(chan error, 1)
	host := &SessionRuntimeHost{dashboard: sidecar}
	go func() {
		runReturned <- host.runWithDashboard(ctx, func(ctx context.Context) error {
			<-ctx.Done()
			close(runtimeCanceled)
			return nil
		})
	}()

	<-timing.started
	ticks <- time.Now()
	<-renderStarted
	cancel()
	<-runtimeCanceled
	select {
	case err := <-runReturned:
		t.Fatalf("runWithDashboard returned before dashboard exit: %v", err)
	default:
	}
	close(releaseRender)
	if err := <-runReturned; err != nil {
		t.Fatalf("runWithDashboard: %v", err)
	}
	<-timing.ticker.stopped
	if got := timing.startCount(); got != 1 {
		t.Fatalf("dashboard start count = %d, want 1", got)
	}
}

func TestSessionRuntimeHostRendersFinalDashboardAfterJoiningPeriodicLoop(t *testing.T) {
	t.Parallel()

	ready := make(chan struct{})
	tickerStopped := make(chan struct{})
	finalRenderStarted := make(chan struct{})
	releaseFinalRender := make(chan struct{})
	timing := &lifecycleDashboardTiming{
		started: make(chan struct{}),
		ticker: &lifecycleDashboardTicker{
			ticks:   make(chan time.Time),
			stopped: tickerStopped,
		},
	}
	sidecar, err := initializerdashboard.NewDashboardSidecar(initializerdashboard.DashboardSidecarConfig{
		Reader: testDashboardReader{},
		Renderer: &blockingDashboardRenderer{
			started: finalRenderStarted,
			release: releaseFinalRender,
		},
		Timing: timing,
		Ready:  func() { close(ready) },
	})
	if err != nil {
		t.Fatalf("NewDashboardSidecar: %v", err)
	}

	runReturned := make(chan error, 1)
	host := &SessionRuntimeHost{dashboard: sidecar}
	go func() {
		runReturned <- host.runWithDashboard(context.Background(), func(context.Context) error {
			<-ready
			return nil
		})
	}()

	<-finalRenderStarted
	select {
	case <-tickerStopped:
	default:
		t.Fatal("final dashboard render started before periodic loop was joined")
	}
	select {
	case err := <-runReturned:
		t.Fatalf("runWithDashboard returned before final render completed: %v", err)
	default:
	}
	close(releaseFinalRender)
	if err := <-runReturned; err != nil {
		t.Fatalf("runWithDashboard: %v", err)
	}
}

type testDashboardReader struct{}

func (testDashboardReader) ReadDashboard(context.Context, time.Time) (initializerdashboard.DashboardRenderInput, error) {
	return initializerdashboard.DashboardRenderInput{}, nil
}

type testDashboardRenderer struct{}

func (testDashboardRenderer) RenderDashboard(initializerdashboard.DashboardRenderInput) {}

type nilTickerTiming struct{}

func (nilTickerTiming) Now() time.Time { return time.Time{} }

func (nilTickerTiming) NewTicker(time.Duration) initializerdashboard.DashboardTicker { return nil }

type blockingDashboardRenderer struct {
	started chan<- struct{}
	release <-chan struct{}
	once    sync.Once
}

func (r *blockingDashboardRenderer) RenderDashboard(initializerdashboard.DashboardRenderInput) {
	r.once.Do(func() {
		close(r.started)
		<-r.release
	})
}

type lifecycleDashboardTiming struct {
	mu      sync.Mutex
	starts  int
	started chan struct{}
	ticker  *lifecycleDashboardTicker
}

func (t *lifecycleDashboardTiming) Now() time.Time { return time.Now() }

func (t *lifecycleDashboardTiming) NewTicker(time.Duration) initializerdashboard.DashboardTicker {
	t.mu.Lock()
	t.starts++
	close(t.started)
	t.mu.Unlock()
	return t.ticker
}

func (t *lifecycleDashboardTiming) startCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.starts
}

type lifecycleDashboardTicker struct {
	ticks   <-chan time.Time
	stop    sync.Once
	stopped chan struct{}
}

func (t *lifecycleDashboardTicker) C() <-chan time.Time { return t.ticks }

func (t *lifecycleDashboardTicker) Stop() {
	t.stop.Do(func() { close(t.stopped) })
}
