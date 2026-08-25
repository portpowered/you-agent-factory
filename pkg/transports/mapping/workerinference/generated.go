// Package workerinference maps generated inference inputs to Factory
// Definitions-owned operation binding contracts.
package workerinference

import (
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

// OperationBindingsFromGenerated maps public operation bindings onto the
// detached inference binding contract.
func OperationBindingsFromGenerated(values *[]factoryapi.WorkstationOperationBinding) []factorydefinitions.ModelOperationBinding {
	if values == nil || len(*values) == 0 {
		return nil
	}
	bindings := make([]factorydefinitions.ModelOperationBinding, 0, len(*values))
	for _, binding := range *values {
		current := factorydefinitions.ModelOperationBinding{
			Slot:           strings.TrimSpace(binding.Slot),
			Config:         contentmapping.PartsFromGenerated(binding.Config),
			DefaultContent: contentmapping.PartsFromGenerated(binding.DefaultContent),
		}
		if binding.Selector != nil {
			current.Selector = &factorydefinitions.ModelOperationBindingSelector{
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

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(string(*value))
}
