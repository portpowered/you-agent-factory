package factory

import interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// RuntimeModeOrDefault normalizes an omitted process mode to batch execution.
func RuntimeModeOrDefault(mode interfaces.RuntimeMode) interfaces.RuntimeMode {
	if mode == "" {
		return interfaces.RuntimeModeBatch
	}
	return mode
}
