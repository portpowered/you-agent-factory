package claude

import (
	"encoding/json"
	"strconv"
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
	case phase == "run.started":
		return runEvent(runID, workerexecution.PhaseStarted)
	case phase == "run.completed":
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
			Provider:        string(providers.IDClaude),
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

func messageEvent(
	runID string,
	progress providers.ExecuteProgress,
	phase string,
) (inference.EventDraft, bool) {
	itemID := progress.Metadata["correlation_id"]
	if itemID == "" {
		itemID = progress.Metadata["message_id"]
	}
	if itemID == "" {
		itemID = "claude-message"
	}
	workerPhase := workerexecution.PhaseCompleted
	if strings.HasSuffix(phase, ".delta") {
		workerPhase = workerexecution.PhaseDelta
	} else if strings.HasSuffix(phase, ".started") {
		workerPhase = workerexecution.PhaseStarted
	}
	var payload []byte
	var err error
	representation := workerexecution.RepresentationSnapshot
	fidelity := workerexecution.FidelityFinalOnly
	switch workerPhase {
	case workerexecution.PhaseDelta:
		blockIndex := 0
		if raw := strings.TrimSpace(progress.Metadata["content_block_index"]); raw != "" {
			if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed >= 0 {
				blockIndex = parsed
			}
		}
		payload, err = json.Marshal(workerexecution.MessageDeltaPayload{
			ContentBlockIndex: blockIndex,
			ContentBlockKind:  workerexecution.ContentBlockText,
			TextDelta:         strings.Clone(progress.Detail),
		})
		representation = workerexecution.RepresentationDelta
		fidelity = workerexecution.FidelityNormalized
	case workerexecution.PhaseStarted:
		contentBlocks := []workerexecution.ContentBlock{}
		if detail := strings.TrimSpace(progress.Detail); detail != "" {
			contentBlocks = append(contentBlocks, workerexecution.ContentBlock{
				Kind: workerexecution.ContentBlockText,
				Text: detail,
			})
		} else {
			contentBlocks = append(contentBlocks, workerexecution.ContentBlock{
				Kind: workerexecution.ContentBlockText,
			})
		}
		payload, err = json.Marshal(workerexecution.MessagePayload{
			Role:          "assistant",
			ContentBlocks: contentBlocks,
		})
		fidelity = workerexecution.FidelityLifecycleOnly
	default:
		payload, err = json.Marshal(workerexecution.MessagePayload{
			Role: "assistant",
			ContentBlocks: []workerexecution.ContentBlock{{
				Kind: workerexecution.ContentBlockText,
				Text: strings.Clone(progress.Detail),
			}},
		})
	}
	if err != nil {
		return inference.EventDraft{}, false
	}
	event, err := inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindMessage,
		Phase:   workerPhase,
		ItemID:  itemID,
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Provider:        string(providers.IDClaude),
			Delivery:        workerexecution.DeliveryNativeStream,
			Representation:  representation,
			Fidelity:        fidelity,
			NativeEventType: phase,
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

func toolEvent(
	runID string,
	progress providers.ExecuteProgress,
	phase string,
) (inference.EventDraft, bool) {
	itemID := progress.Metadata["correlation_id"]
	if itemID == "" {
		itemID = "claude-tool"
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
			Provider:        string(providers.IDClaude),
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
		ItemID:  "claude-message",
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Provider:        string(providers.IDClaude),
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
