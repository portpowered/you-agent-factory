package providercatalog

import (
	"fmt"
	"sort"
)

func validateCatalogSemantics(providers []any) error {
	manifests := make([]map[string]any, len(providers))
	ids := make(map[string]struct{}, len(providers))
	for index, value := range providers {
		manifest := value.(map[string]any)
		manifests[index] = manifest
		id := manifest["id"].(string)
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
	}
	return validateDeprecations(manifests, ids)
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
