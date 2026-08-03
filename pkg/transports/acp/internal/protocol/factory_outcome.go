package protocol

import (
	acpsdk "github.com/coder/acp-go-sdk"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// PromptOutcome is the bounded, safe-to-serialize projection of one Factory
// Session outcome this transport maps into a "session/prompt" result: only
// the deterministic ACP stop reason and the text extracted from supported
// public primary-result content parts, in stable input order. It never
// carries an invocation's raw ErrorCode, Message, session/work identifiers,
// or any other internal field -- a caller must not fall back to serializing
// the source result itself when this projection's Text is empty.
type PromptOutcome struct {
	StopReason acpsdk.StopReason
	Text       []string
}

// MapFactoryInvocationOutcome is a total, deterministic mapping from one
// completed Factory Session invocation (InvokeFactoryTarget, or the
// synchronous activation StartFactoryTarget's own on-demand implementation
// performs for a first turn) outcome to the bounded ACP prompt outcome this
// transport returns. Text is projected only from the invocation's published
// "text" primary-result parts, in the same order they appear on
// result.PrimaryResult; an absent PrimaryResult or one containing only
// unsupported part kinds (image/audio/JSON/binary) yields a nil Text rather
// than fabricated content.
func MapFactoryInvocationOutcome(result factorysessions.InvocationResult) PromptOutcome {
	return PromptOutcome{
		StopReason: mapFactoryTerminalStatus(string(result.Status)),
		Text:       primaryResultText(result.PrimaryResult),
	}
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

// primaryResultText extracts, in stable input order, only the "text" parts
// of a published Factory Session primary result. Every other supported
// public content part kind (image/audio/JSON/binary) is skipped rather than
// fabricated into text.
func primaryResultText(parts []work.WorkContentPart) []string {
	var text []string
	for _, part := range parts {
		if part.Type.Normalized() != work.WorkContentPartTypeText {
			continue
		}
		text = append(text, part.Text)
	}
	return text
}
