package hostedworkers

import (
	"context"
	"net/http"

	"github.com/jonboulle/clockwork"
	hostedlinear "github.com/portpowered/infinite-you/pkg/hostedworkers/linear"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

// SecretResolver resolves hosted-worker auth.secretRef values at runtime.
type SecretResolver func(ctx context.Context, runtimeCfg interfaces.RuntimeConfigLookup, secretRef string) (string, error)

// Submitter submits normalized hosted-poller work requests into factory ingress.
type Submitter func(context.Context, interfaces.WorkRequest) error

// Config carries hosted-poller runtime dependencies injected by the service root.
type Config struct {
	Logger           *zap.Logger
	Clock            clockwork.Clock
	HTTPClient       *http.Client
	SecretResolver   SecretResolver
	LinearEndpoint   string
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
	if c.LinearEndpoint != "" {
		return c.LinearEndpoint
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
