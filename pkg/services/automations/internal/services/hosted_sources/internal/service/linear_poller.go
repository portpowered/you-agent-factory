package service

import (
	"context"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	hostedlinear "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/internal/linear"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// Submitter submits normalized hosted-poller work requests into factory ingress.
type Submitter func(context.Context, work.WorkRequest) error

// LinearPoller is a validated hosted Linear component ready for supervision.
type LinearPoller struct {
	logger         *zap.Logger
	clock          hostedsources.Clock
	httpClient     hostedlinear.HTTPDoer
	secretResolver hostedlinear.SecretResolver
	checkpoints    hostedlinear.CheckpointStore
	linearEndpoint string
	runtimeConfig  interfaces.RuntimeConfigLookup
	workstation    interfaces.FactoryWorkstationConfig
	worker         *interfaces.FactoryWorkerConfig
	submitter      Submitter
}

// NewLinearPoller validates required dependencies and applies production
// defaults before any poller goroutine is started.
func NewLinearPoller(
	logger *zap.Logger,
	clock hostedsources.Clock,
	httpClient hostedlinear.HTTPDoer,
	secretResolver hostedlinear.SecretResolver,
	checkpoints hostedlinear.CheckpointStore,
	linearEndpoint string,
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter Submitter,
) (*LinearPoller, error) {
	switch {
	case clock == nil:
		return nil, fmt.Errorf("construct hosted linear poller: clock is required")
	case httpClient == nil:
		return nil, fmt.Errorf("construct hosted linear poller: HTTP client is required")
	case secretResolver == nil:
		return nil, fmt.Errorf("construct hosted linear poller: secret resolver is required")
	case checkpoints == nil:
		return nil, fmt.Errorf("construct hosted linear poller: checkpoint store is required")
	case runtimeConfig == nil:
		return nil, fmt.Errorf("construct hosted linear poller: runtime config is required")
	case worker == nil:
		return nil, fmt.Errorf("construct hosted linear poller: worker is required")
	case worker.Auth == nil || strings.TrimSpace(worker.Auth.SecretRef) == "":
		return nil, fmt.Errorf("construct hosted linear poller %q: auth.secretRef is required", worker.Name)
	case worker.Linear == nil:
		return nil, fmt.Errorf("construct hosted linear poller %q: linear config is required", worker.Name)
	case submitter == nil:
		return nil, fmt.Errorf("construct hosted linear poller: submitter is required")
	}
	if _, err := hostedlinear.PollInterval(worker.Linear); err != nil {
		return nil, fmt.Errorf("construct hosted linear poller %q: %w", worker.Name, err)
	}

	return &LinearPoller{
		logger:         defaultLogger(logger),
		clock:          clock,
		httpClient:     httpClient,
		secretResolver: secretResolver,
		checkpoints:    checkpoints,
		linearEndpoint: defaultLinearEndpoint(linearEndpoint),
		runtimeConfig:  runtimeConfig,
		workstation:    workstation,
		worker:         worker,
		submitter:      submitter,
	}, nil
}

func defaultLinearEndpoint(value string) string {
	if endpoint := strings.TrimSpace(value); endpoint != "" {
		return endpoint
	}
	return hostedlinear.DefaultEndpoint
}

func defaultLogger(logger *zap.Logger) *zap.Logger {
	if logger != nil {
		return logger
	}
	return zap.NewNop()
}
