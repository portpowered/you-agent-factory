package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	decisionenvelope "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/decisionenvelope"
	invocationinterpolation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationinterpolation"
	invocationoutput "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationoutput"
	invocationworktype "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationworktype"
	quorumpolicy "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/quorumpolicy"
	ttsobservability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/ttsobservability"
	workpropagation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/workpropagation"
	workstationexecution "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/workstationexecution"
)

// NewDecisionEnvelopeService constructs the focused Definitions policy owner.
func NewDecisionEnvelopeService() factorydefinitions.DecisionEnvelopeService {
	return decisionenvelope.NewService()
}

// NewInvocationInterpolationService constructs the focused Definitions policy owner.
func NewInvocationInterpolationService() factorydefinitions.InvocationInterpolationService {
	return invocationinterpolation.NewService()
}

// NewInvocationOutputShapingService constructs the focused Definitions policy owner.
func NewInvocationOutputShapingService() factorydefinitions.InvocationOutputShapingService {
	return invocationoutput.NewService()
}

// NewInvocationWorkTypeService constructs the focused Definitions policy owner.
func NewInvocationWorkTypeService() factorydefinitions.InvocationWorkTypeService {
	return invocationworktype.NewService()
}

// NewQuorumPolicyService constructs the focused Definitions policy owner.
func NewQuorumPolicyService() factorydefinitions.QuorumPolicyService {
	return quorumpolicy.NewService()
}

// NewWorkPropagationPolicyService constructs the focused Definitions policy owner.
func NewWorkPropagationPolicyService() factorydefinitions.WorkPropagationPolicyService {
	return workpropagation.NewService()
}

// NewWorkstationExecutionPolicyService constructs the focused Definitions policy owner.
func NewWorkstationExecutionPolicyService() factorydefinitions.WorkstationExecutionPolicyService {
	return workstationexecution.NewService()
}

// NewTTSObservabilityService constructs the focused Definitions policy owner.
func NewTTSObservabilityService() factorydefinitions.TTSObservabilityService {
	return ttsobservability.NewService()
}
