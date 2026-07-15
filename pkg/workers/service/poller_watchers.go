package service

import (
	"context"
	"strings"
	"sync"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	"go.uber.org/zap"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// StartPollersForRuntime supervises all configured poller workstations until ctx is canceled.
func (s *Service) StartPollersForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	submitter WorkRequestSubmitter,
) {
	if factoryCfg == nil || runtimeCfg == nil || sidecars == nil || submitter == nil {
		return
	}

	for _, workstation := range factoryCfg.Workstations {
		ws := workstation
		if ws.Kind != workertaxonomy.WorkstationKindPoller {
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
		case workertaxonomy.IsScriptWorkerType(workerDef.Type):
			s.StartScriptPoller(ctx, sidecars, runtimeCfg, ws, workerDef, submitter)
		case workertaxonomy.IsPollerWorkerType(workerDef.Type):
			if workerDef.Provider != workertaxonomy.HostedWorkerProviderLinear {
				s.logger().Warn("hosted poller disabled",
					zap.String("workstation", ws.Name),
					zap.String("worker", workerName),
					zap.String("provider", workerDef.Provider),
					zap.String("reason", "unsupported hosted provider"),
				)
				continue
			}
			s.StartHostedLinearPoller(ctx, sidecars, runtimeCfg, ws, workerDef, submitter)
		default:
			continue
		}
	}
}
