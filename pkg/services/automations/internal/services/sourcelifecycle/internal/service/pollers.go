package service

import (
	"context"
	"strings"
	"sync"

	"go.uber.org/zap"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func (s *service) startPollersForRuntime(
	configuration sourcelifecycle.RuntimeSourceConfiguration,
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	submitter automations.WorkRequestSubmitter,
) error {
	for _, workstation := range factoryCfg.Workstations {
		ws := workstation
		if ws.Kind != interfaces.WorkstationKindPoller {
			continue
		}

		workerName := strings.TrimSpace(ws.WorkerTypeName)
		if workerName == "" {
			s.logger.Warn("script poller disabled",
				zap.String("workstation", ws.Name),
				zap.String("reason", "missing worker binding"),
			)
			continue
		}

		workerDef, ok := runtimeCfg.Worker(workerName)
		if !ok || workerDef == nil {
			s.logger.Warn("script poller disabled",
				zap.String("workstation", ws.Name),
				zap.String("worker", workerName),
				zap.String("reason", "worker config not found"),
			)
			continue
		}
		switch {
		case interfaces.IsScriptWorkerType(workerDef.Type):
			supervision := scriptpollers.SupervisionFor(configuration.WorkflowID, ws.Name)
			supervision.CursorScope = scriptpollers.CursorScope{RuntimeID: configuration.RuntimeID, BaseDir: configuration.CursorBaseDir}
			s.scriptPollers.StartScriptPoller(ctx, sidecars, runtimeCfg, ws, workerDef, supervision, submitter)
		case interfaces.IsPollerWorkerType(workerDef.Type):
			if workerDef.Provider != interfaces.HostedWorkerProviderLinear {
				s.logger.Warn("hosted poller disabled",
					zap.String("workstation", ws.Name),
					zap.String("worker", workerName),
					zap.String("provider", workerDef.Provider),
					zap.String("reason", "unsupported hosted provider"),
				)
				continue
			}
			if err := s.hostedPollers.StartLinearPoller(ctx, sidecars, runtimeCfg, ws, workerDef, automations.HostedWorkSubmitter(submitter)); err != nil {
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
func (s *service) ValidatePollersForRuntime(
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	submitter automations.WorkRequestSubmitter,
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
		if err := s.hostedPollers.ValidateLinearPoller(runtimeCfg, workstation, workerDef, automations.HostedWorkSubmitter(submitter)); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) validatePollers(config *runtimeSnapshotConfig, submitter automations.WorkRequestSubmitter) error {
	return s.ValidatePollersForRuntime(config.FactoryConfig(), config, submitter)
}

func (s *service) launch(ctx context.Context, children *sync.WaitGroup, automationID string, configuration sourcelifecycle.RuntimeSourceConfiguration, config *runtimeSnapshotConfig) error {
	if !configuration.Inputs.StartSchedulers {
		return nil
	}
	s.StartCronWatchersForRuntime(ctx, children, automationID, config.FactoryConfig(), config, configuration.Inputs.Submitter)
	return s.startPollersForRuntime(configuration, ctx, children, config.FactoryConfig(), config, configuration.Inputs.Submitter)
}
