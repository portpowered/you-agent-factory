package service

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func validateConfiguredWorkstationRunners(factoryCfg *interfaces.FactoryConfig, factoryRunnerID string, runtimeCfg interfaces.RuntimeConfigLookup) error {
	if factoryCfg == nil {
		return nil
	}
	for i, workstation := range factoryCfg.Workstations {
		runtimeWorkstation, ok := runtimeCfg.Workstation(workstation.Name)
		if ok && runtimeWorkstation != nil {
			workstation = *runtimeWorkstation
		}

		worker, _ := runtimeCfg.Worker(workstation.WorkerTypeName)
		workerModelProvider := ""
		if worker != nil {
			workerModelProvider = worker.ModelProvider
		}

		selection := interfaces.ResolveRunnerSelection(workstation.Runner, factoryRunnerID, workerModelProvider)
		if !runnerSelectionRequiresValidation(selection, workstation.Runner, factoryRunnerID, workerModelProvider) {
			continue
		}
		if err := validateResolvedRunnerSelection(selection); err != nil {
			return fmt.Errorf("workstations[%d](%s).runner: %w", i, workstation.Name, err)
		}
	}
	return nil
}

func runnerSelectionRequiresValidation(selection interfaces.ResolvedRunnerSelection, workstationRunner, factoryRunnerID, workerModelProvider string) bool {
	if workstationRunner != "" || factoryRunnerID != "" {
		return true
	}
	return interfaces.IsBuiltInRunnerID(workerModelProvider) && selection.RunnerID != ""
}

func validateResolvedRunnerSelection(selection interfaces.ResolvedRunnerSelection) error {
	if _, ok := interfaces.BuiltInRunnerMetadata(selection.RunnerID); !ok {
		return fmt.Errorf("unknown runner %q", selection.RunnerID)
	}
	if status, ok := workers.BuiltInRunnerStatus(selection.RunnerID); ok && !status.Available {
		return fmt.Errorf("%s", status.UnavailableReason)
	}
	return nil
}
