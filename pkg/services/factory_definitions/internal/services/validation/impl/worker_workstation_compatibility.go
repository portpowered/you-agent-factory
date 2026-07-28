package impl

import (
	"fmt"
	"strings"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// WorkerWorkstationBehaviorCompatibilityTargets returns canonical validation targets
// for incompatible worker/workstation runtime taxonomy pairings.
func WorkerWorkstationBehaviorCompatibilityTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	workersByName := make(map[string]workerconfig.Config, len(cfg.Workers))
	for _, worker := range cfg.Workers {
		workersByName[worker.Name] = worker
	}

	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		if workstation.Kind == factorydefinitions.WorkstationKindPoller &&
			strings.TrimSpace(workstation.Type) == "" {
			continue
		}
		if !factorydefinitions.RequiresWorkerWorkstationBehaviorCompatibility(
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
		if factorydefinitions.CompatibleWorkerWorkstationBehavior(
			worker.Type, workstation.Type, workstation.Kind,
		) {
			continue
		}

		basePath := fmt.Sprintf("%s.workstations[%d](%s)", validationRoot, workstationIndex, workstation.Name)
		targets = append(targets, Target{
			Code:     CodeWorkerWorkstationBehaviorCompatibility,
			Severity: SeverityError,
			Message: factorydefinitions.WorkerWorkstationBehaviorMismatchMessage(
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
	taxonomy factorydefinitions.SubmittedDefinitionTaxonomy,
) []Target {
	if len(taxonomy.Workers) == 0 || len(taxonomy.Workstations) == 0 {
		return nil
	}

	workersByName := make(map[string]factorydefinitions.SubmittedWorkerTaxonomy, len(taxonomy.Workers))
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
		if _, ok := factorydefinitions.ExpectedWorkerBehaviorClassForWorkstation(
			factorydefinitions.Workstation{
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
			Message: factorydefinitions.WorkerWorkstationBehaviorMismatchMessage(
				workstation.Name,
				submittedWorkstationTypeLabel(workstation),
				workstation.Behavior,
				worker.Name,
				worker.Type,
			),
			Subject: Subject{
				Type:     SubjectTypeWorkstation,
				ID:       factorydefinitions.CanonicalFactoryGraphWorkstationID(config),
				Location: SubjectLocationReference,
			},
			Path: fmt.Sprintf("factory.workstations[%d].worker", workstation.Index),
		})
	}
	return targets
}

func submittedWorkstationConfig(
	workstation factorydefinitions.SubmittedWorkstationTaxonomy,
) factorydefinitions.FactoryWorkstationConfig {
	return factorydefinitions.FactoryWorkstationConfig{
		Name: workstation.Name, Type: workstation.Type, Kind: workstation.Behavior,
		WorkerTypeName: workstation.Worker,
	}
}

func submittedWorkerMatchesWorkstation(
	workerType string,
	workstation factorydefinitions.SubmittedWorkstationTaxonomy,
) bool {
	config := submittedWorkstationConfig(workstation)
	value := factorydefinitions.Workstation{
		Name: config.Name, Type: config.Type, Kind: config.Kind,
		WorkerTypeName: config.WorkerTypeName,
	}
	if factorydefinitions.ExemptFromWorkerWorkstationCompatibility(value) {
		return true
	}
	workerType = strings.TrimSpace(workerType)
	if workerType == "" {
		return true
	}
	if config.Type == factorydefinitions.WorkstationTypeModel && workerType == factorydefinitions.WorkerTypeModel {
		return true
	}
	if config.Type == factorydefinitions.WorkstationTypeInvoke &&
		(workerType == factorydefinitions.WorkerTypeModel || workerType == factorydefinitions.WorkerTypeInference) {
		return true
	}
	if workerType == factorydefinitions.WorkerTypeModel {
		switch config.Type {
		case factorydefinitions.WorkstationTypeAgent, factorydefinitions.WorkstationTypeInference:
			return true
		}
	}
	return factorydefinitions.CompatibleWorkerWorkstationBehavior(workerType, config.Type, config.Kind)
}

func submittedWorkstationTypeLabel(workstation factorydefinitions.SubmittedWorkstationTaxonomy) string {
	if strings.TrimSpace(workstation.Type) != "" {
		return workstation.Type
	}
	if workstation.Behavior == factorydefinitions.WorkstationKindPoller {
		return factorydefinitions.WorkstationTypePoller
	}
	return factorydefinitions.WorkstationTypeModel
}

// PollerRunWorkstationKindTargets returns validation targets when an explicit
// POLLER_RUN workstation type conflicts with a non-poller workstation kind.
func PollerRunWorkstationKindTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		if factorydefinitions.StrictPublicFactoryWorkstationType(workstation.Type) != factorydefinitions.WorkstationTypePoller {
			continue
		}
		if workstation.Kind == "" || workstation.Kind == factorydefinitions.WorkstationKindPoller {
			continue
		}

		behaviorLabel := factorydefinitions.CanonicalPublicWorkstationKind(workstation.Kind)
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
