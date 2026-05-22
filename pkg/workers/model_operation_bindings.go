package workers

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func resolveModelOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	inputTokens []interfaces.Token,
) ([]interfaces.ResolvedModelOperationBinding, error) {
	if workstationDef == nil || workerDef == nil || workstationDef.Type != interfaces.WorkstationTypeInvoke {
		return nil, nil
	}

	operationName := strings.TrimSpace(workstationDef.Operation)
	if operationName == "" {
		return nil, nil
	}

	operation, ok := workerOperationByName(workerDef.Operations, operationName)
	if !ok {
		return nil, fmt.Errorf("worker %q does not declare operation %q", workerDef.Name, operationName)
	}

	authoredBindings := make(map[string]interfaces.ModelOperationBinding, len(workstationDef.OperationBindings))
	for _, binding := range workstationDef.OperationBindings {
		if slot := strings.TrimSpace(binding.Slot); slot != "" {
			authoredBindings[slot] = binding
		}
	}

	resolved := make([]interfaces.ResolvedModelOperationBinding, 0, len(operation.Inputs))
	for _, input := range operation.Inputs {
		binding, ok := authoredBindings[input.Name]
		if !ok {
			binding = interfaces.ModelOperationBinding{
				Slot: input.Name,
				Selector: &interfaces.ModelOperationBindingSelector{
					Slot: input.Name,
				},
			}
		}
		current := resolveModelOperationBinding(binding, inputTokens)
		if len(current.Content) == 0 && input.Required {
			return nil, fmt.Errorf("required slot %q could not be resolved for operation %q", input.Name, operationName)
		}
		resolved = append(resolved, current)
	}

	return resolved, nil
}

// ResolveModelOperationBindings resolves one MODEL_INVOKE-style slot binding
// set against ordered runtime input content using the same rules as workstation
// execution.
func ResolveModelOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	inputTokens []interfaces.Token,
) ([]interfaces.ResolvedModelOperationBinding, error) {
	return resolveModelOperationBindings(workstationDef, workerDef, inputTokens)
}

func workerOperationByName(operations []interfaces.ModelOperation, name string) (interfaces.ModelOperation, bool) {
	for _, operation := range operations {
		if strings.TrimSpace(operation.Name) == name {
			return operation, true
		}
	}
	return interfaces.ModelOperation{}, false
}

func resolveModelOperationBinding(binding interfaces.ModelOperationBinding, inputTokens []interfaces.Token) interfaces.ResolvedModelOperationBinding {
	resolved := interfaces.ResolvedModelOperationBinding{
		Slot:   binding.Slot,
		Source: interfaces.ModelOperationBindingSourceOmitted,
	}

	if !selectorIsEmpty(binding.Selector) {
		if part, ok := findMatchingInputContentPart(inputTokens, binding.Selector); ok {
			resolved.Source = interfaces.ModelOperationBindingSourceInput
			resolved.Content = []interfaces.WorkContentPart{part}
			return resolved
		}
	}
	if len(binding.Config) > 0 {
		resolved.Source = interfaces.ModelOperationBindingSourceConfig
		resolved.Content = cloneResolvedBindingContent(binding.Config)
		return resolved
	}
	if len(binding.DefaultContent) > 0 {
		resolved.Source = interfaces.ModelOperationBindingSourceDefault
		resolved.Content = cloneResolvedBindingContent(binding.DefaultContent)
	}

	return resolved
}

func findMatchingInputContentPart(inputTokens []interfaces.Token, selector *interfaces.ModelOperationBindingSelector) (interfaces.WorkContentPart, bool) {
	for _, token := range inputTokens {
		for _, part := range token.Color.Content {
			if modelOperationBindingSelectorMatches(part, selector) {
				return part, true
			}
		}
	}
	return interfaces.WorkContentPart{}, false
}

func modelOperationBindingSelectorMatches(part interfaces.WorkContentPart, selector *interfaces.ModelOperationBindingSelector) bool {
	if selector == nil {
		return false
	}
	if value := strings.TrimSpace(selector.Slot); value != "" && strings.TrimSpace(part.Slot) != value {
		return false
	}
	if value := strings.TrimSpace(selector.Label); value != "" && strings.TrimSpace(part.Label) != value {
		return false
	}
	if value := strings.TrimSpace(selector.Type); value != "" && modelOperationContentTypeForPart(part) != value {
		return false
	}
	if value := strings.TrimSpace(selector.Role); value != "" && strings.TrimSpace(part.Role) != value {
		return false
	}
	return true
}

func modelOperationContentTypeForPart(part interfaces.WorkContentPart) string {
	switch part.Type.Normalized() {
	case interfaces.WorkContentPartTypeText:
		return interfaces.ModelOperationContentTypeText
	case interfaces.WorkContentPartTypeImage:
		return interfaces.ModelOperationContentTypeImage
	case interfaces.WorkContentPartTypeAudio:
		return interfaces.ModelOperationContentTypeAudio
	case interfaces.WorkContentPartTypeJSON:
		return interfaces.ModelOperationContentTypeJSON
	case interfaces.WorkContentPartTypeBinary:
		return interfaces.ModelOperationContentTypeBinary
	default:
		return strings.TrimSpace(string(part.Type))
	}
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

func cloneResolvedBindingContent(parts []interfaces.WorkContentPart) []interfaces.WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]interfaces.WorkContentPart, len(parts))
	copy(cloned, parts)
	return cloned
}
