package validation

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
