package mapping

import (
	"encoding/json"
	"fmt"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// assistantMessageRole is the one workers.MessagePayload.Role value this L1
// slice treats as customer-facing agent output -- the same literal
// pkg/services/workers/internal/services/workstations/executor/agentrun
// assigns when it constructs an assistant MessagePayload. Every other role
// (user, system, tool, or any other source-native value) is not this
// transport's output to project, so it produces no update rather than an
// error.
const assistantMessageRole = "assistant"
const userMessageRole = "user"

// ProjectMessage projects one MESSAGE-kind record -- workers.PhaseStarted,
// PhaseDelta, or PhaseCompleted, the only phases response-draft validation
// allows for this Kind -- into one agent_message_chunk update, or reports
// that this record produces none. draft.ItemID, when non-blank, becomes the
// update's MessageId unchanged: this function never mints or substitutes an
// identity of its own.
//
// A non-assistant (or blank) role, an unsupported content-block kind, or
// text that reduces to empty all produce (nil, nil): an absent update is
// not an error. A Payload that does not decode into the shape its Phase
// requires (MessageDeltaPayload for PhaseDelta, MessagePayload otherwise)
// reports (nil, ErrMalformedRecord) and never a partial update. A partial
// snapshot (MessagePayload.Partial) still projects its bounded text
// unchanged -- Partial only governs terminal-result authority
// (workers.IsAuthoritativeMessageSnapshot), a concern this pure projection
// does not decide.
func ProjectMessage(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	text, err := messageText(draft.Phase, draft.Payload)
	if err != nil {
		return nil, err
	}
	if text == "" {
		return nil, nil
	}

	chunk := &acpsdk.SessionUpdateAgentMessageChunk{Content: acpsdk.TextBlock(text)}
	if draft.ItemID != "" {
		id := draft.ItemID
		chunk.MessageId = &id
	}
	return &acpsdk.SessionUpdate{AgentMessageChunk: chunk}, nil
}

// ProjectRetained projects a committed record during session/load history
// replay. It differs from Project only for a completed user MESSAGE: the
// prompt-originating client did not need that update live, but a loading
// client needs it to reconstruct the retained conversation with the
// sequencer-assigned MessageId intact.
func ProjectRetained(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	if !isLegalKindPhase(draft.Kind, draft.Phase) || draft.Kind != workers.KindMessage || draft.Phase == workers.PhaseDelta {
		return Project(draft)
	}

	var snapshot workers.MessagePayload
	if err := json.Unmarshal(draft.Payload, &snapshot); err != nil {
		return nil, fmt.Errorf("%w: payload must decode as MessagePayload: %v", ErrMalformedRecord, err)
	}
	if snapshot.Role != userMessageRole {
		return Project(draft)
	}
	text := joinTextBlocks(snapshot.ContentBlocks)
	if text == "" {
		return nil, nil
	}
	chunk := &acpsdk.SessionUpdateUserMessageChunk{Content: acpsdk.TextBlock(text)}
	if draft.ItemID != "" {
		id := draft.ItemID
		chunk.MessageId = &id
	}
	return &acpsdk.SessionUpdate{UserMessageChunk: chunk}, nil
}

// messageText extracts the customer-facing text a MESSAGE record carries:
// the raw delta for PhaseDelta, or the joined TEXT content blocks of an
// assistant-authored snapshot otherwise. It returns ("", nil) for every
// declared no-output case and ("", ErrMalformedRecord) only when payload
// cannot be decoded into the shape its phase requires.
func messageText(phase workers.Phase, payload json.RawMessage) (string, error) {
	if phase == workers.PhaseDelta {
		var delta workers.MessageDeltaPayload
		if err := json.Unmarshal(payload, &delta); err != nil {
			return "", fmt.Errorf("%w: payload must decode as MessageDeltaPayload: %v", ErrMalformedRecord, err)
		}
		if delta.ContentBlockKind != workers.ContentBlockText {
			return "", nil
		}
		return delta.TextDelta, nil
	}

	var snapshot workers.MessagePayload
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return "", fmt.Errorf("%w: payload must decode as MessagePayload: %v", ErrMalformedRecord, err)
	}
	if snapshot.Role != assistantMessageRole {
		return "", nil
	}
	return joinTextBlocks(snapshot.ContentBlocks), nil
}

// joinTextBlocks newline-joins every TEXT content block's text, in stable
// input order, and silently skips every other content-block kind: the
// declared "unsupported content-block kind produces no output" outcome
// applies per block, not to the whole snapshot.
func joinTextBlocks(blocks []workers.ContentBlock) string {
	var parts []string
	for _, block := range blocks {
		if block.Kind != workers.ContentBlockText || block.Text == "" {
			continue
		}
		parts = append(parts, block.Text)
	}
	return strings.Join(parts, "\n")
}

// ProjectReasoning projects one REASONING-kind record -- workers.PhaseStarted,
// PhaseDelta, or PhaseCompleted, the only phases response-draft validation
// allows for this Kind -- into one agent_thought_chunk update, or reports
// that this record produces none. Only the normalized Summary/SummaryDelta
// fields ever cross into the projected text; no other Provenance or
// provider-internal field is exposed. draft.ItemID, when non-blank, becomes
// the update's MessageId unchanged.
func ProjectReasoning(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	text, err := reasoningText(draft.Phase, draft.Payload)
	if err != nil {
		return nil, err
	}
	if text == "" {
		return nil, nil
	}

	chunk := &acpsdk.SessionUpdateAgentThoughtChunk{Content: acpsdk.TextBlock(text)}
	if draft.ItemID != "" {
		id := draft.ItemID
		chunk.MessageId = &id
	}
	return &acpsdk.SessionUpdate{AgentThoughtChunk: chunk}, nil
}

// reasoningText extracts the summary text a REASONING record carries: the
// delta for PhaseDelta, or the accumulated summary otherwise. It returns
// ("", nil) for an empty summary and ("", ErrMalformedRecord) only when
// payload cannot be decoded as workers.ReasoningPayload.
func reasoningText(phase workers.Phase, payload json.RawMessage) (string, error) {
	var body workers.ReasoningPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", fmt.Errorf("%w: payload must decode as ReasoningPayload: %v", ErrMalformedRecord, err)
	}
	if phase == workers.PhaseDelta {
		return body.SummaryDelta, nil
	}
	return body.Summary, nil
}
