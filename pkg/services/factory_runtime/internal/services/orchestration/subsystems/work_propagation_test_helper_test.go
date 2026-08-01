package subsystems

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

func testWorkPropagationPolicy() factorydefinitionswire.WorkPropagationPolicyService {
	return factorydefinitionswire.WorkPropagationPolicyFunc(func(
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
