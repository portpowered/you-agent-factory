package agentrun

import (
	"encoding/json"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// publishAgentFinalMessage delivers the authoritative final assistant turn to
// the request-scoped progress stream. The detached runner uses this helper
// without constructing a Workstation executor or retaining runtime state.
func publishAgentFinalMessage(
	publisher workerexecution.ProgressPublisher,
	dispatchID string,
	content string,
) {
	if publisher == nil || strings.TrimSpace(content) == "" {
		return
	}
	payload, err := json.Marshal(workerexecution.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workerexecution.ContentBlock{{
			Kind: workerexecution.ContentBlockText,
			Text: strings.TrimSpace(content),
		}},
	})
	if err != nil {
		return
	}
	draft := workerexecution.Draft{
		Kind:       workerexecution.KindMessage,
		Phase:      workerexecution.PhaseCompleted,
		DispatchID: strings.TrimSpace(dispatchID),
		ItemID:     strings.TrimSpace(dispatchID) + "-final-message",
		Provenance: workerexecution.Provenance{
			Provider:        "agent-run",
			NativeEventType: "agent_final_response",
			Delivery:        workerexecution.DeliveryNativeFinal,
			Representation:  workerexecution.RepresentationSnapshot,
			Fidelity:        workerexecution.FidelityFinalOnly,
		},
		Payload: payload,
	}
	publisher(workerexecution.CanonicalDraftFragment(dispatchID, draft))
}
