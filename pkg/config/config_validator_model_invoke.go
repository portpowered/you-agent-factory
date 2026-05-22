package config

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func ruleModelInvokeWorkstations(cfg *interfaces.FactoryConfig) []Finding {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	workersByName := make(map[string]interfaces.WorkerConfig, len(cfg.Workers))
	for _, worker := range cfg.Workers {
		workersByName[worker.Name] = worker
	}

	var findings []Finding
	for workstationIndex, workstation := range cfg.Workstations {
		basePath := fmt.Sprintf("workstations[%d](%s)", workstationIndex, workstation.Name)
		workstationType := strings.TrimSpace(workstation.Type)
		operationName := strings.TrimSpace(workstation.Operation)

		if workstationType != interfaces.WorkstationTypeInvoke {
			if operationName != "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".operation",
					Message:  "operation is only supported on MODEL_INVOKE workstations",
					Rule:     "workstation-model-invoke-type",
				})
			}
			continue
		}

		if operationName == "" {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".operation",
				Message:  "MODEL_INVOKE workstation requires an uppercase operation name",
				Rule:     "workstation-model-invoke-operation",
			})
		}
		if strings.TrimSpace(workstation.WorkerTypeName) == "" {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".worker",
				Message:  "MODEL_INVOKE workstation requires a worker reference",
				Rule:     "workstation-model-invoke-worker",
			})
			continue
		}

		worker, ok := workersByName[workstation.WorkerTypeName]
		if !ok {
			continue
		}
		if strings.TrimSpace(worker.Type) != "" && worker.Type != interfaces.WorkerTypeModel {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".worker",
				Message:  fmt.Sprintf("worker %q is incompatible with MODEL_INVOKE; declare type MODEL_WORKER and model operations", workstation.WorkerTypeName),
				Rule:     "workstation-model-invoke-worker-compatibility",
			})
			continue
		}
		if operationName == "" {
			continue
		}

		operation, found := findWorkerOperation(worker.Operations, operationName)
		if !found {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".operation",
				Message:  fmt.Sprintf("worker %q does not declare requested operation %q", workstation.WorkerTypeName, operationName),
				Rule:     "workstation-model-invoke-operation-mismatch",
			})
			continue
		}
		if len(operation.Inputs) == 0 || len(operation.Outputs) == 0 {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".operation",
				Message:  fmt.Sprintf("worker %q operation %q has an incompatible content contract; MODEL_INVOKE requires at least one input slot and one output slot", workstation.WorkerTypeName, operationName),
				Rule:     "workstation-model-invoke-content-contract",
			})
		}
	}

	return findings
}

func findWorkerOperation(operations []interfaces.ModelOperation, name string) (interfaces.ModelOperation, bool) {
	for _, operation := range operations {
		if strings.TrimSpace(operation.Name) == name {
			return operation, true
		}
	}
	return interfaces.ModelOperation{}, false
}
