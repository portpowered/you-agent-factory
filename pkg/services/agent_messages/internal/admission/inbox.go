package admission

import (
	"context"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// Get permits operator observation and authenticated sender/recipient reads.
// Only an active recipient changes QUEUED to READ; expiry is durable maintenance
// after read authorization, never a status regression or an execution effect.
func (e *Engine) Get(ctx context.Context, request agentmessages.GetRequest) (agentmessages.Message, error) {
	if !e.enabled {
		return agentmessages.Message{}, agentmessages.ErrDisabled
	}
	if !validID(request.MessageID) {
		return agentmessages.Message{}, agentmessages.ErrBadRequest
	}
	caller := request.Caller.Clone()
	var identity Identity
	if caller != nil {
		var err error
		identity, err = e.authority.Authenticate(ctx, caller)
		if err != nil {
			return agentmessages.Message{}, err
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	entries, sequence, err := e.ledger.Entries()
	if err != nil {
		return agentmessages.Message{}, err
	}
	for _, entry := range entries {
		if entry.Message.MessageID != request.MessageID {
			continue
		}
		recipient := identity.Matches(entry.RecipientChainIdentity, entry.RecipientWorkIdentity)
		if caller != nil && !recipient && !identity.Matches(entry.SenderChainIdentity, entry.SenderWorkIdentity) {
			return agentmessages.Message{}, agentmessages.ErrNotPermitted
		}
		return e.readEntry(ctx, caller, recipient, entry, sequence)
	}
	return agentmessages.Message{}, agentmessages.ErrMessageNotFound
}

func (e *Engine) readEntry(ctx context.Context, caller *workersessions.CallerIdentity, recipient bool, entry store.Entry, sequence uint64) (agentmessages.Message, error) {
	now := e.now().UTC()
	kind := readTransition(now, recipient, entry.Message)
	if kind == "" {
		if err := e.readAuthority(ctx, caller); err != nil {
			return agentmessages.Message{}, err
		}
		return entry.Message, nil
	}
	entry.Message.Status = agentmessages.Status(kind)
	t := store.Transaction{Version: store.Version, RecordID: e.newID(), Sequence: sequence + 1,
		Kind: kind, CommittedAt: now, Messages: []store.Entry{entry}, Requests: []store.Request{}}
	if err := e.readAuthority(ctx, caller); err != nil {
		return agentmessages.Message{}, err
	}
	if err := e.ledger.Commit(t); err != nil {
		return agentmessages.Message{}, err
	}
	return entry.Message, nil
}

func (e *Engine) readAuthority(ctx context.Context, caller *workersessions.CallerIdentity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if caller != nil {
		return e.authority.Revalidate(ctx, caller)
	}
	return nil
}

func readTransition(now time.Time, recipient bool, m agentmessages.Message) string {
	if m.Status == agentmessages.Replied || m.Status == agentmessages.Expired {
		return ""
	}
	if !m.ExpiresAt.After(now) {
		return store.Expired
	}
	if recipient && m.Status == agentmessages.Queued {
		return store.Read
	}
	return ""
}
