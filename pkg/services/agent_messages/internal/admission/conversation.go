package admission

import (
	"context"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func replyParent(request agentmessages.SendRequest, sender Identity, entries []store.Entry) (*store.Entry, error) {
	if request.InReplyTo == "" {
		return nil, nil
	}
	for _, entry := range entries {
		if entry.Message.MessageID == request.InReplyTo {
			if !sender.Matches(entry.RecipientChainIdentity, entry.RecipientWorkIdentity) {
				return nil, agentmessages.ErrNotPermitted
			}
			return &entry, nil
		}
	}
	return nil, agentmessages.ErrMessageNotFound
}

func replyDefaults(p Prepared, parent *store.Entry, factory string) (Prepared, string) {
	if parent == nil {
		return p, factory
	}
	if p.Request.To == nil {
		p.Request.To = &agentmessages.Address{WorkerSessionID: parent.Message.From.WorkerSessionID}
		factory = parent.Message.From.FactorySessionID
	}
	if p.Request.IfEnded == "" {
		p.Request.IfEnded = parent.Message.ReplyIfEnded
	}
	return p, factory
}

func identicalMessage(now time.Time, p Prepared, sender, recipient Identity, entries []store.Entry) (agentmessages.Message, bool) {
	for _, entry := range entries {
		m := entry.Message
		if entry.SenderChainIdentity == sender.Chain && entry.RecipientChainIdentity == recipient.Chain &&
			m.To.WorkerSessionID == recipient.Observation.WorkerSessionID &&
			m.To.FactorySessionID == recipient.Observation.FactorySessionID &&
			m.BodySHA256 == p.BodySHA256 && m.Body == p.Request.Body &&
			m.SentAt.After(now.Add(-10*time.Minute)) && !m.SentAt.After(now) {
			return m, true
		}
	}
	return agentmessages.Message{}, false
}

func (e *Engine) admit(ctx context.Context, caller *workersessions.CallerIdentity, p Prepared, sender, recipient Identity, parent *store.Entry, entries []store.Entry, sequence uint64, now time.Time, alias store.Request) (agentmessages.Message, error) {
	thread, hop := "", 0
	if parent != nil {
		if parent.Message.Status == agentmessages.Replied || parent.Message.Status == agentmessages.Expired || !parent.Message.ExpiresAt.After(now) {
			return agentmessages.Message{}, agentmessages.ErrBadRequest
		}
		// Hop counts forwarding, not ordinary replies. T5 exposes no forward
		// operation, so replies preserve the thread's existing forward count.
		thread, hop = parent.Message.ThreadID, parent.Message.Hop
	}
	if err := e.quota.Check(now, sender.Chain, thread, hop, entries); err != nil {
		return agentmessages.Message{}, err
	}
	m := e.newMessage(p, sender, recipient, thread, hop, now)
	entry := store.Entry{Message: m, SenderChainIdentity: sender.Chain, RecipientChainIdentity: recipient.Chain,
		SenderWorkIdentity: sender.Work, RecipientWorkIdentity: recipient.Work, Sequence: sequence + 1}
	changed, kind := []store.Entry{entry}, store.Sent
	if parent != nil {
		updated := *parent
		updated.Message.Status = agentmessages.Replied
		updated.Message.RepliedByMessageID = m.MessageID
		changed, kind = append(changed, updated), store.Replied
	}
	alias.MessageID = m.MessageID
	if err := e.commit(ctx, caller, e.transaction(sequence, now, kind, changed, alias)); err != nil {
		return agentmessages.Message{}, err
	}
	return m, nil
}

func (e *Engine) newMessage(p Prepared, sender, recipient Identity, thread string, hop int, now time.Time) agentmessages.Message {
	id := e.newID()
	if thread == "" {
		thread = id
	}
	m := agentmessages.Message{MessageID: id, ThreadID: thread, InReplyTo: p.Request.InReplyTo,
		From: agentmessages.Sender{Principal: "WORKER", WorkerSessionID: sender.Observation.WorkerSessionID, FactorySessionID: sender.Observation.FactorySessionID},
		To:   agentmessages.Recipient{Kind: "WORKER_SESSION", WorkerSessionID: recipient.Observation.WorkerSessionID, FactorySessionID: recipient.Observation.FactorySessionID},
		Body: p.Request.Body, BodySHA256: p.BodySHA256, BodyRedactionCount: p.BodyRedactionCount, Correlation: p.Request.Correlation,
		Delivery: p.Request.Delivery, IfEnded: p.Request.IfEnded, ReplyIfEnded: p.Request.ReplyIfEnded,
		Status: agentmessages.Queued, Hop: hop, SentAt: now, ExpiresAt: now.Add(time.Duration(*p.Request.ExpiresInSeconds) * time.Second)}
	if recipient.Observation.State.Terminal() && m.IfEnded == "REVIVE" {
		m.Reason = "REVIVE_UNSUPPORTED"
	}
	return m
}
