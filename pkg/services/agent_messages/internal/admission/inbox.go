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
	entry, exists, sequence, err := e.ledger.LookupMessage(request.MessageID)
	if err != nil {
		return agentmessages.Message{}, err
	}
	if !exists {
		return agentmessages.Message{}, agentmessages.ErrMessageNotFound
	}
	recipient := identity.Matches(entry.RecipientChainIdentity, entry.RecipientWorkIdentity)
	if caller != nil && !recipient && !identity.Matches(entry.SenderChainIdentity, entry.SenderWorkIdentity) {
		return agentmessages.Message{}, agentmessages.ErrNotPermitted
	}
	return e.readEntry(ctx, caller, recipient, entry, sequence)
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
	if err := e.persist(ctx, t); err != nil {
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
	if recipient && (m.Status == agentmessages.Queued || m.Status == agentmessages.DeliveredRevive) {
		return store.Read
	}
	return ""
}

// One host-owned scheduler expires unreplied content independently of readers.
// Every committed change wakes it so newly admitted or replied content updates
// the next deadline. The admission lock serializes expiry with delivery.
func (e *Engine) startExpiry(life *deliveryLifetime) {
	ctx, cancel := context.WithCancel(context.Background())
	life.expiryCancel = cancel
	life.observers.Add(1)
	go func() {
		defer life.observers.Done()
		defer cancel()
		for {
			deadline, err := e.expireDue(ctx, life)
			if err != nil {
				if ctx.Err() == nil && e.logger != nil {
					e.logger.Warn("Agent Message expiry unavailable", "code", "MESSAGE_STORE_UNAVAILABLE")
				}
				return // A quarantined journal cannot admit further maintenance.
			}
			var fired <-chan time.Time
			var stop func() bool
			if !deadline.IsZero() {
				timer := e.scheduler.NewTimer(deadline.Sub(e.now().UTC()))
				fired, stop = timer.C(), timer.Stop
			}
			select {
			case <-ctx.Done():
			case <-life.expiryWake:
			case <-fired:
			}
			if stop != nil {
				stop()
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()
}

func (e *Engine) expireDue(ctx context.Context, life *deliveryLifetime) (time.Time, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	if e.deliveryLife != life {
		return time.Time{}, context.Canceled
	}
	entries, sequence, err := e.ledger.Entries()
	if err != nil {
		return time.Time{}, err
	}
	now := e.now().UTC()
	var deadline time.Time
	changed := []store.Entry{}
	for _, entry := range entries {
		m := entry.Message
		if m.Status == agentmessages.Replied || m.Status == agentmessages.Expired {
			continue
		}
		if readTransition(now, false, m) == store.Expired {
			entry.Message.Status = agentmessages.Expired
			changed = append(changed, entry)
		} else if deadline.IsZero() || m.ExpiresAt.Before(deadline) {
			deadline = m.ExpiresAt
		}
	}
	if len(changed) == 0 {
		return deadline, nil
	}
	err = e.persist(ctx, store.Transaction{Version: store.Version, RecordID: e.newID(), Sequence: sequence + 1,
		Kind: store.Expired, CommittedAt: now, Messages: changed, Requests: []store.Request{}})
	if err == nil && e.logger != nil {
		e.logger.Info("Agent Message expiry committed", "count", len(changed))
	}
	return deadline, err
}
