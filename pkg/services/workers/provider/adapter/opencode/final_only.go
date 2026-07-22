package opencode

import (
	"context"
	"strings"
	"unicode/utf8"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	responseevents "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

type finalOnlyDecoder struct{}

func (finalOnlyDecoder) Observe(context.Context, adapter.Observation) (adapter.DecodeResult, error) {
	return adapter.DecodeResult{}, nil
}

func (finalOnlyDecoder) Flush(context.Context, adapter.FlushContext) (adapter.DecodeResult, error) {
	return adapter.DecodeResult{}, nil
}

func parseFinalOnly(_ context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	if input.CommandError != nil || input.CommandResult.ExitCode != 0 {
		return adapter.FinalParseResult{}, processTerminalError(input.CommandResult, input.CommandError)
	}
	if !utf8.Valid(input.CommandResult.Stdout) {
		return adapter.FinalParseResult{}, unusableFinalOnlyOutput()
	}
	content := strings.TrimSpace(string(input.CommandResult.Stdout))
	if content == "" {
		return adapter.FinalParseResult{}, unusableFinalOnlyOutput()
	}
	return adapter.FinalParseResult{
		Response: workerexecution.InferenceResponse{Content: content},
		Drafts:   finalOnlyDrafts(input, content),
	}, nil
}

func unusableFinalOnlyOutput() *structuredTerminalError {
	return &structuredTerminalError{
		failureType: workerexecution.WorkFailureTypeUnknown,
		message:     "OpenCode final-only output did not contain an authoritative response.",
	}
}

func finalOnlyDrafts(input adapter.FinalParseContext, content string) []responseevents.Draft {
	publishedContent := boundedText(content)
	correlate := func(draft responseevents.Draft) responseevents.Draft {
		draft.RunID = input.RunID
		draft.DispatchID = input.DispatchID
		return draft
	}
	return []responseevents.Draft{
		correlate(finalOnlyRunDraft(responseevents.PhaseStarted, "started")),
		correlate(responseevents.Draft{
			Kind: responseevents.KindMessage, Phase: responseevents.PhaseCompleted,
			Provenance: responseevents.Provenance{
				Provider: "opencode", NativeEventType: "final_response",
				Delivery: responseevents.DeliveryNativeFinal, Representation: responseevents.RepresentationSnapshot,
				Fidelity: responseevents.FidelityFinalOnly,
			},
			Payload: marshalCanonicalPayload(responseevents.MessagePayload{
				Role: "assistant", ContentBlocks: []responseevents.ContentBlock{{Kind: responseevents.ContentBlockText, Text: publishedContent}},
			}),
		}),
		correlate(finalOnlyRunDraft(responseevents.PhaseCompleted, "completed")),
	}
}

func finalOnlyRunDraft(phase responseevents.Phase, status string) responseevents.Draft {
	return responseevents.Draft{
		Kind: responseevents.KindRun, Phase: phase,
		Provenance: responseevents.Provenance{
			Provider: "opencode", NativeEventType: "command_completion",
			Delivery: responseevents.DeliverySynthesized, Representation: responseevents.RepresentationNotification,
			Fidelity: responseevents.FidelityLifecycleOnly,
		},
		Payload: marshalCanonicalPayload(responseevents.RunPayload{Status: status}),
	}
}

var _ adapter.Decoder = finalOnlyDecoder{}
