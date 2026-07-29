package agy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/inferencecontract"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

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
	provider := string(modelprovider.ProviderAntigravity)
	started, err := finalOnlyRunEvent(runID, provider, workerexecution.PhaseStarted)
	if err != nil {
		return nil, err
	}
	message, err := finalOnlyMessageEvent(runID, provider, content)
	if err != nil {
		return nil, err
	}
	completed, err := finalOnlyRunEvent(runID, provider, workerexecution.PhaseCompleted)
	if err != nil {
		return nil, err
	}
	return []inference.EventDraft{started, message, completed}, nil
}

func finalOnlyRunEvent(runID, provider string, phase workerexecution.Phase) (inference.EventDraft, error) {
	payload, err := json.Marshal(workerexecution.RunPayload{Status: string(phase)})
	if err != nil {
		return inference.EventDraft{}, fmt.Errorf("marshal Agy run payload: %w", err)
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
			Provider:        provider,
			Representation:  workerexecution.RepresentationNotification,
		},
	})
}

func finalOnlyMessageEvent(runID, provider, content string) (inference.EventDraft, error) {
	payload, err := json.Marshal(workerexecution.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workerexecution.ContentBlock{{
			Kind: workerexecution.ContentBlockText,
			Text: strings.Clone(content),
		}},
	})
	if err != nil {
		return inference.EventDraft{}, fmt.Errorf("marshal Agy message payload: %w", err)
	}
	return inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindMessage,
		Phase:   workerexecution.PhaseCompleted,
		ItemID:  "agy-final",
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Delivery:        workerexecution.DeliveryNativeFinal,
			Fidelity:        workerexecution.FidelityFinalOnly,
			NativeEventType: "final_response",
			Provider:        provider,
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
		Code:      failureErrorCode(failure),
		Message:   failure.Message(),
		Retryable: failure.Retryable(),
	})
	if err != nil {
		return fmt.Errorf("marshal Agy failure payload: %w", err)
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
			Provider:        string(modelprovider.ProviderAntigravity),
			Representation:  workerexecution.RepresentationNotification,
		},
	})
	if err != nil {
		return err
	}
	return writer.WriteEvent(ctx, event)
}

func failureErrorCode(failure inference.Failure) string {
	switch failure.Kind() {
	case inference.FailureTimeout:
		return "timeout"
	case inference.FailureThrottled:
		return "throttled"
	case inference.FailureInvalidRequest, inference.FailureMalformedOutput:
		return "permanent_bad_request"
	default:
		return "stream_failed"
	}
}
