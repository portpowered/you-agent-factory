// Package invocationworktype implements Factory Definition policy for
// selecting the Work Type used by simplified Factory invocation.
package invocationworktype

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type Service struct{}

var _ factorydefinitions.InvocationWorkTypeService = Service{}

func NewService() factorydefinitions.InvocationWorkTypeService {
	return Service{}
}

func (Service) DefaultWorkType(cfg *factorydefinitions.FactoryConfig) (string, error) {
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
