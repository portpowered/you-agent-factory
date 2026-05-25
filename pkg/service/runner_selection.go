package service

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func validateConfiguredWorkstationRunners(factoryCfg *interfaces.FactoryConfig, factoryRunnerID string, runtimeCfg interfaces.RuntimeConfigLookup, preflight runnerSelectionPreflight) error {
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
		if !runnerSelectionRequiresValidation(selection) {
			continue
		}
		if err := validateResolvedRunnerSelection(selection, preflight); err != nil {
			return fmt.Errorf("workstations[%d](%s).runner: %w", i, workstation.Name, err)
		}
	}
	return nil
}

type runnerSelectionPreflight struct {
	skipCommandAvailability bool
}

func runnerSelectionRequiresValidation(selection interfaces.ResolvedRunnerSelection) bool {
	return selection.Source != interfaces.RunnerSelectionSourceDefault
}

func validateResolvedRunnerSelection(selection interfaces.ResolvedRunnerSelection, preflight runnerSelectionPreflight) error {
	if _, ok := interfaces.BuiltInRunnerMetadata(selection.RunnerID); !ok {
		return fmt.Errorf("unknown runner %q", selection.RunnerID)
	}
	if status, ok := workers.BuiltInRunnerStatus(selection.RunnerID); ok && !status.Available {
		return fmt.Errorf("%s", status.UnavailableReason)
	}
	if !preflight.skipCommandAvailability {
		if err := workers.ValidateBuiltInRunnerPrerequisites(selection.RunnerID); err != nil {
			return err
		}
	}
	return nil
}
