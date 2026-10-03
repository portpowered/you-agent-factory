package factorydefinitions

import (
	"bytes"
	"encoding/json"

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

// UnmarshalJSON accepts feedback and output as either a JSON string or any other
// JSON value; non-string values are kept as compact JSON text so a structured
// worker response is not discarded over output shape.
func (e *DecisionEnvelope) UnmarshalJSON(data []byte) error {
	var aux struct {
		Decision           string                 `json:"decision"`
		Feedback           json.RawMessage        `json:"feedback"`
		Output             json.RawMessage        `json:"output"`
		RecordedOutputWork []work.FactoryWorkItem `json:"recorded_output_work"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	feedback, err := envelopeText(aux.Feedback)
	if err != nil {
		return err
	}
	output, err := envelopeText(aux.Output)
	if err != nil {
		return err
	}
	*e = DecisionEnvelope{
		Decision:           aux.Decision,
		Feedback:           feedback,
		Output:             output,
		RecordedOutputWork: aux.RecordedOutputWork,
	}
	return nil
}

func envelopeText(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return "", err
		}
		return text, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return "", err
	}
	return compact.String(), nil
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
