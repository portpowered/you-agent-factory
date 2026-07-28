package service

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicydecisionenvelope "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/decisionenvelope"
	invocationpolicyinterpolation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationinterpolation"
	invocationpolicyoutput "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationoutput"
	invocationpolicyquorum "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/quorumpolicy"
	invocationpolicytts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/ttsobservability"
	invocationpolicyworkpropagation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/workpropagation"
	invocationpolicyworkstationexecution "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/workstationexecution"
	invocationpolicyworktype "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationworktype"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
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
		invocationInterpolation: invocationpolicyinterpolation.NewService(),
		invocationOutput:        invocationpolicyoutput.NewService(),
		invocationWorkType:      invocationpolicyworktype.NewService(),
		quorumPolicy:            invocationpolicyquorum.NewService(),
		workPropagation:         invocationpolicyworkpropagation.NewService(),
		workstationExecution:    invocationpolicyworkstationexecution.NewService(),
		ttsObservability:        invocationpolicytts.NewService(),
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
