package wire

import (
	"fmt"
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	invocationpolicywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/wire"
)

// InvocationPolicyPorts is the process-composition bridge for the private
// invocation_policy owner. It is accepted only while the execution owners
// finish consuming the detached ResolveInvocationDefinition projection; the
// Definitions root itself is still the only domain service returned by NewService.
type InvocationPolicyPorts struct {
	DecisionEnvelope        factoryeffect.DecisionEnvelopeService
	InvocationInterpolation factoryeffect.InvocationInterpolationService
	InvocationOutput        factoryeffect.InvocationOutputShapingService
	InvocationWorkType      factoryeffect.InvocationWorkTypeService
	QuorumPolicy            factoryeffect.QuorumPolicyService
	WorkPropagation         factoryeffect.WorkPropagationPolicyService
	WorkstationExecution    factoryeffect.WorkstationExecutionPolicyService
	TTSObservability        factoryeffect.TTSObservabilityService
}

// InvocationPolicyPortsFromNestedOwner constructs the process-composition
// bridge from the nested invocation_policy subservice.
func InvocationPolicyPortsFromNestedOwner() (InvocationPolicyPorts, error) {
	service, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		return InvocationPolicyPorts{}, err
	}
	return InvocationPolicyPorts{
		DecisionEnvelope:        service.DecisionEnvelope(),
		InvocationInterpolation: service.InvocationInterpolation(),
		InvocationOutput:        service.InvocationOutput(),
		InvocationWorkType:      service.InvocationWorkType(),
		QuorumPolicy:            service.QuorumPolicy(),
		WorkPropagation:         service.WorkPropagation(),
		WorkstationExecution:    service.WorkstationExecution(),
		TTSObservability:        service.TTSObservability(),
	}, nil
}

func invocationPolicyService(ports InvocationPolicyPorts) (invocationpolicyservice.Service, error) {
	if ports.DecisionEnvelope == nil ||
		ports.InvocationInterpolation == nil ||
		ports.InvocationOutput == nil ||
		ports.InvocationWorkType == nil ||
		ports.QuorumPolicy == nil ||
		ports.WorkPropagation == nil ||
		ports.WorkstationExecution == nil ||
		ports.TTSObservability == nil {
		service, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
		if err != nil {
			return nil, fmt.Errorf("construct Factory Definitions invocation policy: %w", err)
		}
		return service, nil
	}
	return invocationPolicyPortsService{InvocationPolicyPorts: ports}, nil
}

type invocationPolicyPortsService struct {
	InvocationPolicyPorts
}

func (s invocationPolicyPortsService) DecisionEnvelope() factoryeffect.DecisionEnvelopeService {
	return s.InvocationPolicyPorts.DecisionEnvelope
}

func (s invocationPolicyPortsService) InvocationInterpolation() factoryeffect.InvocationInterpolationService {
	return s.InvocationPolicyPorts.InvocationInterpolation
}

func (s invocationPolicyPortsService) InvocationOutput() factoryeffect.InvocationOutputShapingService {
	return s.InvocationPolicyPorts.InvocationOutput
}

func (s invocationPolicyPortsService) InvocationWorkType() factoryeffect.InvocationWorkTypeService {
	return s.InvocationPolicyPorts.InvocationWorkType
}

func (s invocationPolicyPortsService) QuorumPolicy() factoryeffect.QuorumPolicyService {
	return s.InvocationPolicyPorts.QuorumPolicy
}

func (s invocationPolicyPortsService) WorkPropagation() factoryeffect.WorkPropagationPolicyService {
	return s.InvocationPolicyPorts.WorkPropagation
}

func (s invocationPolicyPortsService) WorkstationExecution() factoryeffect.WorkstationExecutionPolicyService {
	return s.InvocationPolicyPorts.WorkstationExecution
}

func (s invocationPolicyPortsService) TTSObservability() factoryeffect.TTSObservabilityService {
	return s.InvocationPolicyPorts.TTSObservability
}
