package service

import (
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	invocationpolicydecisionenvelope "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/decisionenvelope"
	invocationpolicyinterpolation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/invocationinterpolation"
	invocationpolicyoutput "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/invocationoutput"
	invocationpolicyworktype "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/invocationworktype"
	invocationpolicyquorum "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/quorumpolicy"
	invocationpolicytts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/ttsobservability"
	invocationpolicyworkpropagation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/workpropagation"
	invocationpolicyworkstationexecution "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/workstationexecution"
)

// Service is the private nested invocation_policy implementation behind the
// published Definitions root policy contracts.
type Service struct {
	decisionEnvelope        factoryeffects.DecisionEnvelopeService
	invocationInterpolation factoryeffects.InvocationInterpolationService
	invocationOutput        factoryeffects.InvocationOutputShapingService
	invocationWorkType      factoryeffects.InvocationWorkTypeService
	quorumPolicy            factoryeffects.QuorumPolicyService
	workPropagation         factoryeffects.WorkPropagationPolicyService
	workstationExecution    factoryeffects.WorkstationExecutionPolicyService
	ttsObservability        factoryeffects.TTSObservabilityService
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

func (s *Service) DecisionEnvelope() factoryeffects.DecisionEnvelopeService {
	return s.decisionEnvelope
}

func (s *Service) InvocationInterpolation() factoryeffects.InvocationInterpolationService {
	return s.invocationInterpolation
}

func (s *Service) InvocationOutput() factoryeffects.InvocationOutputShapingService {
	return s.invocationOutput
}

func (s *Service) InvocationWorkType() factoryeffects.InvocationWorkTypeService {
	return s.invocationWorkType
}

func (s *Service) QuorumPolicy() factoryeffects.QuorumPolicyService {
	return s.quorumPolicy
}

func (s *Service) WorkPropagation() factoryeffects.WorkPropagationPolicyService {
	return s.workPropagation
}

func (s *Service) WorkstationExecution() factoryeffects.WorkstationExecutionPolicyService {
	return s.workstationExecution
}

func (s *Service) TTSObservability() factoryeffects.TTSObservabilityService {
	return s.ttsObservability
}
