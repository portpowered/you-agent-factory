package codex

import (
	"context"
	"encoding/json"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func progressEvent(runID string, progress providers.ExecuteProgress) (inference.EventDraft, bool) {
	phase := strings.TrimSpace(progress.Phase)
	if phase == "" || strings.HasPrefix(phase, "diagnostic") {
		return inference.EventDraft{}, false
	}
	switch {
	case phase == "run.started", phase == "turn.started":
		return runEvent(runID, workerexecution.PhaseStarted)
	case phase == "run.completed", phase == "turn.completed":
		return runEvent(runID, workerexecution.PhaseCompleted)
	case strings.HasPrefix(phase, "message."):
		return messageEvent(runID, progress, phase)
	case strings.HasPrefix(phase, "tool."):
		return toolEvent(runID, progress, phase)
	default:
		return inference.EventDraft{}, false
	}
}

func runEvent(runID string, phase workerexecution.Phase) (inference.EventDraft, bool) {
	payload, err := json.Marshal(workerexecution.RunPayload{Status: runPayloadStatus(phase)})
	if err != nil {
		return inference.EventDraft{}, false
	}
	event, err := inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindRun,
		Phase:   phase,
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Provider:        string(providers.IDCodex),
			Delivery:        workerexecution.DeliverySynthesized,
			Representation:  workerexecution.RepresentationNotification,
			Fidelity:        workerexecution.FidelityLifecycleOnly,
			NativeEventType: "providers_progress",
		},
	})
	if err != nil {
		return inference.EventDraft{}, false
	}
	return event, true
}

func runPayloadStatus(phase workerexecution.Phase) string {
	switch phase {
	case workerexecution.PhaseStarted:
		return "started"
	case workerexecution.PhaseCompleted:
		return "completed"
	case workerexecution.PhaseFailed:
		return "failed"
	case workerexecution.PhaseCanceled:
		return "canceled"
	default:
		return strings.ToLower(string(phase))
	}
}

func messageEvent(
	runID string,
	progress providers.ExecuteProgress,
	phase string,
) (inference.EventDraft, bool) {
	itemID := progress.Metadata["correlation_id"]
	if itemID == "" {
		itemID = progress.Metadata["item_id"]
	}
	if itemID == "" {
		itemID = "codex-message"
	}
	workerPhase := workerexecution.PhaseCompleted
	if strings.HasSuffix(phase, ".delta") {
		workerPhase = workerexecution.PhaseDelta
	} else if strings.HasSuffix(phase, ".started") {
		workerPhase = workerexecution.PhaseStarted
	}
	payload, err := json.Marshal(workerexecution.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workerexecution.ContentBlock{{
			Kind: workerexecution.ContentBlockText,
			Text: strings.Clone(progress.Detail),
		}},
	})
	if err != nil {
		return inference.EventDraft{}, false
	}
	fidelity := workerexecution.FidelityFinalOnly
	if workerPhase == workerexecution.PhaseDelta {
		fidelity = workerexecution.FidelityNormalized
	}
	event, err := inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindMessage,
		Phase:   workerPhase,
		ItemID:  itemID,
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Provider:        string(providers.IDCodex),
			Delivery:        workerexecution.DeliveryNativeStream,
			Representation:  workerexecution.RepresentationSnapshot,
			Fidelity:        fidelity,
			NativeEventType: phase,
		},
	})
	if err != nil {
		return inference.EventDraft{}, false
	}
	return event, true
}

func toolEvent(
	runID string,
	progress providers.ExecuteProgress,
	phase string,
) (inference.EventDraft, bool) {
	itemID := progress.Metadata["correlation_id"]
	if itemID == "" {
		itemID = "codex-tool"
	}
	workerPhase := workerexecution.PhaseUpdated
	switch {
	case strings.HasSuffix(phase, ".started"):
		workerPhase = workerexecution.PhaseStarted
	case strings.HasSuffix(phase, ".completed"):
		workerPhase = workerexecution.PhaseCompleted
	case strings.HasSuffix(phase, ".failed"), strings.HasSuffix(phase, ".canceled"):
		workerPhase = workerexecution.PhaseFailed
	}
	payload, err := json.Marshal(workerexecution.ToolPayload{
		ToolCallID:    itemID,
		ToolName:      progress.Metadata["tool_name"],
		Status:        strings.TrimPrefix(phase, "tool."),
		ResultSummary: toolResultSummary(progress.Detail),
	})
	if err != nil {
		return inference.EventDraft{}, false
	}
	event, err := inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindTool,
		Phase:   workerPhase,
		ItemID:  itemID,
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Provider:        string(providers.IDCodex),
			Delivery:        workerexecution.DeliveryNativeStream,
			Representation:  workerexecution.RepresentationNotification,
			Fidelity:        workerexecution.FidelityLifecycleOnly,
			NativeEventType: phase,
		},
	})
	if err != nil {
		return inference.EventDraft{}, false
	}
	return event, true
}

func authoritativeMessageCompletedEvent(
	runID string,
	content string,
) (inference.EventDraft, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return inference.EventDraft{}, false
	}
	payload, err := json.Marshal(workerexecution.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workerexecution.ContentBlock{{
			Kind: workerexecution.ContentBlockText,
			Text: strings.Clone(content),
		}},
	})
	if err != nil {
		return inference.EventDraft{}, false
	}
	event, err := inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindMessage,
		Phase:   workerexecution.PhaseCompleted,
		ItemID:  "codex-message",
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Provider:        string(providers.IDCodex),
			Delivery:        workerexecution.DeliveryNativeStream,
			Representation:  workerexecution.RepresentationSnapshot,
			Fidelity:        workerexecution.FidelityFinalOnly,
			NativeEventType: "message.completed",
		},
	})
	if err != nil {
		return inference.EventDraft{}, false
	}
	return event, true
}

func writeFailureProgress(
	ctx context.Context,
	writer inference.ResponseWriter,
	runID string,
	failure inference.Failure,
) error {
	payload, err := json.Marshal(workerexecution.ErrorPayload{
		Code:      "stream_failed",
		Message:   failure.Message(),
		Retryable: failure.Retryable(),
	})
	if err != nil {
		return err
	}
	event, err := inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindError,
		Phase:   workerexecution.PhaseFailed,
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Provider:        string(providers.IDCodex),
			Delivery:        workerexecution.DeliverySynthesized,
			Representation:  workerexecution.RepresentationNotification,
			Fidelity:        workerexecution.FidelityLifecycleOnly,
			NativeEventType: "provider_failure",
		},
	})
	if err != nil {
		return err
	}
	return writer.WriteEvent(ctx, event)
}

func toolResultSummary(detail string) json.RawMessage {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return nil
	}
	encoded, err := json.Marshal(map[string]string{"detail": detail})
	if err != nil {
		return nil
	}
	return encoded
}
