// Package workpropagation implements Factory Definition policy for propagating
// Work payloads through Workstation transitions, owned by nested invocation_policy.
package workpropagation

import (
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type Service struct{}

var _ factorydefinitions.WorkPropagationPolicyService = Service{}

func NewService() factorydefinitions.WorkPropagationPolicyService {
	return Service{}
}

func (Service) Mode(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) factorydefinitions.WorkPropagationMode {
	if workstation == nil || workstation.WorkPropagation == nil {
		return factorydefinitions.WorkPropagationModeOutputAsPayload
	}
	mode := strings.TrimSpace(string(workstation.WorkPropagation.Mode))
	if mode == "" {
		return factorydefinitions.WorkPropagationModeOutputAsPayload
	}
	return factorydefinitions.WorkPropagationMode(mode)
}
