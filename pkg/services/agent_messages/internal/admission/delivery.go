package admission

import (
	"context"
	"errors"
	"strings"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// deliver reserves a chain under the admission lock, then releases the lock
// during Continue. The durable pending intent prevents another effect on that
// chain while unrelated admissions and inbox operations remain available.
func (e *Engine) deliver(ctx context.Context, caller *workersessions.CallerIdentity, entry store.Entry) (agentmessages.Message, error) {
	m := entry.Message
	if m.IfEnded != "REVIVE" {
		return m, nil
	}
	recipient, err := e.deliveryHead(ctx, entry)
	if err != nil {
		return e.deliveryReason(ctx, entry, "REVIVE_FAILED")
	}
	if !recipient.Observation.State.Terminal() {
		return m, nil
	}
	intents, err := e.ledger.DeliveryIntents()
	if err != nil {
		return agentmessages.Message{}, err
	}
	if reason, blocked := e.reviveGate(entry.RecipientChainIdentity, intents); blocked {
		if reason == "" {
			return m, nil
		}
		return e.deliveryReason(ctx, entry, reason)
	}
	if !recipient.Observation.Revivable {
		return e.deliveryReason(ctx, entry, "REVIVE_UNSUPPORTED")
	}
	entries, sequence, err := e.ledger.Entries()
	if err != nil {
		return agentmessages.Message{}, err
	}
	intent, selected := e.deliveryIntent(entry, recipient.Observation.WorkerSessionID, entries, intents)
	if len(selected) == 0 {
		return m, nil
	}
	tx := store.Transaction{Version: store.Version, RecordID: e.newID(), Sequence: sequence + 1,
		Kind: store.ReviveIntent, CommittedAt: e.now().UTC(), Messages: []store.Entry{}, Requests: []store.Request{}, DeliveryIntents: []store.DeliveryIntent{intent}}
	if err := e.authority.Revalidate(ctx, caller); err != nil {
		return agentmessages.Message{}, err
	}
	if err := e.persist(ctx, tx); err != nil {
		return agentmessages.Message{}, err
	}
	result, admissionErr := e.continueIntent(ctx, intent)
	return e.deliveryOutcome(ctx, intent, selected, m.MessageID, result, admissionErr)
}

// The caller holds e.mu. Restore it before returning, including on panic, so
// every subsequent ledger transition still uses the same admission authority.
func (e *Engine) continueIntent(ctx context.Context, intent store.DeliveryIntent) (workersessions.ContinueResult, error) {
	e.mu.Unlock()
	defer e.mu.Lock()
	return e.continueSession(ctx, workersessions.ContinueRequest{
		FactorySessionID: intent.FactorySessionID, RequestID: intent.RequestID,
		SourceWorkerSessionID: intent.SourceWorkerSessionID, SuccessorWorkerSessionID: intent.SuccessorWorkerSessionID,
		FollowUpInput: intent.FollowUpInput, ResolveHead: true,
	})
}

func (e *Engine) deliveryHead(ctx context.Context, entry store.Entry) (Identity, error) {
	m := entry.Message
	recipient, err := e.authority.Recipient(ctx, m.To.WorkerSessionID, m.To.FactorySessionID)
	if err != nil {
		return Identity{}, err
	}
	head := recipient.Observation.ContinuationHeadWorkerSessionID
	if head != "" && head != recipient.Observation.WorkerSessionID {
		recipient, err = e.authority.Recipient(ctx, head, m.To.FactorySessionID)
	}
	if err != nil || recipient.Chain != entry.RecipientChainIdentity {
		return Identity{}, agentmessages.ErrNotPermitted
	}
	return recipient, nil
}

func (e *Engine) reviveGate(chain string, intents []store.DeliveryIntent) (string, bool) {
	count := 0
	cutoff := e.now().UTC().Add(-time.Hour)
	for _, intent := range intents {
		if intent.RecipientChainIdentity != chain {
			continue
		}
		if intent.State == store.IntentPending {
			return "", true
		}
		if intent.State == store.IntentUncertain {
			return "REVIVE_FAILED", true
		}
		if intent.State == store.IntentAccepted && intent.AcceptedAt.After(cutoff) {
			count++
		}
	}
	return "REVIVE_LIMIT", count >= e.revivesPerHour
}

func (e *Engine) deliveryIntent(entry store.Entry, source string, entries []store.Entry, intents []store.DeliveryIntent) (store.DeliveryIntent, []store.Entry) {
	intent := store.DeliveryIntent{RecipientChainIdentity: entry.RecipientChainIdentity,
		FactorySessionID: entry.Message.To.FactorySessionID, SourceWorkerSessionID: source,
		SuccessorWorkerSessionID: "ws-" + strings.TrimPrefix(e.newID(), "msg-"), ResolveHead: true, State: store.IntentPending}
	selected, bodies := []store.Entry{}, []string{}
	assigned := map[string]bool{}
	for _, prior := range intents {
		for _, id := range prior.MessageIDs {
			assigned[id] = true
		}
	}
	for _, candidate := range entries {
		m := candidate.Message
		if !assigned[m.MessageID] && candidate.RecipientChainIdentity == entry.RecipientChainIdentity && m.To.FactorySessionID == intent.FactorySessionID &&
			m.Status == agentmessages.Queued && m.IfEnded == "REVIVE" && m.ExpiresAt.After(e.now().UTC()) {
			selected = append(selected, candidate)
			intent.MessageIDs = append(intent.MessageIDs, m.MessageID)
			bodies = append(bodies, m.Body)
		}
	}
	if len(selected) > 0 {
		intent.RequestID = intent.MessageIDs[0]
	}
	intent.FollowUpInput = strings.Join(bodies, "\n\n")
	intent.InputSHA256 = digest([]byte(intent.FollowUpInput))
	return intent, selected
}

func (e *Engine) deliveryReason(ctx context.Context, entry store.Entry, reason string) (agentmessages.Message, error) {
	if entry.Message.Reason == reason {
		return entry.Message, nil
	}
	_, sequence, err := e.ledger.Entries()
	if err != nil {
		return agentmessages.Message{}, err
	}
	entry.Message.Reason = reason
	tx := store.Transaction{Version: store.Version, RecordID: e.newID(), Sequence: sequence + 1,
		Kind: store.DeliveryUpdated, CommittedAt: e.now().UTC(), Messages: []store.Entry{entry}, Requests: []store.Request{}}
	if err := e.persist(ctx, tx); err != nil {
		return agentmessages.Message{}, err
	}
	return entry.Message, nil
}

func (e *Engine) deliveryOutcome(ctx context.Context, intent store.DeliveryIntent, entries []store.Entry, messageID string, result workersessions.ContinueResult, admissionErr error) (agentmessages.Message, error) {
	intent = e.classifyAdmission(intent, result, admissionErr)
	var message agentmessages.Message
	for i := range entries {
		current, found, _, err := e.ledger.LookupMessage(entries[i].Message.MessageID)
		if err != nil {
			return agentmessages.Message{}, err
		}
		if !found {
			return agentmessages.Message{}, store.ErrCorrupt
		}
		entries[i] = current
		m := &entries[i].Message
		m.Reason = intent.Reason
		if intent.State == store.IntentAccepted {
			if m.Status == agentmessages.Queued {
				m.Status = agentmessages.DeliveredRevive
			}
			m.RevivedWorkerSessionID, m.To.DeliveredWorkerSessionID = intent.SuccessorWorkerSessionID, intent.SuccessorWorkerSessionID
		}
		if m.MessageID == messageID {
			message = *m
		}
	}
	_, sequence, err := e.ledger.Entries()
	if err != nil {
		return agentmessages.Message{}, err
	}
	tx := store.Transaction{Version: store.Version, RecordID: e.newID(), Sequence: sequence + 1, Kind: store.ReviveOutcome,
		CommittedAt: e.now().UTC(), Messages: entries, Requests: []store.Request{}, DeliveryIntents: []store.DeliveryIntent{intent}}
	if err := e.persist(context.WithoutCancel(ctx), tx); err != nil {
		return agentmessages.Message{}, err
	}
	if e.logger != nil {
		e.logger.Info("Agent Message revival completed", "state", intent.State, "reason", intent.Reason)
	}
	return message, nil
}

func (e *Engine) classifyAdmission(intent store.DeliveryIntent, result workersessions.ContinueResult, admissionErr error) store.DeliveryIntent {
	intent.State, intent.Reason = store.IntentFailed, "REVIVE_FAILED"
	if unsupportedAdmission(result, admissionErr) {
		intent.Reason = "REVIVE_UNSUPPORTED"
	}
	if (admissionErr != nil && !knownRejection(admissionErr)) ||
		(admissionErr == nil && (result.RequestID != intent.RequestID || result.SourceWorkerSessionID != intent.SourceWorkerSessionID || result.SuccessorWorkerSessionID != intent.SuccessorWorkerSessionID || result.Session.ID != intent.SuccessorWorkerSessionID)) {
		intent.State = store.IntentUncertain
	}
	if admissionErr == nil && intent.State != store.IntentUncertain {
		intent.State, intent.Reason, intent.AcceptedAt = store.IntentAccepted, "", e.now().UTC()
	}
	return intent
}

func unsupportedAdmission(result workersessions.ContinueResult, err error) bool {
	if errors.Is(err, workersessions.ErrContinuationProviderSessionMissing) ||
		errors.Is(err, workersessions.ErrContinuationProviderSessionInvalid) || errors.Is(err, workersessions.ErrContinuationExecutionUnavailable) {
		return true
	}
	if !errors.Is(err, workersessions.ErrContinuationNotAccepted) || result.Session.Result == nil || result.Session.Result.Cause == nil {
		return false
	}
	cause := result.Session.Result.Cause
	return cause.ProviderContinuationOutcome == providers.ContinuationOutcomeUnsupported ||
		cause.ProviderContinuationFailureKind == providers.ContinuationFailureKindStale ||
		cause.ProviderContinuationFailureKind == providers.ContinuationFailureKindInvalid ||
		cause.ProviderContinuationFailureKind == providers.ContinuationFailureKindForeign
}

func knownRejection(err error) bool {
	for _, rejected := range []error{workersessions.ErrContinuationProviderSessionMissing, workersessions.ErrContinuationProviderSessionInvalid,
		workersessions.ErrContinuationExecutionUnavailable, workersessions.ErrContinuationSourceNotFound, workersessions.ErrContinuationSourceActive,
		workersessions.ErrContinuationSourceConflict, workersessions.ErrContinuationNotAccepted, workersessions.ErrContinuationServerStopping} {
		if errors.Is(err, rejected) {
			return true
		}
	}
	return false
}
