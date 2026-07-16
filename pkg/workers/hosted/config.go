package hostedworkers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/jonboulle/clockwork"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	hostedlinear "github.com/portpowered/infinite-you/pkg/workers/hosted/linear"
	"go.uber.org/zap"
)

// SecretResolver resolves hosted-worker auth.secretRef values at runtime.
type SecretResolver func(ctx context.Context, runtimeCfg interfaces.RuntimeConfigLookup, secretRef string) (string, error)

// Submitter submits normalized hosted-poller work requests into factory ingress.
type Submitter func(context.Context, work.WorkRequest) error

// Config carries hosted-poller runtime dependencies injected by the service root.
type Config struct {
	Logger         *zap.Logger
	Clock          clockwork.Clock
	HTTPClient     *http.Client
	SecretResolver SecretResolver
	LinearEndpoint string
}

// LinearPollerDependencies contains the complete construction input for one
// hosted Linear poller. Runtime and submission collaborators are required;
// side-effect edges use the same defaults as production when omitted.
type LinearPollerDependencies struct {
	Config        Config
	RuntimeConfig interfaces.RuntimeConfigLookup
	Workstation   interfaces.FactoryWorkstationConfig
	Worker        *workerconfig.Config
	Submitter     Submitter
}

// LinearPoller is a validated hosted Linear component ready for supervision.
type LinearPoller struct {
	config        Config
	runtimeConfig interfaces.RuntimeConfigLookup
	workstation   interfaces.FactoryWorkstationConfig
	worker        *workerconfig.Config
	submitter     Submitter
}

// NewLinearPoller validates required dependencies and applies production
// defaults before any poller goroutine is started.
func NewLinearPoller(deps LinearPollerDependencies) (*LinearPoller, error) {
	switch {
	case deps.RuntimeConfig == nil:
		return nil, fmt.Errorf("construct hosted linear poller: runtime config is required")
	case deps.Worker == nil:
		return nil, fmt.Errorf("construct hosted linear poller: worker is required")
	case deps.Worker.Auth == nil || strings.TrimSpace(deps.Worker.Auth.SecretRef) == "":
		return nil, fmt.Errorf("construct hosted linear poller %q: auth.secretRef is required", deps.Worker.Name)
	case deps.Worker.Linear == nil:
		return nil, fmt.Errorf("construct hosted linear poller %q: linear config is required", deps.Worker.Name)
	case deps.Submitter == nil:
		return nil, fmt.Errorf("construct hosted linear poller: submitter is required")
	}
	if _, err := hostedlinear.PollInterval(deps.Worker.Linear); err != nil {
		return nil, fmt.Errorf("construct hosted linear poller %q: %w", deps.Worker.Name, err)
	}

	config := deps.Config.withProductionDefaults()
	return &LinearPoller{
		config:        config,
		runtimeConfig: deps.RuntimeConfig,
		workstation:   deps.Workstation,
		worker:        deps.Worker,
		submitter:     deps.Submitter,
	}, nil
}

// NewConfig constructs production-equivalent hosted runtime edges without
// starting a poller. Process composition may replace any explicit edge.
func NewConfig(config Config) Config {
	return config.withProductionDefaults()
}

func (c Config) withProductionDefaults() Config {
	c.Logger = c.logger()
	c.Clock = c.supervisorClock()
	c.HTTPClient = c.httpClient()
	c.SecretResolver = c.secretResolver()
	c.LinearEndpoint = c.linearEndpoint()
	return c
}

func (c Config) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: hostedlinear.DefaultRequestTimeout}
}

func (c Config) secretResolver() SecretResolver {
	if c.SecretResolver != nil {
		return c.SecretResolver
	}
	return hostedlinear.ResolveSecretRef
}

func (c Config) linearEndpoint() string {
	if endpoint := strings.TrimSpace(c.LinearEndpoint); endpoint != "" {
		return endpoint
	}
	return hostedlinear.DefaultEndpoint
}

func (c Config) supervisorClock() clockwork.Clock {
	if c.Clock != nil {
		return c.Clock
	}
	return clockwork.NewRealClock()
}

func (c Config) logger() *zap.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return zap.NewNop()
}
