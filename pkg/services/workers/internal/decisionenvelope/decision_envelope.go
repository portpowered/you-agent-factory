// Package decisionenvelope interprets reviewer/checker output at the Workers
// execution boundary. Definitions resolves authored workstation facts; the
// execution owner owns parsing the provider's response.
package decisionenvelope

import (
	"encoding/json"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type Envelope struct {
	Decision           string                 `json:"decision"`
	Feedback           string                 `json:"feedback"`
	Output             string                 `json:"output,omitempty"`
	RecordedOutputWork []work.FactoryWorkItem `json:"recorded_output_work,omitempty"`
}

func UsesDecisionEnvelopeOutcome(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	return workstation != nil && strings.TrimSpace(workstation.OutcomeFormat) == factorydefinitions.WorkstationOutcomeFormatDecisionEnvelope
}

func UsesGoalRoutingDecisionEnvelope(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	return UsesDecisionEnvelopeOutcome(workstation) && len(workstation.ClassificationRoutes) > 0
}

func parse(raw string) (Envelope, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Envelope{}, fmt.Errorf("decision envelope: output is empty")
	}
	var envelope Envelope
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return Envelope{}, fmt.Errorf("decision envelope: invalid JSON: %w", err)
	}
	return envelope, nil
}

func outcome(decision string) (workerexecution.WorkOutcome, error) {
	switch workerexecution.WorkOutcome(strings.TrimSpace(decision)) {
	case workerexecution.OutcomeAccepted, workerexecution.OutcomeContinue,
		workerexecution.OutcomeRejected, workerexecution.OutcomeFailed:
		return workerexecution.WorkOutcome(strings.TrimSpace(decision)), nil
	default:
		if strings.TrimSpace(decision) == "" {
			return "", fmt.Errorf("decision envelope: decision is required")
		}
		return "", fmt.Errorf("decision envelope: unknown decision %q", strings.TrimSpace(decision))
	}
}

func normalizeGoalDecision(decision string) (string, error) {
	trimmed := strings.TrimSpace(decision)
	if trimmed == "" {
		return "", fmt.Errorf("decision envelope: decision is required")
	}
	normalized := strings.ToLower(strings.ReplaceAll(trimmed, "-", "_"))
	switch normalized {
	case "accepted", "needs_changes", "tests_failed", "needs_human", "blocked", "interrupted", "failed":
		return normalized, nil
	default:
		return "", fmt.Errorf("decision envelope: unknown decision %q", trimmed)
	}
}

func failure(dispatchID, transitionID string, err error, partial Envelope) workerexecution.WorkResult {
	message := "reviewer decision envelope invalid"
	if err != nil {
		message += ": " + err.Error()
	}
	return workerexecution.WorkResult{
		DispatchID: dispatchID, TransitionID: transitionID,
		Outcome: workerexecution.OutcomeFailed, Error: message,
		Feedback: partial.Feedback,
	}
}

func WorkResult(dispatchID, transitionID, raw string, goalRouting bool) workerexecution.WorkResult {
	envelope, err := parse(raw)
	if err != nil {
		return failure(dispatchID, transitionID, err, Envelope{})
	}
	if goalRouting {
		label, err := normalizeGoalDecision(envelope.Decision)
		if err != nil {
			return failure(dispatchID, transitionID, err, envelope)
		}
		return workerexecution.WorkResult{
			DispatchID: dispatchID, TransitionID: transitionID,
			Outcome:                     workerexecution.OutcomeAccepted,
			SelectedClassificationLabel: label,
			Feedback:                    envelope.Feedback, Output: envelope.Output,
			RecordedOutputWork: envelope.RecordedOutputWork,
		}
	}
	outcome, err := outcome(envelope.Decision)
	if err != nil {
		return failure(dispatchID, transitionID, err, envelope)
	}
	return workerexecution.WorkResult{
		DispatchID: dispatchID, TransitionID: transitionID,
		Outcome: outcome, Feedback: envelope.Feedback, Output: envelope.Output,
		RecordedOutputWork: envelope.RecordedOutputWork,
	}
}
