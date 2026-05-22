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
		findings = append(findings, validateModelInvokeWorkstation(workstation, workstationIndex, workersByName)...)
	}

	return findings
}

func validateModelInvokeWorkstation(workstation interfaces.FactoryWorkstationConfig, workstationIndex int, workersByName map[string]interfaces.WorkerConfig) []Finding {
	basePath := fmt.Sprintf("workstations[%d](%s)", workstationIndex, workstation.Name)
	operationName := strings.TrimSpace(workstation.Operation)
	if strings.TrimSpace(workstation.Type) != interfaces.WorkstationTypeInvoke {
		return validateNonInvokeOperationUsage(basePath, operationName)
	}

	findings := requiredModelInvokeWorkstationFindings(workstation, basePath, operationName)
	if strings.TrimSpace(workstation.WorkerTypeName) == "" {
		return findings
	}

	worker, ok := workersByName[workstation.WorkerTypeName]
	if !ok {
		return findings
	}
	workerFindings, operation, ok := validateModelInvokeWorker(workstation, worker, basePath, operationName)
	findings = append(findings, workerFindings...)
	if !ok {
		return findings
	}

	return append(findings, validateModelOperationBindings(workstation.OperationBindings, operation.Inputs, basePath+".operationBindings")...)
}

func validateNonInvokeOperationUsage(basePath string, operationName string) []Finding {
	if operationName == "" {
		return nil
	}
	return []Finding{{
		Severity: SeverityError,
		Path:     basePath + ".operation",
		Message:  "operation is only supported on MODEL_INVOKE workstations",
		Rule:     "workstation-model-invoke-type",
	}}
}

func requiredModelInvokeWorkstationFindings(workstation interfaces.FactoryWorkstationConfig, basePath string, operationName string) []Finding {
	var findings []Finding
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
	}
	return findings
}

func validateModelInvokeWorker(workstation interfaces.FactoryWorkstationConfig, worker interfaces.WorkerConfig, basePath string, operationName string) ([]Finding, interfaces.ModelOperation, bool) {
	if strings.TrimSpace(worker.Type) != "" && worker.Type != interfaces.WorkerTypeModel {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".worker",
			Message:  fmt.Sprintf("worker %q is incompatible with MODEL_INVOKE; declare type MODEL_WORKER and model operations", workstation.WorkerTypeName),
			Rule:     "workstation-model-invoke-worker-compatibility",
		}}, interfaces.ModelOperation{}, false
	}
	if operationName == "" {
		return nil, interfaces.ModelOperation{}, false
	}

	operation, found := findWorkerOperation(worker.Operations, operationName)
	if !found {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".operation",
			Message:  fmt.Sprintf("worker %q does not declare requested operation %q", workstation.WorkerTypeName, operationName),
			Rule:     "workstation-model-invoke-operation-mismatch",
		}}, interfaces.ModelOperation{}, false
	}
	if len(operation.Inputs) == 0 || len(operation.Outputs) == 0 {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".operation",
			Message:  fmt.Sprintf("worker %q operation %q has an incompatible content contract; MODEL_INVOKE requires at least one input slot and one output slot", workstation.WorkerTypeName, operationName),
			Rule:     "workstation-model-invoke-content-contract",
		}}, interfaces.ModelOperation{}, false
	}

	return nil, operation, true
}

func findWorkerOperation(operations []interfaces.ModelOperation, name string) (interfaces.ModelOperation, bool) {
	for _, operation := range operations {
		if strings.TrimSpace(operation.Name) == name {
			return operation, true
		}
	}
	return interfaces.ModelOperation{}, false
}

func validateModelOperationBindings(bindings []interfaces.ModelOperationBinding, inputs []interfaces.ModelOperationSlot, path string) []Finding {
	if len(bindings) == 0 {
		return nil
	}

	knownSlots := make(map[string]bool, len(inputs))
	for _, slot := range inputs {
		name := strings.TrimSpace(slot.Name)
		if name != "" {
			knownSlots[name] = true
		}
	}

	seen := make(map[string]bool, len(bindings))
	var findings []Finding
	for i, binding := range bindings {
		bindingPath := fmt.Sprintf("%s[%d](%s)", path, i, binding.Slot)
		slotName := strings.TrimSpace(binding.Slot)
		if slotName == "" {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     bindingPath + ".slot",
				Message:  "operation binding requires a slot name",
				Rule:     "workstation-model-invoke-binding-slot",
			})
			continue
		}
		if seen[slotName] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     bindingPath + ".slot",
				Message:  fmt.Sprintf("duplicate operation binding for slot %q", slotName),
				Rule:     "workstation-model-invoke-binding-duplicate",
			})
			continue
		}
		seen[slotName] = true
		if !knownSlots[slotName] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     bindingPath + ".slot",
				Message:  fmt.Sprintf("operation binding references unknown input slot %q", slotName),
				Rule:     "workstation-model-invoke-binding-unknown-slot",
			})
		}
		if selectorIsEmpty(binding.Selector) && len(binding.Config) == 0 && len(binding.DefaultContent) == 0 {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     bindingPath,
				Message:  "operation binding must declare a selector, config content, or default content",
				Rule:     "workstation-model-invoke-binding-empty",
			})
		}
	}
	return findings
}

func selectorIsEmpty(selector *interfaces.ModelOperationBindingSelector) bool {
	if selector == nil {
		return true
	}
	return strings.TrimSpace(selector.Slot) == "" &&
		strings.TrimSpace(selector.Label) == "" &&
		strings.TrimSpace(selector.Type) == "" &&
		strings.TrimSpace(selector.Role) == ""
}
