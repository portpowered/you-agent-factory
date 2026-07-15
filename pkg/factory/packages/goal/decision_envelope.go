package goal

import (
	"encoding/json"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// MalformedEnvelopeFailureOutcome is the WorkOutcome used when reviewer/checker output
// is not valid JSON or contains an unsupported decision value. Both failure cases use
// the runtime FAILED path so routing does not silently coerce or guess a decision.
const MalformedEnvelopeFailureOutcome = workerexecution.OutcomeFailed

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
	Decision           string                 `json:"decision"`
	Feedback           string                 `json:"feedback"`
	Output             string                 `json:"output,omitempty"`
	RecordedOutputWork []work.FactoryWorkItem `json:"recorded_output_work,omitempty"`
}

// Accepted reviewer/checker decision values map one-to-one onto WorkOutcome.
const (
	DecisionAccepted = string(workerexecution.OutcomeAccepted)
	DecisionContinue = string(workerexecution.OutcomeContinue)
	DecisionRejected = string(workerexecution.OutcomeRejected)
	DecisionFailed   = string(workerexecution.OutcomeFailed)
)

// DecisionEnvelopeOutcomeFormat is the workstation outcomeFormat value that routes
// agent output through the reviewer/checker JSON envelope contract.
const DecisionEnvelopeOutcomeFormat = interfaces.WorkstationOutcomeFormatDecisionEnvelope

// UsesDecisionEnvelopeOutcome reports whether the workstation routes agent output
// through the reviewer/checker decision envelope instead of stop-token markers.
func UsesDecisionEnvelopeOutcome(workstation *interfaces.FactoryWorkstationConfig) bool {
	if workstation == nil {
		return false
	}
	return strings.TrimSpace(workstation.OutcomeFormat) == DecisionEnvelopeOutcomeFormat
}

// UsesGoalRoutingDecisionEnvelope reports whether the workstation parses reviewer or
// checker output as a decision envelope and routes from the parsed goal decision label
// through authored classificationRoutes instead of WorkOutcome arcs.
func UsesGoalRoutingDecisionEnvelope(workstation *interfaces.FactoryWorkstationConfig) bool {
	if workstation == nil {
		return false
	}
	return UsesDecisionEnvelopeOutcome(workstation) && len(workstation.ClassificationRoutes) > 0
}

// Goal routing decisions use the same vocabulary as packaged goal classifier labels.
const (
	GoalRoutingDecisionAccepted     = "accepted"
	GoalRoutingDecisionNeedsChanges = "needs_changes"
	GoalRoutingDecisionTestsFailed  = "tests_failed"
	GoalRoutingDecisionNeedsHuman   = "needs_human"
	GoalRoutingDecisionBlocked      = "blocked"
	GoalRoutingDecisionInterrupted  = "interrupted"
	GoalRoutingDecisionFailed       = "failed"
)

// NormalizeGoalRoutingDecision canonicalizes a packaged-goal envelope decision label.
func NormalizeGoalRoutingDecision(decision string) (string, error) {
	trimmed := strings.TrimSpace(decision)
	if trimmed == "" {
		return "", fmt.Errorf("decision envelope: decision is required")
	}
	normalized := strings.ToLower(strings.ReplaceAll(trimmed, "-", "_"))
	switch normalized {
	case GoalRoutingDecisionAccepted,
		GoalRoutingDecisionNeedsChanges,
		GoalRoutingDecisionTestsFailed,
		GoalRoutingDecisionNeedsHuman,
		GoalRoutingDecisionBlocked,
		GoalRoutingDecisionInterrupted,
		GoalRoutingDecisionFailed:
		return normalized, nil
	default:
		return "", fmt.Errorf("decision envelope: unknown decision %q", trimmed)
	}
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
func OutcomeFromDecision(decision string) (workerexecution.WorkOutcome, error) {
	trimmed := strings.TrimSpace(decision)
	if trimmed == "" {
		return "", fmt.Errorf("decision envelope: decision is required")
	}
	switch workerexecution.WorkOutcome(trimmed) {
	case workerexecution.OutcomeAccepted,
		workerexecution.OutcomeContinue,
		workerexecution.OutcomeRejected,
		workerexecution.OutcomeFailed:
		return workerexecution.WorkOutcome(trimmed), nil
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
func WorkResultFromDecisionEnvelope(dispatchID, transitionID string, envelope DecisionEnvelope) (workerexecution.WorkResult, error) {
	outcome, err := OutcomeFromDecision(envelope.Decision)
	if err != nil {
		return workerexecution.WorkResult{}, err
	}
	return workerexecution.WorkResult{
		DispatchID:         dispatchID,
		TransitionID:       transitionID,
		Outcome:            outcome,
		Feedback:           envelope.Feedback,
		Output:             envelope.Output,
		RecordedOutputWork: envelope.RecordedOutputWork,
	}, nil
}

// WorkResultFromDecisionEnvelopeJSON parses reviewer/checker output and maps it to WorkResult.
func WorkResultFromDecisionEnvelopeJSON(dispatchID, transitionID, raw string) (workerexecution.WorkResult, error) {
	envelope, err := ParseDecisionEnvelopeJSON(raw)
	if err != nil {
		return workerexecution.WorkResult{}, err
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
) workerexecution.WorkResult {
	message := "reviewer decision envelope invalid"
	if err != nil {
		message += ": " + err.Error()
	}
	return workerexecution.WorkResult{
		DispatchID:   dispatchID,
		TransitionID: transitionID,
		Outcome:      MalformedEnvelopeFailureOutcome,
		Error:        message,
		Feedback:     partial.Feedback,
	}
}

// WorkResultFromDecisionEnvelopeJSONOrFailed parses reviewer/checker output and always
// returns a WorkResult. Invalid JSON and unknown decisions use MalformedEnvelopeFailureOutcome.
func WorkResultFromDecisionEnvelopeJSONOrFailed(dispatchID, transitionID, raw string) workerexecution.WorkResult {
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

// WorkResultFromGoalRoutingDecisionEnvelope maps a parsed envelope onto WorkResult using the
// packaged goal routing vocabulary. Routing uses SelectedClassificationLabel so optional
// envelope output text can remain in WorkResult.Output.
func WorkResultFromGoalRoutingDecisionEnvelope(dispatchID, transitionID string, envelope DecisionEnvelope) (workerexecution.WorkResult, error) {
	label, err := NormalizeGoalRoutingDecision(envelope.Decision)
	if err != nil {
		return workerexecution.WorkResult{}, err
	}
	return workerexecution.WorkResult{
		DispatchID:                  dispatchID,
		TransitionID:                transitionID,
		Outcome:                     workerexecution.OutcomeAccepted,
		SelectedClassificationLabel: label,
		Feedback:                    envelope.Feedback,
		Output:                      envelope.Output,
		RecordedOutputWork:          envelope.RecordedOutputWork,
	}, nil
}

// WorkResultFromGoalRoutingDecisionEnvelopeJSON parses reviewer/checker output and maps it
// to the packaged goal routing WorkResult contract.
func WorkResultFromGoalRoutingDecisionEnvelopeJSON(dispatchID, transitionID, raw string) (workerexecution.WorkResult, error) {
	envelope, err := ParseDecisionEnvelopeJSON(raw)
	if err != nil {
		return workerexecution.WorkResult{}, err
	}
	return WorkResultFromGoalRoutingDecisionEnvelope(dispatchID, transitionID, envelope)
}

// WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed parses reviewer/checker output for
// packaged goal routing workstations and always returns a WorkResult.
func WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(dispatchID, transitionID, raw string) workerexecution.WorkResult {
	envelope, err := ParseDecisionEnvelopeJSON(raw)
	if err != nil {
		return FailedWorkResultFromDecisionEnvelopeError(dispatchID, transitionID, err, DecisionEnvelope{})
	}
	result, err := WorkResultFromGoalRoutingDecisionEnvelope(dispatchID, transitionID, envelope)
	if err != nil {
		return FailedWorkResultFromDecisionEnvelopeError(dispatchID, transitionID, err, envelope)
	}
	return result
}
