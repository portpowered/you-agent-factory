package validation

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	workercompatibility "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

// WorkerWorkstationBehaviorCompatibilityTargets returns canonical validation targets
// for incompatible worker/workstation runtime taxonomy pairings.
func WorkerWorkstationBehaviorCompatibilityTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	workersByName := make(map[string]workerconfig.Config, len(cfg.Workers))
	for _, worker := range cfg.Workers {
		workersByName[worker.Name] = worker
	}

	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		if workstation.Kind == interfaces.WorkstationKindPoller &&
			strings.TrimSpace(workstation.Type) == "" {
			continue
		}
		if !workercompatibility.RequiresWorkerWorkstationBehaviorCompatibility(
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
		if workercompatibility.CompatibleWorkerWorkstationBehavior(
			worker.Type, workstation.Type, workstation.Kind,
		) {
			continue
		}

		basePath := fmt.Sprintf("%s.workstations[%d](%s)", validationRoot, workstationIndex, workstation.Name)
		targets = append(targets, Target{
			Code:     CodeWorkerWorkstationBehaviorCompatibility,
			Severity: SeverityError,
			Message: workercompatibility.WorkerWorkstationBehaviorMismatchMessage(
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

// SubmittedTaxonomyCompatibilityTargets validates the exact public taxonomy
// observed before representation mapping normalizes compatibility aliases.
// The transport only copies these detached values; this service owns all
// compatibility decisions and target construction.
func SubmittedTaxonomyCompatibilityTargets(
	taxonomy interfaces.SubmittedDefinitionTaxonomy,
) []Target {
	if len(taxonomy.Workers) == 0 || len(taxonomy.Workstations) == 0 {
		return nil
	}

	workersByName := make(map[string]interfaces.SubmittedWorkerTaxonomy, len(taxonomy.Workers))
	for _, worker := range taxonomy.Workers {
		if name := strings.TrimSpace(worker.Name); name != "" {
			workersByName[name] = worker
		}
	}

	var targets []Target
	for _, workstation := range taxonomy.Workstations {
		worker, ok := workersByName[strings.TrimSpace(workstation.Worker)]
		if !ok || strings.TrimSpace(worker.Type) == "" ||
			submittedWorkerMatchesWorkstation(worker.Type, workstation) {
			continue
		}
		config := submittedWorkstationConfig(workstation)
		if _, ok := interfaces.ExpectedWorkerBehaviorClassForWorkstation(
			interfaces.Workstation{
				Name: config.Name, Type: config.Type, Kind: config.Kind,
				WorkerTypeName: config.WorkerTypeName,
			},
			worker.Type,
		); !ok {
			continue
		}
		targets = append(targets, Target{
			Code:     CodeWorkerWorkstationBehaviorCompatibility,
			Severity: SeverityError,
			Message: interfaces.WorkerWorkstationBehaviorMismatchMessage(
				workstation.Name,
				submittedWorkstationTypeLabel(workstation),
				workstation.Behavior,
				worker.Name,
				worker.Type,
			),
			Subject: Subject{
				Type:     SubjectTypeWorkstation,
				ID:       interfaces.CanonicalFactoryGraphWorkstationID(config),
				Location: SubjectLocationReference,
			},
			Path: fmt.Sprintf("factory.workstations[%d].worker", workstation.Index),
		})
	}
	return targets
}

func submittedWorkstationConfig(
	workstation interfaces.SubmittedWorkstationTaxonomy,
) interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name: workstation.Name, Type: workstation.Type, Kind: workstation.Behavior,
		WorkerTypeName: workstation.Worker,
	}
}

func submittedWorkerMatchesWorkstation(
	workerType string,
	workstation interfaces.SubmittedWorkstationTaxonomy,
) bool {
	config := submittedWorkstationConfig(workstation)
	value := interfaces.Workstation{
		Name: config.Name, Type: config.Type, Kind: config.Kind,
		WorkerTypeName: config.WorkerTypeName,
	}
	if interfaces.ExemptFromWorkerWorkstationCompatibility(value) {
		return true
	}
	workerType = strings.TrimSpace(workerType)
	if workerType == "" {
		return true
	}
	if config.Type == interfaces.WorkstationTypeModel && workerType == interfaces.WorkerTypeModel {
		return true
	}
	if config.Type == interfaces.WorkstationTypeInvoke &&
		(workerType == interfaces.WorkerTypeModel || workerType == interfaces.WorkerTypeInference) {
		return true
	}
	if workerType == interfaces.WorkerTypeModel {
		switch config.Type {
		case interfaces.WorkstationTypeAgent, interfaces.WorkstationTypeInference:
			return true
		}
	}
	return interfaces.CompatibleWorkerWorkstationBehavior(workerType, config.Type, config.Kind)
}

func submittedWorkstationTypeLabel(workstation interfaces.SubmittedWorkstationTaxonomy) string {
	if strings.TrimSpace(workstation.Type) != "" {
		return workstation.Type
	}
	if workstation.Behavior == interfaces.WorkstationKindPoller {
		return interfaces.WorkstationTypePoller
	}
	return interfaces.WorkstationTypeModel
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

		behaviorLabel := interfaces.CanonicalPublicWorkstationKind(workstation.Kind)
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
