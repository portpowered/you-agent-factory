// Package invocationworktype implements Factory Definition invocation work-type
// policy owned by nested invocation_policy.
package invocationworktype

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func DefaultWorkType(cfg *factorydefinitions.FactoryConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("factory config is required")
	}

	var found string
	for _, workType := range cfg.WorkTypes {
		for _, behavior := range workType.HandlingBehavior {
			if factorydefinitions.StrictPublicWorkTypeHandlingBehavior(behavior) != factorydefinitions.WorkTypeHandlingBehaviorDefault {
				continue
			}
			if found != "" {
				return "", fmt.Errorf("expected at most one work type with handlingBehavior DEFAULT, found multiple")
			}
			found = workType.Name
		}
	}
	if found == "" {
		return "", fmt.Errorf("expected exactly one work type with handlingBehavior DEFAULT for simplified prompt runs")
	}
	return found, nil
}
