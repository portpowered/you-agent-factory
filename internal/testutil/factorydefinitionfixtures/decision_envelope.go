package factorydefinitionfixtures

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// DecisionEnvelopeService is a programmable Factory Definitions root fake.
// Consumer tests can inject only the envelope behavior their scenario reaches
// without constructing the packaged Goal implementation.
type DecisionEnvelopeService struct {
	UsesEnvelope    func(*factorydefinitions.FactoryWorkstationConfig) bool
	UsesGoalRouting func(*factorydefinitions.FactoryWorkstationConfig) bool
	Result          func(string, string, string) workerexecution.WorkResult
	GoalResult      func(string, string, string) workerexecution.WorkResult
}

func (s DecisionEnvelopeService) UsesDecisionEnvelopeOutcome(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) bool {
	return s.UsesEnvelope != nil && s.UsesEnvelope(workstation)
}

func (s DecisionEnvelopeService) UsesGoalRoutingDecisionEnvelope(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) bool {
	return s.UsesGoalRouting != nil && s.UsesGoalRouting(workstation)
}

func (s DecisionEnvelopeService) WorkResultFromDecisionEnvelopeJSONOrFailed(
	dispatchID string,
	transitionID string,
	raw string,
) workerexecution.WorkResult {
	if s.Result == nil {
		return workerexecution.WorkResult{}
	}
	return s.Result(dispatchID, transitionID, raw)
}

func (s DecisionEnvelopeService) WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
	dispatchID string,
	transitionID string,
	raw string,
) workerexecution.WorkResult {
	if s.GoalResult == nil {
		return workerexecution.WorkResult{}
	}
	return s.GoalResult(dispatchID, transitionID, raw)
}
