package kiro

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	inference "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/inferencecontract"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const providerIdentity = "kiro"

func writeFinalOnlyProgress(
	ctx context.Context,
	writer inference.ResponseWriter,
	runID, content string,
) error {
	events, err := finalOnlyProgressEvents(runID, content)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := writer.WriteEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func finalOnlyProgressEvents(runID, content string) ([]inference.EventDraft, error) {
	started, err := finalOnlyRunEvent(runID, workerexecution.PhaseStarted)
	if err != nil {
		return nil, err
	}
	message, err := finalOnlyMessageEvent(runID, content)
	if err != nil {
		return nil, err
	}
	completed, err := finalOnlyRunEvent(runID, workerexecution.PhaseCompleted)
	if err != nil {
		return nil, err
	}
	return []inference.EventDraft{started, message, completed}, nil
}

func finalOnlyRunEvent(runID string, phase workerexecution.Phase) (inference.EventDraft, error) {
	payload, err := json.Marshal(workerexecution.RunPayload{Status: string(phase)})
	if err != nil {
		return inference.EventDraft{}, fmt.Errorf("marshal Kiro run payload: %w", err)
	}
	return inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindRun,
		Phase:   phase,
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Delivery:        workerexecution.DeliverySynthesized,
			Fidelity:        workerexecution.FidelityLifecycleOnly,
			NativeEventType: "command_completion",
			Provider:        providerIdentity,
			Representation:  workerexecution.RepresentationNotification,
		},
	})
}

func finalOnlyMessageEvent(runID, content string) (inference.EventDraft, error) {
	payload, err := json.Marshal(workerexecution.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workerexecution.ContentBlock{{
			Kind: workerexecution.ContentBlockText,
			Text: strings.Clone(content),
		}},
	})
	if err != nil {
		return inference.EventDraft{}, fmt.Errorf("marshal Kiro message payload: %w", err)
	}
	return inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindMessage,
		Phase:   workerexecution.PhaseCompleted,
		ItemID:  "kiro-final",
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Delivery:        workerexecution.DeliveryNativeFinal,
			Fidelity:        workerexecution.FidelityFinalOnly,
			NativeEventType: "final_response",
			Provider:        providerIdentity,
			Representation:  workerexecution.RepresentationSnapshot,
		},
	})
}

func writeFailureProgress(
	ctx context.Context,
	writer inference.ResponseWriter,
	runID string,
	failure inference.Failure,
) error {
	payload, err := json.Marshal(workerexecution.ErrorPayload{
		Code:      string(failure.Kind()),
		Message:   failure.Message(),
		Retryable: failure.Retryable(),
	})
	if err != nil {
		return fmt.Errorf("marshal Kiro failure payload: %w", err)
	}
	event, err := inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindError,
		Phase:   workerexecution.PhaseFailed,
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Delivery:        workerexecution.DeliverySynthesized,
			Fidelity:        workerexecution.FidelityLifecycleOnly,
			NativeEventType: "provider_failure",
			Provider:        providerIdentity,
			Representation:  workerexecution.RepresentationNotification,
		},
	})
	if err != nil {
		return err
	}
	return writer.WriteEvent(ctx, event)
}
