package service

import (
	"context"
	"sync"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
)

// StartHostedLinearPoller supervises one hosted Linear poller workstation until ctx is canceled.
func (s *Service) StartHostedLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *workerconfig.Config,
	submitter WorkRequestSubmitter,
) error {
	if submitter == nil {
		return nil
	}
	return hostedworkers.StartLinearPoller(
		ctx,
		sidecars,
		s.hostedWorkersConfig(),
		runtimeCfg,
		workstation,
		workerDef,
		hostedworkers.Submitter(submitter),
	)
}

func (s *Service) validateHostedLinearPoller(
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *workerconfig.Config,
	submitter WorkRequestSubmitter,
) error {
	_, err := hostedworkers.NewLinearPoller(hostedworkers.LinearPollerDependencies{
		Config: s.hostedWorkersConfig(), RuntimeConfig: runtimeCfg,
		Workstation: workstation, Worker: workerDef,
		Submitter: hostedworkers.Submitter(submitter),
	})
	return err
}

func (s *Service) hostedWorkersConfig() hostedworkers.Config {
	if s == nil {
		return hostedworkers.Config{}
	}
	return s.cfg.HostedWorkers
}
