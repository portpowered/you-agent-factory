package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/modelhost"
)

// ApplyInvocationBootstrapLocalModelTestFixture wires hermetic ready LOCAL managed-model
// overrides for callers that build an in-process invocation bootstrap, such as
// models CLI offline invoke tests.
func ApplyInvocationBootstrapLocalModelTestFixture(
	cfg *FactoryServiceConfig,
	healthEndpoint string,
	runtime localmodels.Runtime,
	assets localmodels.AssetPuller,
) {
	if cfg == nil {
		return
	}
	cfg.LocalModelRuntimeOverride = runtime
	cfg.ModelAssets = assets
	cfg.ModelHostOverride = newInvocationBootstrapSupervisedModelHost(assets, healthEndpoint)
	cfg.SkipBuiltInRunnerPrerequisiteValidation = true
}

func newInvocationBootstrapSupervisedModelHost(assets localmodels.AssetPuller, healthEndpoint string) modelhost.Host {
	launcher := &invocationBootstrapFakeProcessLauncher{healthEndpoint: strings.TrimSpace(healthEndpoint)}
	return modelhost.NewCatalogHost(modelhost.NewLocalAssetGateway(assets), modelhost.Options{
		SourceResolver: modelhost.DefaultManagedRuntimeSourceResolverAdapter(),
		Supervisor: modelhost.SupervisorConfig{
			ReadinessTimeout:    500 * time.Millisecond,
			HealthCheckInterval: 10 * time.Millisecond,
			ProcessLauncher:     launcher,
			HealthChecker:       modelhost.HTTPHealthChecker{Path: "/health"},
		},
	})
}

type invocationBootstrapFakeProcessLauncher struct {
	mu             sync.Mutex
	healthEndpoint string
}

func (f *invocationBootstrapFakeProcessLauncher) Start(_ context.Context, _ modelhost.ProcessStartSpec) (modelhost.ManagedProcess, error) {
	return &invocationBootstrapFakeManagedProcess{
		endpoint: f.healthEndpoint,
		stopCh:   make(chan struct{}),
	}, nil
}

type invocationBootstrapFakeManagedProcess struct {
	endpoint string
	stopCh   chan struct{}
}

func (p *invocationBootstrapFakeManagedProcess) HealthEndpoint() string {
	return p.endpoint
}

func (p *invocationBootstrapFakeManagedProcess) Stop(context.Context) error {
	select {
	case <-p.stopCh:
	default:
		close(p.stopCh)
	}
	return nil
}

func (p *invocationBootstrapFakeManagedProcess) Wait() error {
	<-p.stopCh
	return nil
}

var _ modelhost.ProcessLauncher = (*invocationBootstrapFakeProcessLauncher)(nil)
var _ modelhost.ManagedProcess = (*invocationBootstrapFakeManagedProcess)(nil)
