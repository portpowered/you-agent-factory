package factorydefinitions

import (
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// DecisionEnvelope is the canonical JSON response shape for reviewer/checker
// workers in packaged @you/goal flows.
type DecisionEnvelope struct {
	Decision           string                 `json:"decision"`
	Feedback           string                 `json:"feedback"`
	Output             string                 `json:"output,omitempty"`
	RecordedOutputWork []work.FactoryWorkItem `json:"recorded_output_work,omitempty"`
}

const (
	MalformedEnvelopeFailureOutcome = workerexecution.OutcomeFailed

	DecisionAccepted = string(workerexecution.OutcomeAccepted)
	DecisionContinue = string(workerexecution.OutcomeContinue)
	DecisionRejected = string(workerexecution.OutcomeRejected)
	DecisionFailed   = string(workerexecution.OutcomeFailed)

	DecisionEnvelopeOutcomeFormat = contracts.WorkstationOutcomeFormatDecisionEnvelope

	GoalRoutingDecisionAccepted     = "accepted"
	GoalRoutingDecisionNeedsChanges = "needs_changes"
	GoalRoutingDecisionTestsFailed  = "tests_failed"
	GoalRoutingDecisionNeedsHuman   = "needs_human"
	GoalRoutingDecisionBlocked      = "blocked"
	GoalRoutingDecisionInterrupted  = "interrupted"
	GoalRoutingDecisionFailed       = "failed"
)

// DecisionEnvelopeService interprets packaged Goal decision-envelope output
// without exposing the packaged Goal implementation to peer services.
type DecisionEnvelopeService interface {
	UsesDecisionEnvelopeOutcome(*FactoryWorkstationConfig) bool
	UsesGoalRoutingDecisionEnvelope(*FactoryWorkstationConfig) bool
	WorkResultFromDecisionEnvelopeJSONOrFailed(
		string,
		string,
		string,
	) workerexecution.WorkResult
	WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
		string,
		string,
		string,
	) workerexecution.WorkResult
}
