// Package dashboard owns the bounded process-level simple dashboard sidecar.
package dashboard

import (
	"context"
	"errors"
	"fmt"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/transports/cli/dashboardrender"
)

const defaultDashboardRenderInterval = 30 * time.Second

// DashboardRenderInput is the bounded snapshot passed to the process dashboard
// renderer. It contains presentation data only and does not expose runtime
// mutation or process-host operations.
type DashboardRenderInput struct {
	EngineState interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	RenderData  dashboardrender.SimpleDashboardRenderData
	Now         time.Time
}

// DashboardReader supplies one consistent dashboard view from the active
// runtime. Composition adapters should implement only this read operation.
type DashboardReader interface {
	ReadDashboard(context.Context, time.Time) (DashboardRenderInput, error)
}

// DashboardRenderer emits one dashboard view to the configured output.
type DashboardRenderer interface {
	RenderDashboard(DashboardRenderInput)
}

// DashboardTicker is the cancellable timing primitive used by the sidecar.
type DashboardTicker interface {
	C() <-chan time.Time
	Stop()
}

// DashboardTiming supplies wall-clock reads and render ticks. Keeping timing
// behind this narrow seam makes lifecycle tests deterministic.
type DashboardTiming interface {
	Now() time.Time
	NewTicker(time.Duration) DashboardTicker
}

// DashboardSidecarConfig contains only process-dashboard dependencies.
type DashboardSidecarConfig struct {
	Reader         DashboardReader
	Renderer       DashboardRenderer
	Timing         DashboardTiming
	RenderInterval time.Duration
	Ready          func()
	ReportError    func(error)
}

// DashboardSidecar owns the periodic rendering loop independently of the
// runtime host. Construct it at the process-composition boundary.
type DashboardSidecar struct {
	reader         DashboardReader
	renderer       DashboardRenderer
	timing         DashboardTiming
	renderInterval time.Duration
	ready          func()
	reportError    func(error)
}

// NewDashboardSidecar validates and constructs a bounded dashboard lifecycle
// collaborator. Required inputs are rejected here instead of being recovered
// lazily from a runtime host or service facade.
func NewDashboardSidecar(cfg DashboardSidecarConfig) (*DashboardSidecar, error) {
	if cfg.Reader == nil {
		return nil, errors.New("initialize dashboard sidecar: dashboard reader is required")
	}
	if cfg.Renderer == nil {
		return nil, errors.New("initialize dashboard sidecar: dashboard renderer is required")
	}
	if cfg.Timing == nil {
		cfg.Timing = realDashboardTiming{}
	}
	if cfg.RenderInterval < 0 {
		return nil, fmt.Errorf("initialize dashboard sidecar: render interval must not be negative: %s", cfg.RenderInterval)
	}
	if cfg.RenderInterval == 0 {
		cfg.RenderInterval = defaultDashboardRenderInterval
	}
	return &DashboardSidecar{
		reader:         cfg.Reader,
		renderer:       cfg.Renderer,
		timing:         cfg.Timing,
		renderInterval: cfg.RenderInterval,
		ready:          cfg.Ready,
		reportError:    cfg.ReportError,
	}, nil
}

// Run renders on each configured tick until the owning process context is
// cancelled. Cancellation is normal shutdown.
func (s *DashboardSidecar) Run(ctx context.Context) error {
	if s == nil {
		return errors.New("run dashboard sidecar: sidecar is nil")
	}
	ticker := s.timing.NewTicker(s.renderInterval)
	if ticker == nil {
		return errors.New("run dashboard sidecar: timing source returned a nil ticker")
	}
	defer ticker.Stop()
	if s.ready != nil {
		s.ready()
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C():
			s.render(ctx)
		}
	}
}

// RenderFinal emits the shutdown snapshot after the periodic loop has exited.
// The process owner calls this synchronously so shutdown cannot return before
// the final operator-visible dashboard output is complete.
func (s *DashboardSidecar) RenderFinal(ctx context.Context) {
	if s == nil {
		return
	}
	s.render(ctx)
}

func (s *DashboardSidecar) render(ctx context.Context) {
	input, err := s.reader.ReadDashboard(ctx, s.timing.Now())
	if err != nil {
		if s.reportError != nil {
			s.reportError(fmt.Errorf("simple dashboard render failed: %w", err))
		}
		return
	}
	s.renderer.RenderDashboard(input)
}

type realDashboardTiming struct{}

func (realDashboardTiming) Now() time.Time { return time.Now() }

func (realDashboardTiming) NewTicker(interval time.Duration) DashboardTicker {
	return realDashboardTicker{Ticker: time.NewTicker(interval)}
}

type realDashboardTicker struct{ *time.Ticker }

func (t realDashboardTicker) C() <-chan time.Time { return t.Ticker.C }
