package run

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers/provider/parityfixtures"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestResponseStreamNDJSON_PublicVocabularyDecodesAfterPrivateRemoval(t *testing.T) {
	t.Parallel()

	event := responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          "event-migration-1",
		Sequence:         1,
		FactorySessionID: "session-1",
		RunID:            "run-1",
		Kind:             responseevents.KindProgress,
		Phase:            responseevents.PhaseUpdated,
		Payload:          []byte(`{"label":"planning","message":"next step"}`),
		Provenance: responseevents.Provenance{
			Provider:       "test-provider",
			Delivery:       responseevents.DeliverySynthesized,
			Representation: responseevents.RepresentationNotification,
			Fidelity:       responseevents.FidelityNormalized,
		},
		DispatchID: "dispatch-1",
	}
	invocation := interfaces.FactoryInvocationResult{
		RequestID: "req-migration-1",
		TraceID:   "trace-migration-1",
		Status:    factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "done"},
		},
	}

	lines, err := parityfixtures.EncodeTransportCLINDJSON([]responseevents.FactoryResponseEvent{event}, invocation)
	if err != nil {
		t.Fatalf("EncodeTransportCLINDJSON: %v", err)
	}
	decodedEvents, decodedInvocation, err := parityfixtures.DecodeTransportCLINDJSON(lines)
	if err != nil {
		t.Fatalf("DecodeTransportCLINDJSON: %v", err)
	}
	if len(decodedEvents) != 1 || decodedEvents[0].EventID != event.EventID {
		t.Fatalf("decoded events = %#v", decodedEvents)
	}
	if decodedInvocation.RequestId != invocation.RequestID {
		t.Fatalf("decoded invocation = %#v", decodedInvocation)
	}

	for _, retired := range []string{"progress", "compaction", "primary_result", "stream_gap"} {
		if strings.Contains(strings.Join(lines, "\n"), `"recordType":"`+retired+`"`) {
			t.Fatalf("public vocabulary output still contains retired recordType %q:\n%s", retired, strings.Join(lines, "\n"))
		}
	}
}

func TestResponseStreamNDJSON_RendererOutputDecodesThroughPublicContract(t *testing.T) {
	t.Parallel()

	event := canonicalResponseEventFixture(2, responseevents.KindMessage)
	result := apisurface.FactoryInvocationResult{
		RequestID: "req-migration-2",
		Status:    factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "final"},
		},
	}

	var output strings.Builder
	renderer := newJSONResponseStreamRenderer(&output)
	renderer.onResponseEvents([]responseevents.FactoryResponseEvent{event})
	if err := renderer.writeFinalInvocationResult(result); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}
	if _, _, err := parityfixtures.DecodeTransportCLINDJSON(strings.Split(strings.TrimSpace(output.String()), "\n")); err != nil {
		t.Fatalf("renderer output must decode through public transport contract: %v\n%s", err, output.String())
	}
}
