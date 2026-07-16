package validationentry

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workercompatibility "github.com/portpowered/infinite-you/pkg/workers/compatibility"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"
)

// workerWorkstationCompatibilityTargetsFromAPI validates public worker and
// workstation taxonomy before transport mapping collapses compatibility aliases.
func workerWorkstationCompatibilityTargetsFromAPI(factory factoryapi.Factory) []factoryvalidation.Target {
	if factory.Workstations == nil || factory.Workers == nil {
		return nil
	}

	workersByName := make(map[string]factoryapi.Worker, len(*factory.Workers))
	for _, worker := range *factory.Workers {
		if strings.TrimSpace(worker.Name) != "" {
			workersByName[worker.Name] = worker
		}
	}

	var targets []factoryvalidation.Target
	for workstationIndex, workstation := range *factory.Workstations {
		worker, ok := workersByName[strings.TrimSpace(workstation.Worker)]
		if !ok || worker.Type == nil || workerMatchesWorkstationPublicAPI(string(*worker.Type), workstation) {
			continue
		}
		cfg := workstationConfigFromAPI(workstation)
		if _, ok := workercompatibility.ExpectedWorkerBehaviorClassForWorkstation(compatibilityWorkstation(cfg), string(*worker.Type)); !ok {
			continue
		}
		targets = append(targets, factoryvalidation.Target{
			Code:     factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility,
			Severity: factoryvalidation.SeverityError,
			Message: workercompatibility.WorkerWorkstationBehaviorMismatchMessage(
				workstation.Name, displayWorkstationTypeFromAPI(workstation), workertaxonomy.WorkstationKind(cfg.Kind), worker.Name, string(*worker.Type),
			),
			Subject: factoryvalidation.Subject{
				Type:     factoryvalidation.SubjectTypeWorkstation,
				ID:       interfaces.CanonicalFactoryGraphWorkstationID(cfg),
				Location: factoryvalidation.SubjectLocationReference,
			},
			Path: fmt.Sprintf("factory.workstations[%d].worker", workstationIndex),
		})
	}
	return targets
}

func workstationConfigFromAPI(workstation factoryapi.Workstation) interfaces.FactoryWorkstationConfig {
	cfg := interfaces.FactoryWorkstationConfig{Name: workstation.Name, WorkerTypeName: workstation.Worker}
	if workstation.Type != nil {
		cfg.Type = string(*workstation.Type)
	}
	if workstation.Behavior != nil {
		cfg.Kind = interfaces.WorkstationKind(*workstation.Behavior)
	}
	return cfg
}

func compatibilityWorkstation(value interfaces.FactoryWorkstationConfig) workercompatibility.Workstation {
	return workercompatibility.Workstation{
		Name: value.Name, Type: value.Type, Kind: workertaxonomy.WorkstationKind(value.Kind), WorkerTypeName: value.WorkerTypeName,
	}
}

func displayWorkstationTypeFromAPI(workstation factoryapi.Workstation) string {
	if workstation.Type != nil {
		return string(*workstation.Type)
	}
	if workstation.Behavior != nil && *workstation.Behavior == factoryapi.WorkstationKindPoller {
		return interfaces.WorkstationTypePoller
	}
	return interfaces.WorkstationTypeModel
}

func workerMatchesWorkstationPublicAPI(workerType string, workstation factoryapi.Workstation) bool {
	cfg := workstationConfigFromAPI(workstation)
	if workercompatibility.ExemptFromWorkerWorkstationCompatibility(compatibilityWorkstation(cfg)) {
		return true
	}
	workerType = strings.TrimSpace(workerType)
	if workerType == "" {
		return true
	}
	workstationType := cfg.Type
	if workstationType == string(factoryapi.WorkstationTypeModelWorkstation) && workerType == string(factoryapi.WorkerTypeModelWorker) {
		return true
	}
	if workstationType == string(factoryapi.WorkstationTypeModelInvoke) &&
		(workerType == string(factoryapi.WorkerTypeModelWorker) || workerType == string(factoryapi.WorkerTypeInferenceWorker)) {
		return true
	}
	if workerType == string(factoryapi.WorkerTypeModelWorker) {
		switch workstationType {
		case string(factoryapi.WorkstationTypeAgentRun), string(factoryapi.WorkstationTypeInferenceRun):
			return true
		}
	}
	return workercompatibility.CompatibleWorkerWorkstationBehavior(workerType, workstationType, workertaxonomy.WorkstationKind(cfg.Kind))
}
