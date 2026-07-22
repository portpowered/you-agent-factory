package goal

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/decisionenvelope"
)

type DecisionEnvelope = factorydefinitions.DecisionEnvelope

const (
	MalformedEnvelopeFailureOutcome = factorydefinitions.MalformedEnvelopeFailureOutcome
	DecisionAccepted                = factorydefinitions.DecisionAccepted
	DecisionContinue                = factorydefinitions.DecisionContinue
	DecisionRejected                = factorydefinitions.DecisionRejected
	DecisionFailed                  = factorydefinitions.DecisionFailed
	DecisionEnvelopeOutcomeFormat   = factorydefinitions.DecisionEnvelopeOutcomeFormat
	GoalRoutingDecisionAccepted     = factorydefinitions.GoalRoutingDecisionAccepted
	GoalRoutingDecisionNeedsChanges = factorydefinitions.GoalRoutingDecisionNeedsChanges
	GoalRoutingDecisionTestsFailed  = factorydefinitions.GoalRoutingDecisionTestsFailed
	GoalRoutingDecisionNeedsHuman   = factorydefinitions.GoalRoutingDecisionNeedsHuman
	GoalRoutingDecisionBlocked      = factorydefinitions.GoalRoutingDecisionBlocked
	GoalRoutingDecisionInterrupted  = factorydefinitions.GoalRoutingDecisionInterrupted
	GoalRoutingDecisionFailed       = factorydefinitions.GoalRoutingDecisionFailed
)

var (
	NewDecisionEnvelopeService                            = decisionenvelope.NewService
	UsesDecisionEnvelopeOutcome                           = decisionenvelope.UsesDecisionEnvelopeOutcome
	UsesGoalRoutingDecisionEnvelope                       = decisionenvelope.UsesGoalRoutingDecisionEnvelope
	NormalizeGoalRoutingDecision                          = decisionenvelope.NormalizeGoalRoutingDecision
	SupportedDecisions                                    = decisionenvelope.SupportedDecisions
	OutcomeFromDecision                                   = decisionenvelope.OutcomeFromDecision
	ParseDecisionEnvelopeJSON                             = decisionenvelope.ParseDecisionEnvelopeJSON
	WorkResultFromDecisionEnvelope                        = decisionenvelope.WorkResultFromDecisionEnvelope
	WorkResultFromDecisionEnvelopeJSON                    = decisionenvelope.WorkResultFromDecisionEnvelopeJSON
	FailedWorkResultFromDecisionEnvelopeError             = decisionenvelope.FailedWorkResultFromDecisionEnvelopeError
	WorkResultFromDecisionEnvelopeJSONOrFailed            = decisionenvelope.WorkResultFromDecisionEnvelopeJSONOrFailed
	WorkResultFromGoalRoutingDecisionEnvelope             = decisionenvelope.WorkResultFromGoalRoutingDecisionEnvelope
	WorkResultFromGoalRoutingDecisionEnvelopeJSON         = decisionenvelope.WorkResultFromGoalRoutingDecisionEnvelopeJSON
	WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed = decisionenvelope.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed
)
