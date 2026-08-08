package protocol

import (
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// PromptOutcome is the bounded, safe-to-serialize projection of one Factory
// Session outcome this transport maps into a "session/prompt" result: only
// the deterministic ACP stop reason and the text projected from the
// invocation's own published primary-result content parts, in stable input
// order. Those parts are the Factory's answer to the customer, which is why
// they cross this boundary at all. It never
// carries an invocation's raw ErrorCode, Message, session/work identifiers,
// or any other internal field -- a caller must not fall back to serializing
// the source result itself when this projection's Text is empty.
type PromptOutcome struct {
	StopReason acpsdk.StopReason
	Text       []string
}

// MapFactoryInvocationOutcome is a total, deterministic mapping from one
// completed Factory Session invocation (InvokeFactorySession, or the
// synchronous activation StartAsync's own on-demand implementation
// performs for a first turn) outcome to the bounded ACP prompt outcome this
// transport returns. Text is projected from the invocation's published
// primary-result parts in the same order they appear on result.PrimaryResult:
// its "text" parts when it has any, and otherwise its structured "json" parts
// serialized as-is, so a Factory that succeeds always returns its result. An
// absent PrimaryResult, or one carrying only parts with no text form at all
// (image/audio/binary), yields a nil Text rather than fabricated content.
func MapFactoryInvocationOutcome(result factorysessions.InvocationResult) PromptOutcome {
	return PromptOutcome{
		StopReason: mapFactoryTerminalStatus(string(result.Status)),
		Text:       primaryResultText(result.PrimaryResult),
	}
}

// FactoryInvocationFailure reports the JSON-RPC error a non-completed Factory
// invocation must answer "session/prompt" with, or nil when the invocation
// reached a completed outcome.
//
// ACP has no failure StopReason: the vocabulary is end_turn, max_tokens,
// max_turn_requests, refusal, and cancelled, none of which means "this run
// broke". A failed invocation therefore cannot be expressed in a successful
// prompt response at all, and reporting one as end_turn makes a failed run
// indistinguishable from a successful one. The protocol's own failure channel
// is the JSON-RPC error response, which is what this returns.
//
// Only InvocationErrorCode crosses the boundary, never Message. The error code
// is a closed, Factory Session-owned vocabulary; Message is free-form
// diagnostic text that can carry provider commands, paths, and credentials,
// which is why PromptOutcome refuses to serialize it and why this maps the
// code rather than passing the message through.
//
// A cancelled or timed-out invocation is not a failure: both have a truthful
// StopReason (cancelled), so they keep answering with a successful response.
func FactoryInvocationFailure(result factorysessions.InvocationResult) *acpsdk.RequestError {
	if !failedInvocationStatus(string(result.Status)) {
		return nil
	}
	return acpsdk.NewInternalError(map[string]any{"reason": boundedInvocationErrorCode(result.ErrorCode)})
}

func failedInvocationStatus(status string) bool {
	return status == string(factorysessions.InvocationTerminalStatusFailed)
}

// boundedInvocationErrorCode narrows the published error code to the declared
// vocabulary. An unrecognized value is replaced rather than forwarded: the
// whole reason this is safe to disclose is that its value set is closed, so a
// code this transport does not recognize is treated as unbounded text.
func boundedInvocationErrorCode(code string) string {
	switch factorysessions.InvocationErrorCode(code) {
	case factorysessions.InvocationErrorCodeCanceled,
		factorysessions.InvocationErrorCodeRuntimeFailure,
		factorysessions.InvocationErrorCodeTimedOut:
		return code
	}
	return string(factorysessions.InvocationErrorCodeRuntimeFailure)
}

// mapFactoryTerminalStatus is the total mapping MapFactoryInvocationOutcome
// applies to the published InvocationTerminalStatus vocabulary
// ("COMPLETED"/"CANCELED"/"FAILED"/"TIMED_OUT"). A published completed
// outcome ("COMPLETED"/"SUCCEEDED") maps to end_turn. A turn that stopped
// before reaching a genuine completed or failed outcome -- caller-cancelled
// or timed out -- maps to cancelled, the closest published ACP stop reason
// for "did not run to natural completion" (ACP has no dedicated timeout
// vocabulary). A published failure, or any other unmapped status, falls back
// to end_turn -- the same documented safe default this transport uses
// elsewhere for a Factory failure -- and never discloses the underlying
// status, error code, or message.
func mapFactoryTerminalStatus(status string) acpsdk.StopReason {
	switch status {
	case string(factorysessions.InvocationTerminalStatusCompleted), "SUCCEEDED":
		return acpsdk.StopReasonEndTurn
	case string(factorysessions.InvocationTerminalStatusCanceled), string(factorysessions.InvocationTerminalStatusTimedOut):
		return acpsdk.StopReasonCancelled
	default:
		return acpsdk.StopReasonEndTurn
	}
}

// primaryResultText projects a result's published text, preferring the
// Factory's own text parts and falling back to serializing its structured
// parts when it published no text at all.
//
// The fallback exists because a Factory that succeeds must return its result.
// @you/deep-research publishes its synthesis as a JSON part rather than a text
// one, so a text-only projection produced a completed turn carrying no
// assistant message -- on the wire, indistinguishable from a Factory that ran
// and had nothing to say.
//
// It is a fallback rather than an addition: when a Factory publishes text, that
// text is what it chose to say, and appending a structured part beside it would
// show a customer the same answer twice in two shapes.
func primaryResultText(parts []work.WorkContentPart) []string {
	if text := textPrimaryResultParts(parts); len(text) > 0 {
		return text
	}
	return structuredPrimaryResultParts(parts)
}

func textPrimaryResultParts(parts []work.WorkContentPart) []string {
	var text []string
	for _, part := range parts {
		if part.Type.Normalized() != work.WorkContentPartTypeText {
			continue
		}
		text = append(text, part.Text)
	}
	return text
}

// structuredPrimaryResultParts serializes JSON parts in the order they were
// published. Only JSON is projected: an image, audio, or binary part has no
// text form, so rendering one would fabricate content rather than deliver it.
func structuredPrimaryResultParts(parts []work.WorkContentPart) []string {
	var text []string
	for _, part := range parts {
		if part.Type.Normalized() != work.WorkContentPartTypeJSON {
			continue
		}
		encoded := strings.TrimSpace(string(part.JSON))
		if encoded == "" {
			continue
		}
		text = append(text, encoded)
	}
	return text
}
