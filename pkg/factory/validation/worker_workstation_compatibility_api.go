package validation

import (
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// WorkerWorkstationCompatibilityTargets validates that each workstation references
// a worker with a compatible inference, agent, script, or poller behavior class.
func WorkerWorkstationCompatibilityTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil {
		return nil
	}

	workersByName := make(map[string]interfaces.WorkerConfig, len(cfg.Workers))
	for _, worker := range cfg.Workers {
		if strings.TrimSpace(worker.Name) == "" {
			continue
		}
		workersByName[worker.Name] = worker
	}

	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		workerName := strings.TrimSpace(workstation.WorkerTypeName)
		if workerName == "" {
			continue
		}
		worker, ok := workersByName[workerName]
		if !ok {
			continue
		}
		if interfaces.WorkerMatchesWorkstationBehavior(worker.Type, workstation) {
			continue
		}

		targets = append(targets, Target{
			Code:     CodeWorkerWorkstationBehaviorCompatibility,
			Severity: SeverityError,
			Message: interfaces.WorkerWorkstationBehaviorMismatchMessage(
				workstation.Name,
				workstation.Type,
				workstation.Kind,
				worker.Name,
				strings.TrimSpace(worker.Type),
			),
			Subject: Subject{
				Type:     SubjectTypeWorkstation,
				ID:       interfaces.CanonicalFactoryGraphWorkstationID(workstation),
				Location: SubjectLocationReference,
			},
			Path: fmt.Sprintf("%s.workstations[%d].worker", validationRoot, workstationIndex),
		})
	}
	return targets
}

// WorkerWorkstationCompatibilityTargetsFromAPI validates worker/workstation behavior
// pairings on the public OpenAPI factory payload before internal runtime projection
// collapses new taxonomy names onto legacy runtime identifiers.
func WorkerWorkstationCompatibilityTargetsFromAPI(factory factoryapi.Factory) []Target {
	if factory.Workstations == nil || factory.Workers == nil {
		return nil
	}

	workersByName := make(map[string]factoryapi.Worker, len(*factory.Workers))
	for _, worker := range *factory.Workers {
		if strings.TrimSpace(worker.Name) == "" {
			continue
		}
		workersByName[worker.Name] = worker
	}

	var targets []Target
	for workstationIndex, workstation := range *factory.Workstations {
		workerName := strings.TrimSpace(workstation.Worker)
		if workerName == "" {
			continue
		}
		worker, ok := workersByName[workerName]
		if !ok || worker.Type == nil {
			continue
		}
		if workerMatchesWorkstationPublicAPI(string(*worker.Type), workstation) {
			continue
		}

		workstationType := displayWorkstationTypeFromAPI(workstation)
		if _, hasExpected := expectedWorkerBehaviorClassFromAPI(workstation, string(*worker.Type)); !hasExpected {
			continue
		}

		targets = append(targets, Target{
			Code:     CodeWorkerWorkstationBehaviorCompatibility,
			Severity: SeverityError,
			Message: interfaces.WorkerWorkstationBehaviorMismatchMessage(
				workstation.Name,
				workstationType,
				workstationConfigFromAPI(workstation).Kind,
				worker.Name,
				string(*worker.Type),
			),
			Subject: Subject{
				Type:     SubjectTypeWorkstation,
				ID:       interfaces.CanonicalFactoryGraphWorkstationID(workstationConfigFromAPI(workstation)),
				Location: SubjectLocationReference,
			},
			Path: fmt.Sprintf("%s.workstations[%d].worker", validationRoot, workstationIndex),
		})
	}
	return targets
}

func workstationConfigFromAPI(workstation factoryapi.Workstation) interfaces.FactoryWorkstationConfig {
	cfg := interfaces.FactoryWorkstationConfig{
		Name:           workstation.Name,
		WorkerTypeName: workstation.Worker,
	}
	if workstation.Type != nil {
		cfg.Type = string(*workstation.Type)
	}
	if workstation.Behavior != nil {
		cfg.Kind = interfaces.WorkstationKind(*workstation.Behavior)
	}
	return cfg
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

func expectedWorkerBehaviorClassFromAPI(workstation factoryapi.Workstation, workerType string) (interfaces.WorkerWorkstationBehaviorClass, bool) {
	return interfaces.ExpectedWorkerBehaviorClassForWorkstation(workstationConfigFromAPI(workstation), workerType)
}

func workerMatchesWorkstationPublicAPI(workerType string, workstation factoryapi.Workstation) bool {
	cfg := workstationConfigFromAPI(workstation)
	if interfaces.ExemptFromWorkerWorkstationCompatibility(cfg) {
		return true
	}

	workerType = strings.TrimSpace(workerType)
	if workerType == "" {
		return true
	}

	workstationType := ""
	if workstation.Type != nil {
		workstationType = string(*workstation.Type)
	}

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

	return interfaces.CompatibleWorkerWorkstationBehavior(
		workerType,
		workstationType,
		cfg.Kind,
	)
}
