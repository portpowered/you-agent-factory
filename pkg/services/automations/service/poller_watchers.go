package service

import (
	"context"
	"strings"
	"sync"

	"go.uber.org/zap"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// StartPollersForRuntime supervises all configured poller workstations until ctx is canceled.
func (s *Service) StartPollersForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	submitter WorkRequestSubmitter,
) error {
	if factoryCfg == nil || runtimeCfg == nil || sidecars == nil || submitter == nil {
		return nil
	}
	if err := s.ValidatePollersForRuntime(factoryCfg, runtimeCfg, submitter); err != nil {
		return err
	}
	return s.startPollersForRuntime(ctx, sidecars, factoryCfg, runtimeCfg, submitter)
}

func (s *Service) startPollersForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	submitter WorkRequestSubmitter,
) error {
	for _, workstation := range factoryCfg.Workstations {
		ws := workstation
		if ws.Kind != interfaces.WorkstationKindPoller {
			continue
		}

		workerName := strings.TrimSpace(ws.WorkerTypeName)
		if workerName == "" {
			s.logger().Warn("script poller disabled",
				zap.String("workstation", ws.Name),
				zap.String("reason", "missing worker binding"),
			)
			continue
		}

		workerDef, ok := runtimeCfg.Worker(workerName)
		if !ok || workerDef == nil {
			s.logger().Warn("script poller disabled",
				zap.String("workstation", ws.Name),
				zap.String("worker", workerName),
				zap.String("reason", "worker config not found"),
			)
			continue
		}
		switch {
		case interfaces.IsScriptWorkerType(workerDef.Type):
			s.StartScriptPoller(ctx, sidecars, runtimeCfg, ws, workerDef, submitter)
		case interfaces.IsPollerWorkerType(workerDef.Type):
			if workerDef.Provider != interfaces.HostedWorkerProviderLinear {
				s.logger().Warn("hosted poller disabled",
					zap.String("workstation", ws.Name),
					zap.String("worker", workerName),
					zap.String("provider", workerDef.Provider),
					zap.String("reason", "unsupported hosted provider"),
				)
				continue
			}
			if err := s.StartHostedLinearPoller(ctx, sidecars, runtimeCfg, ws, workerDef, submitter); err != nil {
				return err
			}
		default:
			continue
		}
	}
	return nil
}

// ValidatePollersForRuntime rejects invalid hosted poller construction before
// the scheduler starts any worker lifecycle.
func (s *Service) ValidatePollersForRuntime(
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	submitter WorkRequestSubmitter,
) error {
	if factoryCfg == nil || runtimeCfg == nil || submitter == nil {
		return nil
	}
	for _, workstation := range factoryCfg.Workstations {
		if workstation.Kind != interfaces.WorkstationKindPoller {
			continue
		}
		workerDef, ok := runtimeCfg.Worker(strings.TrimSpace(workstation.WorkerTypeName))
		if !ok || workerDef == nil || !interfaces.IsPollerWorkerType(workerDef.Type) || workerDef.Provider != interfaces.HostedWorkerProviderLinear {
			continue
		}
		if err := s.validateHostedLinearPoller(runtimeCfg, workstation, workerDef, submitter); err != nil {
			return err
		}
	}
	return nil
}
