package runtime

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// resolveRuntimeModelOperationBindings projects authored inference operation
// bindings into the detached Workers request. Factory Runtime owns this
// Definitions-to-execution translation because Workers must not depend on
// Factory Definitions internals.
func resolveRuntimeModelOperationBindings(
	workstation *interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	inputTokens []workers.Token,
) ([]workers.ResolvedModelOperationBinding, error) {
	if workstation == nil || worker == nil || !interfaces.IsInferenceRunWorkstationType(workstation.Type) {
		return nil, nil
	}
	operationName := strings.TrimSpace(workstation.Operation)
	if operationName == "" {
		return nil, nil
	}
	operation, found := runtimeModelOperationByName(worker.Operations, operationName)
	if !found {
		return nil, fmt.Errorf("worker %q does not declare operation %q", worker.Name, operationName)
	}

	authoredBindings := make(map[string]interfaces.ModelOperationBinding, len(workstation.OperationBindings))
	for _, binding := range workstation.OperationBindings {
		if slot := strings.TrimSpace(binding.Slot); slot != "" {
			authoredBindings[slot] = binding
		}
	}
	inputTokens = runtimeNonResourceTokens(inputTokens)
	resolved := make([]workers.ResolvedModelOperationBinding, 0, len(operation.Inputs))
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
		current := resolveRuntimeModelOperationBinding(binding, inputTokens)
		if len(current.Content) == 0 && input.Required {
			return nil, fmt.Errorf("required slot %q could not be resolved for operation %q", input.Name, operationName)
		}
		resolved = append(resolved, current)
	}
	return resolved, nil
}

func runtimeModelOperationByName(
	operations []interfaces.ModelOperation,
	name string,
) (interfaces.ModelOperation, bool) {
	for _, operation := range operations {
		if strings.TrimSpace(operation.Name) == name {
			return operation, true
		}
	}
	return interfaces.ModelOperation{}, false
}

func resolveRuntimeModelOperationBinding(
	binding interfaces.ModelOperationBinding,
	inputTokens []workers.Token,
) workers.ResolvedModelOperationBinding {
	resolved := workers.ResolvedModelOperationBinding{
		Slot:   binding.Slot,
		Source: workers.ModelOperationBindingSourceOmitted,
	}
	if !runtimeModelOperationSelectorIsEmpty(binding.Selector) {
		if part, ok := runtimeMatchingModelOperationContentPart(inputTokens, binding.Selector); ok {
			resolved.Source = workers.ModelOperationBindingSourceInput
			resolved.Content = []work.WorkContentPart{part}
			return resolved
		}
	}
	if len(binding.Config) > 0 {
		resolved.Source = workers.ModelOperationBindingSourceConfig
		resolved.Content = work.CloneWorkContentParts(binding.Config)
		return resolved
	}
	if len(binding.DefaultContent) > 0 {
		resolved.Source = workers.ModelOperationBindingSourceDefault
		resolved.Content = work.CloneWorkContentParts(binding.DefaultContent)
	}
	return resolved
}

func runtimeMatchingModelOperationContentPart(
	inputTokens []workers.Token,
	selector *interfaces.ModelOperationBindingSelector,
) (work.WorkContentPart, bool) {
	for _, token := range inputTokens {
		for _, part := range token.Color.Content {
			if runtimeModelOperationSelectorMatches(part, selector) {
				return part, true
			}
		}
	}
	return work.WorkContentPart{}, false
}

func runtimeModelOperationSelectorMatches(
	part work.WorkContentPart,
	selector *interfaces.ModelOperationBindingSelector,
) bool {
	if selector == nil {
		return false
	}
	if value := strings.TrimSpace(selector.Slot); value != "" && strings.TrimSpace(part.Slot) != value {
		return false
	}
	if value := strings.TrimSpace(selector.Label); value != "" && strings.TrimSpace(part.Label) != value {
		return false
	}
	if value := strings.TrimSpace(selector.Type); value != "" && runtimeModelOperationContentType(part) != value {
		return false
	}
	if value := strings.TrimSpace(selector.Role); value != "" && strings.TrimSpace(part.Role) != value {
		return false
	}
	return true
}

func runtimeModelOperationContentType(part work.WorkContentPart) string {
	switch part.Type.Normalized() {
	case work.WorkContentPartTypeText:
		return interfaces.ModelOperationContentTypeText
	case work.WorkContentPartTypeImage:
		return interfaces.ModelOperationContentTypeImage
	case work.WorkContentPartTypeAudio:
		return interfaces.ModelOperationContentTypeAudio
	case work.WorkContentPartTypeJSON:
		return interfaces.ModelOperationContentTypeJSON
	case work.WorkContentPartTypeBinary:
		return interfaces.ModelOperationContentTypeBinary
	default:
		return strings.TrimSpace(string(part.Type))
	}
}

func runtimeModelOperationSelectorIsEmpty(
	selector *interfaces.ModelOperationBindingSelector,
) bool {
	if selector == nil {
		return true
	}
	return strings.TrimSpace(selector.Slot) == "" &&
		strings.TrimSpace(selector.Label) == "" &&
		strings.TrimSpace(selector.Type) == "" &&
		strings.TrimSpace(selector.Role) == ""
}

func runtimeNonResourceTokens(tokens []workers.Token) []workers.Token {
	if len(tokens) == 0 {
		return nil
	}
	filtered := make([]workers.Token, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == workers.DataTypeResource {
			continue
		}
		filtered = append(filtered, token)
	}
	return filtered
}
