package wire

import (
	"fmt"
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	invocationpolicywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/wire"
)

// InvocationPolicy is the single process-composition projection for the
// private invocation_policy owner. The projection is constructed once by
// process Wire and shared with the Definitions root and execution adapters;
// individual policy services are not independently constructed or provided.
type InvocationPolicy struct {
	DecisionEnvelope        factoryeffect.DecisionEnvelopeService
	InvocationInterpolation factoryeffect.InvocationInterpolationService
	InvocationOutput        factoryeffect.InvocationOutputShapingService
	InvocationWorkType      factoryeffect.InvocationWorkTypeService
	QuorumPolicy            factoryeffect.QuorumPolicyService
	WorkPropagation         factoryeffect.WorkPropagationPolicyService
	WorkstationExecution    factoryeffect.WorkstationExecutionPolicyService
	TTSObservability        factoryeffect.TTSObservabilityService
}

// NewInvocationPolicy constructs the process-composition projection from the
// nested invocation_policy subservice.
func NewInvocationPolicy() (InvocationPolicy, error) {
	service, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		return InvocationPolicy{}, err
	}
	return InvocationPolicy{
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

func invocationPolicyService(ports InvocationPolicy) (invocationpolicyservice.Service, error) {
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
	return invocationPolicyPortsService{InvocationPolicy: ports}, nil
}

type invocationPolicyPortsService struct {
	InvocationPolicy
}

func (s invocationPolicyPortsService) DecisionEnvelope() factoryeffect.DecisionEnvelopeService {
	return s.InvocationPolicy.DecisionEnvelope
}

func (s invocationPolicyPortsService) InvocationInterpolation() factoryeffect.InvocationInterpolationService {
	return s.InvocationPolicy.InvocationInterpolation
}

func (s invocationPolicyPortsService) InvocationOutput() factoryeffect.InvocationOutputShapingService {
	return s.InvocationPolicy.InvocationOutput
}

func (s invocationPolicyPortsService) InvocationWorkType() factoryeffect.InvocationWorkTypeService {
	return s.InvocationPolicy.InvocationWorkType
}

func (s invocationPolicyPortsService) QuorumPolicy() factoryeffect.QuorumPolicyService {
	return s.InvocationPolicy.QuorumPolicy
}

func (s invocationPolicyPortsService) WorkPropagation() factoryeffect.WorkPropagationPolicyService {
	return s.InvocationPolicy.WorkPropagation
}

func (s invocationPolicyPortsService) WorkstationExecution() factoryeffect.WorkstationExecutionPolicyService {
	return s.InvocationPolicy.WorkstationExecution
}

func (s invocationPolicyPortsService) TTSObservability() factoryeffect.TTSObservabilityService {
	return s.InvocationPolicy.TTSObservability
}
