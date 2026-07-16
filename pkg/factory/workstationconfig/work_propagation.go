package workstationconfig

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// WorkPropagationMode returns the authored workstation payload propagation mode.
// Omitted policy defaults to output-as-payload behavior.
func WorkPropagationMode(workstation *interfaces.FactoryWorkstationConfig) interfaces.WorkPropagationMode {
	if workstation == nil || workstation.WorkPropagation == nil {
		return interfaces.WorkPropagationModeOutputAsPayload
	}
	mode := strings.TrimSpace(string(workstation.WorkPropagation.Mode))
	if mode == "" {
		return interfaces.WorkPropagationModeOutputAsPayload
	}
	return interfaces.WorkPropagationMode(mode)
}
