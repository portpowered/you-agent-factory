package service

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicydecisionenvelope "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/decisionenvelope"
	factoryinvocationinterpolation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/invocationinterpolation"
	factoryinvocationoutput "github.com/portpowered/infinite-you/pkg/services/factory_definitions/invocationoutput"
	factoryinvocationworktype "github.com/portpowered/infinite-you/pkg/services/factory_definitions/invocationworktype"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	factoryquorumpolicy "github.com/portpowered/infinite-you/pkg/services/factory_definitions/quorumpolicy"
	factoryttsobservability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/ttsobservability"
	factoryworkpropagation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workpropagation"
	factoryworkstationexecution "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workstationexecution"
)

// Service is the private nested invocation_policy implementation behind the
// published Definitions root policy contracts.
type Service struct {
	decisionEnvelope       factorydefinitions.DecisionEnvelopeService
	invocationInterpolation factorydefinitions.InvocationInterpolationService
	invocationOutput       factorydefinitions.InvocationOutputShapingService
	invocationWorkType     factorydefinitions.InvocationWorkTypeService
	quorumPolicy           factorydefinitions.QuorumPolicyService
	workPropagation        factorydefinitions.WorkPropagationPolicyService
	workstationExecution   factorydefinitions.WorkstationExecutionPolicyService
	ttsObservability       factorydefinitions.TTSObservabilityService
}

var _ invocationpolicyservice.Service = (*Service)(nil)

// New constructs the invocation_policy implementation from the current
// transitional policy packages. Later fold stories relocate those
// implementations under this owner without changing the published root surface.
func New() *Service {
	return &Service{
		decisionEnvelope:        invocationpolicydecisionenvelope.NewService(),
		invocationInterpolation: factoryinvocationinterpolation.NewService(),
		invocationOutput:        factoryinvocationoutput.NewService(),
		invocationWorkType:      factoryinvocationworktype.NewService(),
		quorumPolicy:            factoryquorumpolicy.NewService(),
		workPropagation:         factoryworkpropagation.NewService(),
		workstationExecution:    factoryworkstationexecution.NewService(),
		ttsObservability:        factoryttsobservability.NewService(),
	}
}

func (s *Service) DecisionEnvelope() factorydefinitions.DecisionEnvelopeService {
	return s.decisionEnvelope
}

func (s *Service) InvocationInterpolation() factorydefinitions.InvocationInterpolationService {
	return s.invocationInterpolation
}

func (s *Service) InvocationOutput() factorydefinitions.InvocationOutputShapingService {
	return s.invocationOutput
}

func (s *Service) InvocationWorkType() factorydefinitions.InvocationWorkTypeService {
	return s.invocationWorkType
}

func (s *Service) QuorumPolicy() factorydefinitions.QuorumPolicyService {
	return s.quorumPolicy
}

func (s *Service) WorkPropagation() factorydefinitions.WorkPropagationPolicyService {
	return s.workPropagation
}

func (s *Service) WorkstationExecution() factorydefinitions.WorkstationExecutionPolicyService {
	return s.workstationExecution
}

func (s *Service) TTSObservability() factorydefinitions.TTSObservabilityService {
	return s.ttsObservability
}
