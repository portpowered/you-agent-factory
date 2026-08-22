package providercatalog

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

func validateCatalogSemantics(providers []any) error {
	manifests := make([]map[string]any, len(providers))
	ids := make(map[string]struct{}, len(providers))
	for index, value := range providers {
		manifest := value.(map[string]any)
		manifests[index] = manifest
		id, ok := manifest["id"].(string)
		if !ok || strings.TrimSpace(id) == "" {
			return fmt.Errorf("provider identity at index %d: id must be a non-empty string", index)
		}
		if _, exists := ids[id]; exists {
			return fmt.Errorf("provider identity collision: duplicate canonical id %q", id)
		}
		ids[id] = struct{}{}
	}
	if err := validateAliases(manifests, ids); err != nil {
		return err
	}
	for _, manifest := range manifests {
		if err := validateCapabilities(manifest); err != nil {
			return err
		}
		if err := validateModels(manifest); err != nil {
			return err
		}
		if err := validateTools(manifest); err != nil {
			return err
		}
		if err := validateKnownLimits(manifest); err != nil {
			return err
		}
		if err := validateManifestCapabilityFacts(manifest); err != nil {
			return err
		}
	}
	return validateDeprecations(manifests, ids)
}

func validateModels(manifest map[string]any) error {
	value, exists := manifest["models"]
	if !exists {
		return nil
	}
	models, ok := value.([]any)
	if !ok {
		return fmt.Errorf("provider %q: models must be an array", manifest["id"])
	}
	providerID, _ := manifest["id"].(string)
	seen := make(map[string]struct{}, len(models))
	for index, raw := range models {
		model, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("provider %q: model %d is not an object", providerID, index)
		}
		id, ok := model["id"].(string)
		if !ok || strings.TrimSpace(id) == "" {
			return fmt.Errorf("provider %q: model %d has an empty id", providerID, index)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("provider %q: duplicate model id %q", providerID, id)
		}
		seen[id] = struct{}{}
		if err := validateEfforts(providerID, id, model["efforts"]); err != nil {
			return err
		}
		if err := validateModalities(providerID, id, model["modalities"]); err != nil {
			return err
		}
	}
	return nil
}

func validateEfforts(providerID, modelID string, value any) error {
	efforts, ok := value.([]any)
	if !ok {
		return fmt.Errorf("provider %q model %q: efforts must be an array", providerID, modelID)
	}
	seen := make(map[string]struct{}, len(efforts))
	for _, raw := range efforts {
		effort, ok := raw.(string)
		if !ok {
			return fmt.Errorf("provider %q model %q: effort must be a string", providerID, modelID)
		}
		if !isKnownEffort(effort) {
			return fmt.Errorf("provider %q model %q: unknown effort %q", providerID, modelID, effort)
		}
		if _, duplicate := seen[effort]; duplicate {
			return fmt.Errorf("provider %q model %q: duplicate effort %q", providerID, modelID, effort)
		}
		seen[effort] = struct{}{}
	}
	return nil
}

func isKnownEffort(value string) bool {
	switch value {
	case "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func validateModalities(providerID, modelID string, value any) error {
	modalities, ok := value.([]any)
	if !ok {
		return fmt.Errorf("provider %q model %q: modalities must be an array", providerID, modelID)
	}
	seen := make(map[string]struct{}, len(modalities))
	for index, raw := range modalities {
		modality, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("provider %q model %q: modality %d is not an object", providerID, modelID, index)
		}
		direction, _ := modality["direction"].(string)
		kind, _ := modality["modality"].(string)
		support, _ := modality["support"].(string)
		transport, _ := modality["transport"].(string)
		if direction != "input" && direction != "output" {
			return fmt.Errorf("provider %q model %q: unknown modality direction %q", providerID, modelID, direction)
		}
		if kind != "text" && kind != "image" && kind != "audio" && kind != "video" {
			return fmt.Errorf("provider %q model %q: unknown modality %q", providerID, modelID, kind)
		}
		if !isCapabilitySupport(support) {
			return fmt.Errorf("provider %q model %q: unknown modality support %q", providerID, modelID, support)
		}
		if !isModalityTransport(transport) {
			return fmt.Errorf("provider %q model %q: unknown modality transport %q", providerID, modelID, transport)
		}
		if err := validateRouteSupport(providerID+" model "+modelID, direction, kind, support, transport, modality["condition"]); err != nil {
			return err
		}
		if (support == "unsupported") != (transport == "none") && support != "unknown" {
			return fmt.Errorf("provider %q model %q modality %s/%s has inconsistent support and transport", providerID, modelID, direction, kind)
		}
		key := direction + "\x00" + kind
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("provider %q model %q: duplicate modality %s/%s", providerID, modelID, direction, kind)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateTools(manifest map[string]any) error {
	value, exists := manifest["tools"]
	if !exists {
		return nil
	}
	tools, ok := value.([]any)
	if !ok {
		return fmt.Errorf("provider %q: tools must be an array", manifest["id"])
	}
	providerID, _ := manifest["id"].(string)
	seen := make(map[string]struct{}, len(tools))
	for index, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("provider %q: tool %d is not an object", providerID, index)
		}
		name, _ := tool["name"].(string)
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("provider %q: tool %d has an empty name", providerID, index)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("provider %q: duplicate tool %q", providerID, name)
		}
		support, _ := tool["support"].(string)
		if !isCapabilitySupport(support) {
			return fmt.Errorf("provider %q tool %q: unknown support %q", providerID, name, support)
		}
		description, _ := tool["description"].(string)
		if strings.TrimSpace(description) == "" {
			return fmt.Errorf("provider %q tool %q: description is required", providerID, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func validateKnownLimits(manifest map[string]any) error {
	value, exists := manifest["knownLimits"]
	if !exists {
		return nil
	}
	limits, ok := value.([]any)
	if !ok {
		return fmt.Errorf("provider %q: knownLimits must be an array", manifest["id"])
	}
	providerID, _ := manifest["id"].(string)
	seen := make(map[string]struct{}, len(limits))
	for index, raw := range limits {
		limit, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("provider %q: known limit %d is not an object", providerID, index)
		}
		name, _ := limit["name"].(string)
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("provider %q: known limit %d has an empty name", providerID, index)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("provider %q: duplicate known limit %q", providerID, name)
		}
		seen[name] = struct{}{}
		kind, _ := limit["kind"].(string)
		maximum, hasMaximum := limit["maximum"]
		defaultValue, hasDefault := limit["default"]
		value, hasValue := limit["value"]
		valueText, valueIsString := value.(string)
		switch kind {
		case "maximum":
			if !hasMaximum || !isPositiveInteger(maximum) || hasDefault || hasValue {
				return fmt.Errorf("provider %q known limit %q: maximum record is incomplete", providerID, name)
			}
		case "default":
			if !hasDefault || !isPositiveInteger(defaultValue) || hasMaximum || hasValue {
				return fmt.Errorf("provider %q known limit %q: default record is incomplete", providerID, name)
			}
		case "behavior":
			if !hasValue || !valueIsString || strings.TrimSpace(valueText) == "" || hasMaximum || hasDefault {
				return fmt.Errorf("provider %q known limit %q: behavior record is incomplete", providerID, name)
			}
		default:
			return fmt.Errorf("provider %q known limit %q: unknown kind %q", providerID, name, kind)
		}
	}
	return nil
}

func isPositiveInteger(value any) bool {
	switch number := value.(type) {
	case int:
		return number > 0
	case int8:
		return number > 0
	case int16:
		return number > 0
	case int32:
		return number > 0
	case int64:
		return number > 0
	case uint:
		return number > 0
	case uint8:
		return number > 0
	case uint16:
		return number > 0
	case uint32:
		return number > 0
	case uint64:
		return number > 0
	case float32:
		return number > 0 && float32(math.Trunc(float64(number))) == number
	case float64:
		return number > 0 && math.Trunc(number) == number
	default:
		return false
	}
}

func validateAliases(manifests []map[string]any, ids map[string]struct{}) error {
	owners := make(map[string]string)
	for _, manifest := range manifests {
		id := manifest["id"].(string)
		for _, alias := range stringsFrom(manifest["aliases"]) {
			if alias == id {
				return fmt.Errorf("provider %q: alias %q duplicates its canonical id", id, alias)
			}
			if _, exists := ids[alias]; exists {
				return fmt.Errorf("provider %q: alias %q shadows a canonical provider id", id, alias)
			}
			if owner, exists := owners[alias]; exists {
				return fmt.Errorf("provider alias collision: %q is owned by both %q and %q", alias, owner, id)
			}
			owners[alias] = id
		}
	}
	return nil
}

func validateCapabilities(manifest map[string]any) error {
	id := manifest["id"].(string)
	nativeStreaming := boolField(manifest, "maximumResponseFidelityCapabilities", "nativeStreaming")
	sessionResume := boolField(manifest, "maximumExecutionCapabilities", "sessionResume")
	toolExecution := boolField(manifest, "maximumExecutionCapabilities", "toolExecution")
	streamDependents := []string{"messageDeltas", "toolOutputDeltas", "providerReconnect"}
	var impossible []string
	for _, field := range streamDependents {
		if boolField(manifest, "maximumResponseFidelityCapabilities", field) && !nativeStreaming {
			impossible = append(impossible, field+" requires nativeStreaming")
		}
	}
	if boolField(manifest, "maximumResponseFidelityCapabilities", "toolLifecycle") && !toolExecution {
		impossible = append(impossible, "toolLifecycle requires toolExecution")
	}
	if boolField(manifest, "maximumResponseFidelityCapabilities", "providerReconnect") && !sessionResume {
		impossible = append(impossible, "providerReconnect requires sessionResume")
	}
	if len(impossible) != 0 {
		sort.Strings(impossible)
		return fmt.Errorf("provider %q: impossible capability combination: %s", id, canonicalSet(impossible))
	}
	return nil
}

func validateDeprecations(manifests []map[string]any, ids map[string]struct{}) error {
	deprecated := make(map[string]bool, len(manifests))
	for _, manifest := range manifests {
		_, deprecated[manifest["id"].(string)] = manifest["deprecation"]
	}
	for _, manifest := range manifests {
		id := manifest["id"].(string)
		value, exists := manifest["deprecation"]
		if !exists {
			continue
		}
		deprecation := value.(map[string]any)
		replacement, hasReplacement := deprecation["replacementProviderId"].(string)
		if !hasReplacement {
			continue
		}
		if replacement == id {
			return fmt.Errorf("provider %q: replacementProviderId cannot identify the deprecated provider itself", id)
		}
		if _, exists := ids[replacement]; !exists {
			return fmt.Errorf("provider %q: replacementProviderId %q is not a canonical provider id", id, replacement)
		}
		if deprecated[replacement] {
			return fmt.Errorf("provider %q: replacementProviderId %q is also deprecated", id, replacement)
		}
	}
	return nil
}
