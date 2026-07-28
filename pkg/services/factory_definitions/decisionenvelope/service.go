// Package decisionenvelope is a transitional re-export surface for packaged Goal
// decision-envelope interpretation. Implementation is owned by nested
// internal/services/invocation_policy/decisionenvelope; deletion is deferred to DEL packets.
package decisionenvelope

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicydecisionenvelope "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/decisionenvelope"
)

type DecisionEnvelope = factorydefinitions.DecisionEnvelope
type FactoryWorkstationConfig = factorydefinitions.FactoryWorkstationConfig

const MalformedEnvelopeFailureOutcome = factorydefinitions.MalformedEnvelopeFailureOutcome

const (
	DecisionAccepted = factorydefinitions.DecisionAccepted
	DecisionContinue = factorydefinitions.DecisionContinue
	DecisionRejected = factorydefinitions.DecisionRejected
	DecisionFailed   = factorydefinitions.DecisionFailed
)

const DecisionEnvelopeOutcomeFormat = factorydefinitions.DecisionEnvelopeOutcomeFormat

const (
	GoalRoutingDecisionAccepted     = factorydefinitions.GoalRoutingDecisionAccepted
	GoalRoutingDecisionNeedsChanges = factorydefinitions.GoalRoutingDecisionNeedsChanges
	GoalRoutingDecisionTestsFailed  = factorydefinitions.GoalRoutingDecisionTestsFailed
	GoalRoutingDecisionNeedsHuman   = factorydefinitions.GoalRoutingDecisionNeedsHuman
	GoalRoutingDecisionBlocked      = factorydefinitions.GoalRoutingDecisionBlocked
	GoalRoutingDecisionInterrupted  = factorydefinitions.GoalRoutingDecisionInterrupted
	GoalRoutingDecisionFailed       = factorydefinitions.GoalRoutingDecisionFailed
)

func NewService() factorydefinitions.DecisionEnvelopeService {
	return invocationpolicydecisionenvelope.NewService()
}

var (
	UsesDecisionEnvelopeOutcome                           = invocationpolicydecisionenvelope.UsesDecisionEnvelopeOutcome
	UsesGoalRoutingDecisionEnvelope                       = invocationpolicydecisionenvelope.UsesGoalRoutingDecisionEnvelope
	NormalizeGoalRoutingDecision                          = invocationpolicydecisionenvelope.NormalizeGoalRoutingDecision
	SupportedDecisions                                    = invocationpolicydecisionenvelope.SupportedDecisions
	OutcomeFromDecision                                   = invocationpolicydecisionenvelope.OutcomeFromDecision
	ParseDecisionEnvelopeJSON                             = invocationpolicydecisionenvelope.ParseDecisionEnvelopeJSON
	WorkResultFromDecisionEnvelope                        = invocationpolicydecisionenvelope.WorkResultFromDecisionEnvelope
	WorkResultFromDecisionEnvelopeJSON                    = invocationpolicydecisionenvelope.WorkResultFromDecisionEnvelopeJSON
	FailedWorkResultFromDecisionEnvelopeError             = invocationpolicydecisionenvelope.FailedWorkResultFromDecisionEnvelopeError
	WorkResultFromDecisionEnvelopeJSONOrFailed            = invocationpolicydecisionenvelope.WorkResultFromDecisionEnvelopeJSONOrFailed
	WorkResultFromGoalRoutingDecisionEnvelope             = invocationpolicydecisionenvelope.WorkResultFromGoalRoutingDecisionEnvelope
	WorkResultFromGoalRoutingDecisionEnvelopeJSON         = invocationpolicydecisionenvelope.WorkResultFromGoalRoutingDecisionEnvelopeJSON
	WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed = invocationpolicydecisionenvelope.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed
)
