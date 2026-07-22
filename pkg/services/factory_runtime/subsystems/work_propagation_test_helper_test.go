package subsystems

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func testWorkPropagationPolicy() factorydefinitions.WorkPropagationPolicyService {
	return factorydefinitions.WorkPropagationPolicyFunc(func(
		workstation *factorydefinitions.FactoryWorkstationConfig,
	) factorydefinitions.WorkPropagationMode {
		if workstation != nil &&
			workstation.WorkPropagation != nil &&
			workstation.WorkPropagation.Mode != "" {
			return workstation.WorkPropagation.Mode
		}
		return factorydefinitions.WorkPropagationModeOutputAsPayload
	})
}
