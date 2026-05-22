package config

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func ruleWorkerModelOperations(cfg *interfaces.FactoryConfig) []Finding {
	if cfg == nil || len(cfg.Workers) == 0 {
		return nil
	}

	var findings []Finding
	for workerIndex, worker := range cfg.Workers {
		basePath := fmt.Sprintf("workers[%d](%s)", workerIndex, worker.Name)
		if len(worker.Operations) == 0 && strings.TrimSpace(worker.ModelLocality) == "" {
			continue
		}
		if strings.TrimSpace(worker.Type) != "" && worker.Type != interfaces.WorkerTypeModel {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath,
				Message:  "model capability declarations require worker type MODEL_WORKER",
				Rule:     "worker-model-operation-worker-type",
			})
		}

		seenOperations := make(map[string]bool, len(worker.Operations))
		for operationIndex, operation := range worker.Operations {
			operationPath := fmt.Sprintf("%s.operations[%d](%s)", basePath, operationIndex, operation.Name)
			name := strings.TrimSpace(operation.Name)
			if name == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     operationPath + ".name",
					Message:  "missing required 'name' field",
					Rule:     "worker-model-operation-name",
				})
			} else if seenOperations[name] {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     operationPath + ".name",
					Message:  fmt.Sprintf("duplicate operation name %q on the same worker", name),
					Rule:     "worker-model-operation-duplicate",
				})
			}
			seenOperations[name] = true

			findings = append(findings, validateModelOperationSlots(operation.Inputs, operationPath+".inputs", "input")...)
			findings = append(findings, validateModelOperationSlots(operation.Outputs, operationPath+".outputs", "output")...)
		}
	}
	return findings
}

func validateModelOperationSlots(slots []interfaces.ModelOperationSlot, path string, direction string) []Finding {
	if len(slots) == 0 {
		return nil
	}

	var findings []Finding
	seenSlots := make(map[string]bool, len(slots))
	for slotIndex, slot := range slots {
		slotPath := fmt.Sprintf("%s[%d](%s)", path, slotIndex, slot.Name)
		name := strings.TrimSpace(slot.Name)
		if name == "" {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     slotPath + ".name",
				Message:  "missing required 'name' field",
				Rule:     "worker-model-operation-slot-name",
			})
		} else if seenSlots[name] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     slotPath + ".name",
				Message:  fmt.Sprintf("duplicate %s slot name %q within one operation direction", direction, name),
				Rule:     "worker-model-operation-slot-duplicate",
			})
		}
		seenSlots[name] = true

		if len(slot.ContentTypes) == 0 {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     slotPath + ".contentTypes",
				Message:  "at least one content type is required",
				Rule:     "worker-model-operation-slot-content-types",
			})
		}
	}
	return findings
}
