package agy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

const maxPublishedTextBytes = 256 * 1024

type finalOnlyDecoder struct{}

func (finalOnlyDecoder) Observe(context.Context, adapter.Observation) (adapter.DecodeResult, error) {
	return adapter.DecodeResult{}, nil
}

func (finalOnlyDecoder) Flush(context.Context, adapter.FlushContext) (adapter.DecodeResult, error) {
	return adapter.DecodeResult{}, nil
}

func parseFinalOnly(_ context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	if input.CommandError != nil || input.CommandResult.ExitCode != 0 {
		terminal := processTerminalError(input.CommandResult, input.CommandError)
		return adapter.FinalParseResult{Drafts: timeoutFailureDrafts(input, terminal)}, terminal
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

func unusableFinalOnlyOutput() *terminalError {
	return &terminalError{
		failureType: workerexecution.WorkFailureTypeUnknown,
		message:     "Agy final-only output did not contain an authoritative response.",
	}
}

func timeoutFailureDrafts(input adapter.FinalParseContext, terminal *terminalError) []responseevents.Draft {
	if terminal == nil || terminal.failureType != workerexecution.WorkFailureTypeTimeout {
		return nil
	}
	var drafts []responseevents.Draft
	if partial, ok := partialTimeoutMessageDrafts(input); ok {
		drafts = append(drafts, partial...)
	}
	drafts = append(drafts, timeoutTerminalFailureDrafts(input, terminal)...)
	return drafts
}

func timeoutTerminalFailureDrafts(input adapter.FinalParseContext, terminal *terminalError) []responseevents.Draft {
	return []responseevents.Draft{
		timeoutErrorDraft(input, terminal),
		timeoutFailedRunDraft(input),
	}
}

func timeoutErrorDraft(input adapter.FinalParseContext, terminal *terminalError) responseevents.Draft {
	return responseevents.Draft{
		RunID: input.RunID, DispatchID: input.DispatchID,
		Kind: responseevents.KindError, Phase: responseevents.PhaseUpdated,
		Provenance: responseevents.Provenance{
			Provider: string(modelprovider.Agy), NativeEventType: "session_timeout",
			Delivery: responseevents.DeliverySynthesized, Representation: responseevents.RepresentationNotification,
			Fidelity: responseevents.FidelityLifecycleOnly,
		},
		Payload: marshalCanonicalPayload(responseevents.ErrorPayload{
			Code: string(terminal.failureType), Message: terminal.message, Retryable: terminal.retryable,
		}),
	}
}

func timeoutFailedRunDraft(input adapter.FinalParseContext) responseevents.Draft {
	draft := finalOnlyRunDraft(responseevents.PhaseFailed, "failed")
	draft.RunID = input.RunID
	draft.DispatchID = input.DispatchID
	return draft
}

func partialTimeoutMessageDrafts(input adapter.FinalParseContext) ([]responseevents.Draft, bool) {
	if !isAgySessionTimeout(input.CommandResult, input.CommandError) {
		return nil, false
	}
	content, ok := usableTimeoutCapture(input.CommandResult)
	if !ok {
		return nil, false
	}
	return []responseevents.Draft{partialTimeoutMessageDraft(input, content)}, true
}

func isAgySessionTimeout(result workerprocess.CommandResult, commandErr error) bool {
	return errors.Is(commandErr, agypty.ErrSessionTimedOut) || result.ExitCode == 124
}

func usableTimeoutCapture(result workerprocess.CommandResult) (string, bool) {
	if !utf8.Valid(result.Stdout) {
		return "", false
	}
	content := strings.TrimSpace(string(result.Stdout))
	if content == "" {
		return "", false
	}
	if agypty.ContainsTerminalEscapeOrControl(content) {
		return "", false
	}
	return boundedText(content), true
}

func partialTimeoutMessageDraft(input adapter.FinalParseContext, content string) responseevents.Draft {
	return responseevents.Draft{
		RunID: input.RunID, DispatchID: input.DispatchID,
		Kind: responseevents.KindMessage, Phase: responseevents.PhaseCompleted,
		Provenance: responseevents.Provenance{
			Provider: string(modelprovider.Agy), NativeEventType: "timeout_partial_response",
			Delivery: responseevents.DeliverySynthesized, Representation: responseevents.RepresentationSnapshot,
			Fidelity: responseevents.FidelityLossy,
		},
		Payload: marshalCanonicalPayload(responseevents.MessagePayload{
			Role: "assistant", Partial: true,
			ContentBlocks: []responseevents.ContentBlock{{Kind: responseevents.ContentBlockText, Text: content}},
		}),
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
				Provider: string(modelprovider.Agy), NativeEventType: "final_response",
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
			Provider: string(modelprovider.Agy), NativeEventType: "command_completion",
			Delivery: responseevents.DeliverySynthesized, Representation: responseevents.RepresentationNotification,
			Fidelity: responseevents.FidelityLifecycleOnly,
		},
		Payload: marshalCanonicalPayload(responseevents.RunPayload{Status: status}),
	}
}

func boundedText(value string) string {
	if len(value) <= maxPublishedTextBytes {
		return value
	}
	end := maxPublishedTextBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end]
}

func marshalCanonicalPayload(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

var _ adapter.Decoder = finalOnlyDecoder{}
