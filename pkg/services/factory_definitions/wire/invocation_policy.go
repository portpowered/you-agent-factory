package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	invocationpolicywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/wire"
)

// InvocationPolicyPorts are the published Definitions root policy contracts
// constructed from the nested invocation_policy owner.
type InvocationPolicyPorts struct {
	DecisionEnvelope        factorydefinitions.DecisionEnvelopeService
	InvocationInterpolation factorydefinitions.InvocationInterpolationService
	InvocationOutput        factorydefinitions.InvocationOutputShapingService
	InvocationWorkType      factorydefinitions.InvocationWorkTypeService
	QuorumPolicy            factorydefinitions.QuorumPolicyService
	WorkPropagation         factorydefinitions.WorkPropagationPolicyService
	WorkstationExecution    factorydefinitions.WorkstationExecutionPolicyService
	TTSObservability        factorydefinitions.TTSObservabilityService
}

// InvocationPolicyPortsFromNestedOwner constructs published root policy contracts
// from the nested invocation_policy subservice.
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
