package dashboard

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewDashboardSidecarRejectsMissingRequiredInputs(t *testing.T) {
	t.Parallel()

	_, err := NewDashboardSidecar(DashboardSidecarConfig{})
	if err == nil || !strings.Contains(err.Error(), "dashboard reader is required") {
		t.Fatalf("NewDashboardSidecar() error = %v, want missing reader", err)
	}

	_, err = NewDashboardSidecar(DashboardSidecarConfig{Reader: dashboardReaderFunc(func(context.Context, time.Time) (DashboardRenderInput, error) {
		return DashboardRenderInput{}, nil
	})})
	if err == nil || !strings.Contains(err.Error(), "dashboard renderer is required") {
		t.Fatalf("NewDashboardSidecar() error = %v, want missing renderer", err)
	}
}

func TestDashboardSidecarReportsReadinessRendersAndObservesCancellation(t *testing.T) {
	t.Parallel()

	tick := make(chan time.Time)
	ticker := &fakeDashboardTicker{ticks: tick, stopped: make(chan struct{})}
	now := time.Date(2026, time.July, 10, 20, 0, 0, 0, time.UTC)
	read := make(chan time.Time, 1)
	rendered := make(chan DashboardRenderInput, 1)
	ready := make(chan struct{})
	timing := &fakeDashboardTiming{now: now, ticker: ticker}
	sidecar, err := NewDashboardSidecar(DashboardSidecarConfig{
		Reader: dashboardReaderFunc(func(_ context.Context, got time.Time) (DashboardRenderInput, error) {
			read <- got
			return DashboardRenderInput{Now: got}, nil
		}),
		Renderer: dashboardRendererFunc(func(input DashboardRenderInput) { rendered <- input }),
		Timing:   timing,
		Ready:    func() { close(ready) },
	})
	if err != nil {
		t.Fatalf("NewDashboardSidecar() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	exited := make(chan error, 1)
	go func() { exited <- sidecar.Run(ctx) }()

	<-ready
	if timing.interval != defaultDashboardRenderInterval {
		t.Fatalf("ticker interval = %s, want %s", timing.interval, defaultDashboardRenderInterval)
	}
	tick <- now
	if got := <-read; !got.Equal(now) {
		t.Fatalf("reader time = %s, want %s", got, now)
	}
	if got := <-rendered; !got.Now.Equal(now) {
		t.Fatalf("render input time = %s, want %s", got.Now, now)
	}

	cancel()
	if err := <-exited; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	<-ticker.stopped
}

func TestDashboardSidecarReportsReadFailureAndContinues(t *testing.T) {
	t.Parallel()

	tick := make(chan time.Time)
	reported := make(chan error, 1)
	rendered := make(chan struct{}, 1)
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sidecar, err := NewDashboardSidecar(DashboardSidecarConfig{
		Reader: dashboardReaderFunc(func(context.Context, time.Time) (DashboardRenderInput, error) {
			return DashboardRenderInput{}, errors.New("snapshot unavailable")
		}),
		Renderer: dashboardRendererFunc(func(DashboardRenderInput) { rendered <- struct{}{} }),
		Timing: &fakeDashboardTiming{
			now:    time.Now(),
			ticker: &fakeDashboardTicker{ticks: tick, stopped: make(chan struct{})},
		},
		Ready:       func() { close(ready) },
		ReportError: func(err error) { reported <- err },
	})
	if err != nil {
		t.Fatalf("NewDashboardSidecar() error = %v", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- sidecar.Run(ctx) }()
	<-ready
	tick <- time.Now()
	if got := <-reported; !strings.Contains(got.Error(), "snapshot unavailable") {
		t.Fatalf("reported error = %v", got)
	}
	select {
	case <-rendered:
		t.Fatal("renderer called after read failure")
	default:
	}
	cancel()
	if err := <-exited; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

type dashboardReaderFunc func(context.Context, time.Time) (DashboardRenderInput, error)

func (f dashboardReaderFunc) ReadDashboard(ctx context.Context, now time.Time) (DashboardRenderInput, error) {
	return f(ctx, now)
}

type dashboardRendererFunc func(DashboardRenderInput)

func (f dashboardRendererFunc) RenderDashboard(input DashboardRenderInput) { f(input) }

type fakeDashboardTiming struct {
	now      time.Time
	ticker   DashboardTicker
	mu       sync.Mutex
	interval time.Duration
}

func (f *fakeDashboardTiming) Now() time.Time { return f.now }

func (f *fakeDashboardTiming) NewTicker(interval time.Duration) DashboardTicker {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.interval = interval
	return f.ticker
}

type fakeDashboardTicker struct {
	ticks   <-chan time.Time
	stop    sync.Once
	stopped chan struct{}
}

func (f *fakeDashboardTicker) C() <-chan time.Time { return f.ticks }
func (f *fakeDashboardTicker) Stop() {
	f.stop.Do(func() { close(f.stopped) })
}
