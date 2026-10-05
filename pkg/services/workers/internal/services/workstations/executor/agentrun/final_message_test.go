package agentrun

import (
	"encoding/json"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPublishAgentFinalMessagePublishesCanonicalDraft(t *testing.T) {
	t.Parallel()
	correlation := workerexecution.ExecutionCorrelation{
		FactorySessionID: "factory", RuntimeID: "runtime", RequestID: "request", TraceID: "trace",
		GenerationID: "generation", DispatchID: "logical-dispatch", AttemptID: "dispatch-1",
	}
	var got workerexecution.ProgressFragment
	publishAgentFinalMessage(func(fragment workerexecution.ProgressFragment) {
		got = fragment
	}, " dispatch-1 ", correlation, "  final answer  ")

	if got.DispatchID != "dispatch-1" || got.CanonicalDraft == nil || got.Correlation != correlation {
		t.Fatalf("published fragment = %#v, want dispatch and canonical draft", got)
	}
	draft, ok := got.CanonicalDraft.(workerexecution.Draft)
	if !ok {
		t.Fatalf("canonical draft type = %T, want workers.Draft", got.CanonicalDraft)
	}
	if draft.DispatchID != "dispatch-1" || draft.ItemID != "dispatch-1-final-message" ||
		draft.Phase != workerexecution.PhaseCompleted || draft.Kind != workerexecution.KindMessage ||
		draft.Provenance != (workerexecution.Provenance{
			Provider: "agent-run", NativeEventType: "agent_final_response",
			Delivery: workerexecution.DeliveryNativeFinal, Fidelity: workerexecution.FidelityFinalOnly,
			Representation: workerexecution.RepresentationSnapshot,
		}) {
		t.Fatalf("draft = %#v, want final agent message metadata", draft)
	}
	var payload workerexecution.MessagePayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("draft payload: %v", err)
	}
	if len(payload.ContentBlocks) != 1 || payload.ContentBlocks[0].Text != "final answer" {
		t.Fatalf("draft payload = %#v, want trimmed final answer", payload)
	}

	publishAgentFinalMessage(nil, "dispatch-1", correlation, "ignored")
	publishAgentFinalMessage(func(workerexecution.ProgressFragment) {
		t.Fatal("empty final message was published")
	}, "dispatch-1", correlation, " ")
}
