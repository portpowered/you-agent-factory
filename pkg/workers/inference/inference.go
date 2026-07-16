package inference

import (
	"encoding/json"
	"fmt"
	"strings"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/work"
	contentcontract "github.com/portpowered/infinite-you/pkg/work/content/contract"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// ResolveInferenceOperationBindings resolves supported operation input slots for
// inference-run workstations into provider-neutral binding content.
func ResolveInferenceOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *workerconfig.Config,
	inputTokens []factorytoken.Token,
) ([]workerexecution.ResolvedModelOperationBinding, error) {
	if workstationDef == nil || workerDef == nil || !workertaxonomy.IsInferenceRunWorkstationType(workstationDef.Type) {
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

	resolved := make([]workerexecution.ResolvedModelOperationBinding, 0, len(operation.Inputs))
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

// DirectInferenceWorkstationConfig builds a synthetic inference-run workstation
// definition for direct model invocation using the same binding contract as
// authored INFERENCE_RUN workstations.
func DirectInferenceWorkstationConfig(operation string, bindings []interfaces.ModelOperationBinding) *interfaces.FactoryWorkstationConfig {
	return &interfaces.FactoryWorkstationConfig{
		Type:              workertaxonomy.WorkstationTypeInference,
		Operation:         strings.TrimSpace(operation),
		OperationBindings: bindings,
	}
}

// InferenceOperationUserMessage builds the provider-neutral inference request
// envelope shared by direct and session inference execution paths.
func InferenceOperationUserMessage(
	operation string,
	inputContent []work.WorkContentPart,
	bindings []workerexecution.ResolvedModelOperationBinding,
) string {
	payload := struct {
		Operation string                                          `json:"operation"`
		Input     []work.WorkContentPart                          `json:"input,omitempty"`
		Bindings  []workerexecution.ResolvedModelOperationBinding `json:"bindings,omitempty"`
	}{
		Operation: strings.TrimSpace(operation),
		Input:     append([]work.WorkContentPart(nil), inputContent...),
		Bindings:  workerexecution.CloneResolvedModelOperationBindings(bindings),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return strings.TrimSpace(operation)
	}
	return string(encoded)
}

// WorkContentFromInferenceOutput maps one inference response body onto ordered
// canonical WorkContent parts declared by the target operation.
func WorkContentFromInferenceOutput(raw string, operation workerconfig.ModelOperation) ([]work.WorkContentPart, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	var content []work.WorkContentPart
	if err := json.Unmarshal([]byte(trimmed), &content); err == nil {
		return orderWorkContentByOperationOutputs(contentcontract.SupportedParts(content), operation), nil
	}
	var envelope struct {
		Content []work.WorkContentPart `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && envelope.Content != nil {
		return orderWorkContentByOperationOutputs(contentcontract.SupportedParts(envelope.Content), operation), nil
	}
	if modelOperationHasOnlyTextOutputs(operation) {
		return []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: raw,
		}}, nil
	}
	return nil, fmt.Errorf("inference response is not valid WorkContent JSON for operation %q", strings.TrimSpace(operation.Name))
}

// MarshalWorkContentOutput serializes canonical WorkContent parts for accepted
// inference workstation output.
func MarshalWorkContentOutput(parts []work.WorkContentPart) (string, error) {
	if len(parts) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(parts)
	if err != nil {
		return "", fmt.Errorf("marshal inference WorkContent output: %w", err)
	}
	return string(encoded), nil
}

func modelOperationByName(operations []workerconfig.ModelOperation, name string) (workerconfig.ModelOperation, bool) {
	for _, operation := range operations {
		if strings.TrimSpace(operation.Name) == name {
			return operation, true
		}
	}
	return workerconfig.ModelOperation{}, false
}

func resolveModelOperationBinding(binding interfaces.ModelOperationBinding, inputTokens []factorytoken.Token) workerexecution.ResolvedModelOperationBinding {
	resolved := workerexecution.ResolvedModelOperationBinding{
		Slot:   binding.Slot,
		Source: workerexecution.ModelOperationBindingSourceOmitted,
	}

	if !selectorIsEmpty(binding.Selector) {
		if part, ok := findMatchingInputContentPart(inputTokens, binding.Selector); ok {
			resolved.Source = workerexecution.ModelOperationBindingSourceInput
			resolved.Content = []work.WorkContentPart{part}
			return resolved
		}
	}
	if len(binding.Config) > 0 {
		resolved.Source = workerexecution.ModelOperationBindingSourceConfig
		resolved.Content = cloneResolvedBindingContent(binding.Config)
		return resolved
	}
	if len(binding.DefaultContent) > 0 {
		resolved.Source = workerexecution.ModelOperationBindingSourceDefault
		resolved.Content = cloneResolvedBindingContent(binding.DefaultContent)
	}

	return resolved
}

func findMatchingInputContentPart(inputTokens []factorytoken.Token, selector *interfaces.ModelOperationBindingSelector) (work.WorkContentPart, bool) {
	for _, token := range inputTokens {
		for _, part := range token.Color.Content {
			if modelOperationBindingSelectorMatches(part, selector) {
				return part, true
			}
		}
	}
	return work.WorkContentPart{}, false
}

func modelOperationBindingSelectorMatches(part work.WorkContentPart, selector *interfaces.ModelOperationBindingSelector) bool {
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

func modelOperationContentTypeForPart(part work.WorkContentPart) string {
	switch part.Type.Normalized() {
	case work.WorkContentPartTypeText:
		return workerconfig.ModelOperationContentTypeText
	case work.WorkContentPartTypeImage:
		return workerconfig.ModelOperationContentTypeImage
	case work.WorkContentPartTypeAudio:
		return workerconfig.ModelOperationContentTypeAudio
	case work.WorkContentPartTypeJSON:
		return workerconfig.ModelOperationContentTypeJSON
	case work.WorkContentPartTypeBinary:
		return workerconfig.ModelOperationContentTypeBinary
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

func cloneResolvedBindingContent(parts []work.WorkContentPart) []work.WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]work.WorkContentPart, len(parts))
	copy(cloned, parts)
	return cloned
}

func modelOperationHasOnlyTextOutputs(operation workerconfig.ModelOperation) bool {
	if len(operation.Outputs) == 0 {
		return true
	}
	for _, output := range operation.Outputs {
		if len(output.ContentTypes) == 0 {
			return false
		}
		for _, contentType := range output.ContentTypes {
			if strings.TrimSpace(contentType) != workerconfig.ModelOperationContentTypeText {
				return false
			}
		}
	}
	return true
}

func orderWorkContentByOperationOutputs(parts []work.WorkContentPart, operation workerconfig.ModelOperation) []work.WorkContentPart {
	if len(parts) == 0 || len(operation.Outputs) == 0 {
		return parts
	}

	ordered := make([]work.WorkContentPart, 0, len(parts))
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

func slotContentTypes(slot workerconfig.ModelOperationSlot) []string {
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
