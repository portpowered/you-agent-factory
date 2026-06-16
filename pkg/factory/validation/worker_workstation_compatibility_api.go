package validation

import (
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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

// WorkerWorkstationBehaviorCompatibilityTargets returns canonical validation targets
// for incompatible worker/workstation runtime taxonomy pairings.
func WorkerWorkstationBehaviorCompatibilityTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	workersByName := make(map[string]interfaces.WorkerConfig, len(cfg.Workers))
	for _, worker := range cfg.Workers {
		workersByName[worker.Name] = worker
	}

	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		if workstation.Kind == interfaces.WorkstationKindPoller &&
			strings.TrimSpace(workstation.Type) == "" {
			continue
		}
		if !interfaces.RequiresWorkerWorkstationBehaviorCompatibility(
			workstation.Type,
			workstation.Kind,
			workstation.WorkerTypeName,
		) {
			continue
		}

		worker, ok := workersByName[workstation.WorkerTypeName]
		if !ok {
			continue
		}
		if interfaces.CompatibleWorkerWorkstationBehavior(worker.Type, workstation.Type, workstation.Kind) {
			continue
		}

		basePath := fmt.Sprintf("%s.workstations[%d](%s)", validationRoot, workstationIndex, workstation.Name)
		targets = append(targets, Target{
			Code:     CodeWorkerWorkstationBehaviorCompatibility,
			Severity: SeverityError,
			Message: interfaces.WorkerWorkstationBehaviorMismatchMessage(
				workstation.Name,
				workstation.Type,
				workstation.Kind,
				worker.Name,
				worker.Type,
			),
			Subject: Subject{
				Type:     SubjectTypeWorkstation,
				ID:       workstation.Name,
				Location: SubjectLocationReference,
			},
			Path: basePath + ".worker",
		})
	}

	return targets
}

// PollerRunWorkstationKindTargets returns validation targets when an explicit
// POLLER_RUN workstation type conflicts with a non-poller workstation kind.
func PollerRunWorkstationKindTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		if interfaces.StrictPublicFactoryWorkstationType(workstation.Type) != interfaces.WorkstationTypePoller {
			continue
		}
		if workstation.Kind == "" || workstation.Kind == interfaces.WorkstationKindPoller {
			continue
		}

		behaviorLabel := interfaces.GeneratedPublicWorkstationKind(workstation.Kind)
		basePath := fmt.Sprintf("%s.workstations[%d](%s)", validationRoot, workstationIndex, workstation.Name)
		targets = append(targets, Target{
			Code:     CodePollerRunWorkstationKindMismatch,
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"workstation %q uses POLLER_RUN but behavior %q is not poller; set behavior to POLLER or choose a different workstation type",
				workstation.Name,
				behaviorLabel,
			),
			Subject: Subject{
				Type:     SubjectTypeWorkstation,
				ID:       workstation.Name,
				Location: SubjectLocationReference,
			},
			Path: basePath + ".behavior",
		})
	}

	return targets
}
