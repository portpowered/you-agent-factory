package inference

import (
	"encoding/json"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	contentcontract "github.com/portpowered/infinite-you/pkg/work/content/contract"
)

// ResolveInferenceOperationBindings resolves supported operation input slots for
// inference-run workstations into provider-neutral binding content.
func ResolveInferenceOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	inputTokens []interfaces.Token,
) ([]interfaces.ResolvedModelOperationBinding, error) {
	if workstationDef == nil || workerDef == nil || !interfaces.IsInferenceRunWorkstationType(workstationDef.Type) {
		return nil, nil
	}

	operationName := strings.TrimSpace(workstationDef.Operation)
	if operationName == "" {
		return nil, nil
	}

	operation, ok := modelOperationByName(workerDef.Operations, operationName)
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

// OperationBindingsFromGenerated maps authored OpenAPI operation bindings onto the
// internal inference binding contract.
func OperationBindingsFromGenerated(values *[]factoryapi.WorkstationOperationBinding) []interfaces.ModelOperationBinding {
	if values == nil || len(*values) == 0 {
		return nil
	}
	bindings := make([]interfaces.ModelOperationBinding, 0, len(*values))
	for _, binding := range *values {
		current := interfaces.ModelOperationBinding{
			Slot:           strings.TrimSpace(binding.Slot),
			Config:         contentcontract.PartsFromGenerated(binding.Config),
			DefaultContent: contentcontract.PartsFromGenerated(binding.DefaultContent),
		}
		if binding.Selector != nil {
			current.Selector = &interfaces.ModelOperationBindingSelector{
				Slot:  stringValue(binding.Selector.Slot),
				Label: stringValue(binding.Selector.Label),
				Type:  stringValue(binding.Selector.Type),
				Role:  stringValue(binding.Selector.Role),
			}
		}
		bindings = append(bindings, current)
	}
	return bindings
}

// DirectInferenceWorkstationConfig builds a synthetic inference-run workstation
// definition for direct model invocation using the same binding contract as
// authored INFERENCE_RUN workstations.
func DirectInferenceWorkstationConfig(operation string, bindings []interfaces.ModelOperationBinding) *interfaces.FactoryWorkstationConfig {
	return &interfaces.FactoryWorkstationConfig{
		Type:              interfaces.WorkstationTypeInference,
		Operation:         strings.TrimSpace(operation),
		OperationBindings: bindings,
	}
}

// InferenceOperationUserMessage builds the provider-neutral inference request
// envelope shared by direct and session inference execution paths.
func InferenceOperationUserMessage(
	operation string,
	inputContent []interfaces.WorkContentPart,
	bindings []interfaces.ResolvedModelOperationBinding,
) string {
	payload := struct {
		Operation string                                     `json:"operation"`
		Input     []interfaces.WorkContentPart               `json:"input,omitempty"`
		Bindings  []interfaces.ResolvedModelOperationBinding `json:"bindings,omitempty"`
	}{
		Operation: strings.TrimSpace(operation),
		Input:     append([]interfaces.WorkContentPart(nil), inputContent...),
		Bindings:  interfaces.CloneResolvedModelOperationBindings(bindings),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return strings.TrimSpace(operation)
	}
	return string(encoded)
}

// WorkContentFromInferenceOutput maps one inference response body onto ordered
// canonical WorkContent parts declared by the target operation.
func WorkContentFromInferenceOutput(raw string, operation interfaces.ModelOperation) ([]interfaces.WorkContentPart, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	var content factoryapi.WorkContent
	if err := json.Unmarshal([]byte(trimmed), &content); err == nil {
		return orderWorkContentByOperationOutputs(contentcontract.PartsFromGenerated(&content), operation), nil
	}
	var envelope struct {
		Content factoryapi.WorkContent `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && envelope.Content != nil {
		return orderWorkContentByOperationOutputs(contentcontract.PartsFromGenerated(&envelope.Content), operation), nil
	}
	var parts []interfaces.WorkContentPart
	if err := json.Unmarshal([]byte(trimmed), &parts); err == nil {
		return orderWorkContentByOperationOutputs(parts, operation), nil
	}
	if modelOperationHasOnlyTextOutputs(operation) {
		return []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: raw,
		}}, nil
	}
	return nil, fmt.Errorf("inference response is not valid WorkContent JSON for operation %q", strings.TrimSpace(operation.Name))
}

// MarshalWorkContentOutput serializes canonical WorkContent parts for accepted
// inference workstation output.
func MarshalWorkContentOutput(parts []interfaces.WorkContentPart) (string, error) {
	if len(parts) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(parts)
	if err != nil {
		return "", fmt.Errorf("marshal inference WorkContent output: %w", err)
	}
	return string(encoded), nil
}

func modelOperationByName(operations []interfaces.ModelOperation, name string) (interfaces.ModelOperation, bool) {
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

func modelOperationHasOnlyTextOutputs(operation interfaces.ModelOperation) bool {
	if len(operation.Outputs) == 0 {
		return true
	}
	for _, output := range operation.Outputs {
		if len(output.ContentTypes) == 0 {
			return false
		}
		for _, contentType := range output.ContentTypes {
			if strings.TrimSpace(contentType) != interfaces.ModelOperationContentTypeText {
				return false
			}
		}
	}
	return true
}

func orderWorkContentByOperationOutputs(parts []interfaces.WorkContentPart, operation interfaces.ModelOperation) []interfaces.WorkContentPart {
	if len(parts) == 0 || len(operation.Outputs) == 0 {
		return parts
	}

	ordered := make([]interfaces.WorkContentPart, 0, len(parts))
	used := make([]bool, len(parts))
	for _, slot := range operation.Outputs {
		wantTypes := slotContentTypes(slot)
		if len(wantTypes) == 0 {
			continue
		}
		for i, part := range parts {
			if used[i] {
				continue
			}
			partType := modelOperationContentTypeForPart(part)
			for _, wantType := range wantTypes {
				if partType == wantType {
					ordered = append(ordered, part)
					used[i] = true
					break
				}
			}
			if used[i] {
				break
			}
		}
	}
	for i, part := range parts {
		if !used[i] {
			ordered = append(ordered, part)
		}
	}
	return ordered
}

func slotContentTypes(slot interfaces.ModelOperationSlot) []string {
	if len(slot.ContentTypes) == 0 {
		return nil
	}
	types := make([]string, 0, len(slot.ContentTypes))
	for _, contentType := range slot.ContentTypes {
		if trimmed := strings.TrimSpace(contentType); trimmed != "" {
			types = append(types, trimmed)
		}
	}
	return types
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(string(*value))
}
