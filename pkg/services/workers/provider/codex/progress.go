package codex

import (
	"encoding/json"
	"fmt"
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
	payload, err := json.Marshal(workerexecution.RunPayload{Status: string(phase)})
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

func messageEvent(
	runID string,
	progress providers.ExecuteProgress,
	phase string,
) (inference.EventDraft, bool) {
	itemID := progress.Metadata["correlation_id"]
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
		ResultSummary: json.RawMessage(fmt.Sprintf("%q", progress.Detail)),
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
