// Package workerinference maps generated inference inputs to worker-owned contracts.
package workerinference

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

// OperationBindingsFromGenerated maps authored OpenAPI operation bindings onto
// the worker-owned inference binding contract.
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

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(string(*value))
}
