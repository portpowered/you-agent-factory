package goal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// DecisionEnvelope is the canonical JSON response shape for reviewer/checker workers
// in packaged @you/goal flows.
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
