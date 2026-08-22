package stdio

import (
	"context"
	"encoding/json"
	"fmt"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

const (
	promptHistorySourceType events.SourceType = "acp_prompt"
	promptHistorySchemaID   events.SchemaID   = "acp.prompt.v1"
)

// recordPromptHistory commits this newly admitted ACP prompt as the Chat
// topic's user-authored MESSAGE record before Factory execution begins. It
// deliberately uses the existing workers.MessagePayload vocabulary rather
// than a transport-local event shape, and derives a stable idempotency tuple
// from the immutable Chat Turn ID. The later retained-history loader projects
// this record as user_message_chunk; normal live prompt streaming leaves it
// suppressed because the originating client already supplied the prompt.
func (s *Server) recordPromptHistory(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	prompt session.PromptTurn,
) (uint64, error) {
	if s.events == nil {
		return startResult.Session.Version, nil
	}
	blocks := make([]workers.ContentBlock, len(prompt.Content))
	for i, content := range prompt.Content {
		blocks[i] = workers.ContentBlock{Kind: workers.ContentBlockText, Text: content.Text}
	}
	payload, err := json.Marshal(workers.MessagePayload{Role: "user", ContentBlocks: blocks})
	if err != nil {
		return 0, fmt.Errorf("marshal prompt history: %w", err)
	}
	sequenced, err := s.chatSessions.Sequence(ctx, chatsessions.SequenceRequest{
		SessionID: startResult.Session.ID, SourceType: promptHistorySourceType,
		SourceID: events.SourceID(startResult.Turn.ID), SourceSequence: 1,
		SourceEventID: "user_message", SchemaID: promptHistorySchemaID,
		Kind: workers.KindMessage, Phase: workers.PhaseCompleted, Payload: payload,
	})
	if err != nil {
		return 0, fmt.Errorf("sequence prompt history: %w", err)
	}
	advanced, err := s.chatSessions.AdvanceStreamHead(ctx, chatsessions.AdvanceStreamHeadRequest{
		SessionID: startResult.Session.ID, ExpectedVersion: startResult.Session.Version,
		AggregateSequence: sequenced.AggregateSequence, SourceType: promptHistorySourceType,
		SourceID: events.SourceID(startResult.Turn.ID), SourceSequence: 1, SourceEventID: "user_message",
	})
	if err != nil {
		return 0, fmt.Errorf("advance prompt history stream head: %w", err)
	}
	return advanced.Session.Version, nil
}

// recoverStrandedTurn makes one best-effort attempt to move turnID to
// fallback after its primary terminalizing AdvanceTurn call already failed,
// so that failure alone can never strand the session's busy/active-turn
// state forever. Its own outcome is intentionally not surfaced to the
// caller: the caller already has the primary failure to report, and no
// further fallback is attempted beyond this single recovery call.
func (s *Server) recoverStrandedTurn(ctx context.Context, sessionID, turnID string, fallback chatsessions.TurnState) {
	_, _ = s.chatSessions.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: sessionID,
		TurnID:    turnID,
		Next:      fallback,
	})
}
