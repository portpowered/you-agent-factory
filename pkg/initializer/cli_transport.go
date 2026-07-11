package initializer

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory"
	initializerdashboard "github.com/portpowered/infinite-you/pkg/initializer/dashboard"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"go.uber.org/zap"
)

// CLITransport bundles initializer-produced domain services with the session
// runtime host used by CLI local in-process startup without constructing root
// FactoryService at the composition boundary.
type CLITransport struct {
	Services *Services
	Host     *SessionRuntimeHost
}

// InitializeCLITransport loads factory configuration, composes domain services,
// and returns the transport bundle used by local in-process CLI startup.
func InitializeCLITransport(ctx context.Context, cfg *Config) (*CLITransport, error) {
	if cfg == nil {
		return nil, fmt.Errorf("initialize CLI transport: config is required")
	}
	hostCfg := *cfg
	renderer := hostCfg.SimpleDashboardRenderer
	hostCfg.SimpleDashboardRenderer = nil
	core, err := BuildCore(ctx, &hostCfg)
	if err != nil {
		return nil, err
	}
	host := NewSessionRuntimeHostFromCore(core, cfg)
	if renderer != nil {
		sidecar, sidecarErr := initializerdashboard.NewDashboardSidecar(initializerdashboard.DashboardSidecarConfig{
			Reader: initializerdashboard.NewRuntimeDashboardReader(host.host),
			Renderer: initializerdashboard.DashboardRendererFunc(func(input initializerdashboard.DashboardRenderInput) {
				renderer(runtimehost.SimpleDashboardRenderInput{EngineState: input.EngineState, RenderData: input.RenderData, Now: input.Now})
			}),
			Timing: initializerdashboard.ClockTiming{Clock: factory.EnsureClock(cfg.Clock)},
			ReportError: func(err error) {
				logger := cfg.Logger
				if logger == nil {
					logger = zap.NewNop()
				}
				logger.Error("simple dashboard render failed", zap.Error(err))
			},
		})
		if sidecarErr != nil {
			return nil, sidecarErr
		}
		host.dashboard = sidecar
	}
	return &CLITransport{
		Services: servicesFromCoreWithModels(core, host.ModelService()),
		Host:     host,
	}, nil
}

// Runner returns the local in-process runtime seam used by pkg/cli/run.
func (t *CLITransport) Runner() LocalRuntimeRunner {
	if t == nil || t.Host == nil {
		return nil
	}
	return t.Host
}
