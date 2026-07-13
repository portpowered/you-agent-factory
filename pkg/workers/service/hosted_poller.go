package service

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
)

// StartHostedLinearPoller supervises one hosted Linear poller workstation until ctx is canceled.
func (s *Service) StartHostedLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	submitter WorkRequestSubmitter,
) {
	if sidecars == nil || submitter == nil {
		return
	}
	hostedworkers.StartLinearPoller(
		ctx,
		sidecars,
		s.hostedWorkersConfig(),
		runtimeCfg,
		workstation,
		workerDef,
		hostedworkers.Submitter(submitter),
	)
}

func (s *Service) hostedWorkersConfig() hostedworkers.Config {
	if s == nil {
		return hostedworkers.Config{}
	}
	return hostedworkers.Config{
		Logger:         s.cfg.Logger,
		Clock:          s.cfg.Clock,
		HTTPClient:     s.cfg.HostedHTTPClient,
		SecretResolver: s.cfg.HostedSecretResolver,
		LinearEndpoint: s.cfg.HostedLinearEndpoint,
	}
}
