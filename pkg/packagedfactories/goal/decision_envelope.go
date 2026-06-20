package goal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// MalformedEnvelopeFailureOutcome is the WorkOutcome used when reviewer/checker output
// is not valid JSON or contains an unsupported decision value. Both failure cases use
// the runtime FAILED path so routing does not silently coerce or guess a decision.
const MalformedEnvelopeFailureOutcome = interfaces.OutcomeFailed

// DecisionEnvelope is the canonical JSON response shape for reviewer/checker workers
// in packaged @you/goal flows.
//
// Malformed-envelope failure behavior:
//   - Invalid JSON, empty output, missing decision, and unknown decision values all map
//     to WorkResult.Outcome FAILED (MalformedEnvelopeFailureOutcome).
//   - The failure WorkResult carries actionable inspection text in Error and, when the
//     JSON shape parsed, any reviewer-provided Feedback from the envelope.
//   - Callers that need a WorkResult for every reviewer output should use
//     WorkResultFromDecisionEnvelopeJSONOrFailed; callers that prefer explicit Go errors
//     can use WorkResultFromDecisionEnvelopeJSON and FailedWorkResultFromDecisionEnvelopeError.
type DecisionEnvelope struct {
	Decision           string                     `json:"decision"`
	Feedback           string                     `json:"feedback"`
	Output             string                     `json:"output,omitempty"`
	RecordedOutputWork []interfaces.FactoryWorkItem `json:"recorded_output_work,omitempty"`
}

// Accepted reviewer/checker decision values map one-to-one onto WorkOutcome.
const (
	DecisionAccepted = string(interfaces.OutcomeAccepted)
	DecisionContinue = string(interfaces.OutcomeContinue)
	DecisionRejected = string(interfaces.OutcomeRejected)
	DecisionFailed   = string(interfaces.OutcomeFailed)
)

// DecisionEnvelopeOutcomeFormat is the workstation outcomeFormat value that routes
// agent output through the reviewer/checker JSON envelope contract.
const DecisionEnvelopeOutcomeFormat = "decision-envelope"

// UsesDecisionEnvelopeOutcome reports whether the workstation routes agent output
// through the reviewer/checker decision envelope instead of stop-token markers.
func UsesDecisionEnvelopeOutcome(workstation *interfaces.FactoryWorkstationConfig) bool {
	if workstation == nil {
		return false
	}
	return strings.TrimSpace(workstation.OutcomeFormat) == DecisionEnvelopeOutcomeFormat
}

// SupportedDecisions returns the accepted decision vocabulary in stable order.
func SupportedDecisions() []string {
	return []string{
		DecisionAccepted,
		DecisionContinue,
		DecisionRejected,
		DecisionFailed,
	}
}

// OutcomeFromDecision maps an envelope decision value to a WorkOutcome.
func OutcomeFromDecision(decision string) (interfaces.WorkOutcome, error) {
	trimmed := strings.TrimSpace(decision)
	if trimmed == "" {
		return "", fmt.Errorf("decision envelope: decision is required")
	}
	switch interfaces.WorkOutcome(trimmed) {
	case interfaces.OutcomeAccepted,
		interfaces.OutcomeContinue,
		interfaces.OutcomeRejected,
		interfaces.OutcomeFailed:
		return interfaces.WorkOutcome(trimmed), nil
	default:
		return "", fmt.Errorf("decision envelope: unknown decision %q", trimmed)
	}
}

// ParseDecisionEnvelopeJSON unmarshals reviewer/checker output as a decision envelope.
func ParseDecisionEnvelopeJSON(raw string) (DecisionEnvelope, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DecisionEnvelope{}, fmt.Errorf("decision envelope: output is empty")
	}
	var envelope DecisionEnvelope
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return DecisionEnvelope{}, fmt.Errorf("decision envelope: invalid JSON: %w", err)
	}
	return envelope, nil
}

// WorkResultFromDecisionEnvelope maps a parsed envelope onto the existing WorkResult contract.
func WorkResultFromDecisionEnvelope(dispatchID, transitionID string, envelope DecisionEnvelope) (interfaces.WorkResult, error) {
	outcome, err := OutcomeFromDecision(envelope.Decision)
	if err != nil {
		return interfaces.WorkResult{}, err
	}
	return interfaces.WorkResult{
		DispatchID:         dispatchID,
		TransitionID:       transitionID,
		Outcome:            outcome,
		Feedback:           envelope.Feedback,
		Output:             envelope.Output,
		RecordedOutputWork: envelope.RecordedOutputWork,
	}, nil
}

// WorkResultFromDecisionEnvelopeJSON parses reviewer/checker output and maps it to WorkResult.
func WorkResultFromDecisionEnvelopeJSON(dispatchID, transitionID, raw string) (interfaces.WorkResult, error) {
	envelope, err := ParseDecisionEnvelopeJSON(raw)
	if err != nil {
		return interfaces.WorkResult{}, err
	}
	return WorkResultFromDecisionEnvelope(dispatchID, transitionID, envelope)
}

// FailedWorkResultFromDecisionEnvelopeError maps malformed-envelope failures onto WorkResult
// using MalformedEnvelopeFailureOutcome. When partial carries parsed envelope fields, any
// reviewer feedback is preserved for inspection without treating the decision as understood.
func FailedWorkResultFromDecisionEnvelopeError(
	dispatchID, transitionID string,
	err error,
	partial DecisionEnvelope,
) interfaces.WorkResult {
	message := "reviewer decision envelope invalid"
	if err != nil {
		message += ": " + err.Error()
	}
	return interfaces.WorkResult{
		DispatchID:   dispatchID,
		TransitionID: transitionID,
		Outcome:      MalformedEnvelopeFailureOutcome,
		Error:        message,
		Feedback:     partial.Feedback,
	}
}

// WorkResultFromDecisionEnvelopeJSONOrFailed parses reviewer/checker output and always
// returns a WorkResult. Invalid JSON and unknown decisions use MalformedEnvelopeFailureOutcome.
func WorkResultFromDecisionEnvelopeJSONOrFailed(dispatchID, transitionID, raw string) interfaces.WorkResult {
	envelope, err := ParseDecisionEnvelopeJSON(raw)
	if err != nil {
		return FailedWorkResultFromDecisionEnvelopeError(dispatchID, transitionID, err, DecisionEnvelope{})
	}
	result, err := WorkResultFromDecisionEnvelope(dispatchID, transitionID, envelope)
	if err != nil {
		return FailedWorkResultFromDecisionEnvelopeError(dispatchID, transitionID, err, envelope)
	}
	return result
}
