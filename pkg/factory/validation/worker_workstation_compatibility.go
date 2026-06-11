package validation

import (
	"fmt"
	"strings"

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

		expected, ok := interfaces.ExpectedWorkerBehaviorClassForWorkstation(workstation, worker.Type)
		if !ok {
			continue
		}
		actual, ok := interfaces.WorkerBehaviorClass(worker.Type)
		if !ok {
			continue
		}

		targets = append(targets, Target{
			Code:     CodeWorkerWorkstationIncompatibleBehavior,
			Severity: SeverityError,
			Message: fmt.Sprintf(
				`workstation %q type %s requires a %s worker, but worker %q type %s is a %s worker`,
				workstation.Name,
				interfaces.DisplayWorkstationTypeForCompatibility(workstation),
				expected,
				worker.Name,
				strings.TrimSpace(worker.Type),
				actual,
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
