package admission

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	"github.com/portpowered/infinite-you/pkg/services/events"
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
		e.observeRecipient(ctx, entry, recipient)
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
	// Background delivery consumes previously authorized durable messages;
	// it does not reuse the sender's execution credentials.
	if caller != nil {
		if err := e.authority.Revalidate(ctx, caller); err != nil {
			return agentmessages.Message{}, err
		}
	}
	if err := e.persist(ctx, tx); err != nil {
		return agentmessages.Message{}, err
	}
	result, admissionErr := e.continueIntent(ctx, intent)
	intent = e.classifyAdmission(intent, result, admissionErr)
	message, err := e.deliveryOutcome(ctx, intent, selected, m.MessageID)
	if err == nil && intent.State == store.IntentAdmitted {
		e.observeDelivery(ctx, intent)
	}
	return message, err
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
		if intent.State == store.IntentPending || intent.State == store.IntentAdmitted {
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

func (e *Engine) deliveryOutcome(ctx context.Context, intent store.DeliveryIntent, entries []store.Entry, messageID string) (agentmessages.Message, error) {
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
		for _, entry := range entries {
			e.logger.Info("Agent Message revival outcome committed", "message_id", entry.Message.MessageID, "state", intent.State, "reason", intent.Reason)
		}
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
		intent.State, intent.Reason, intent.AcceptedAt = store.IntentAdmitted, "REVIVE_PENDING", e.now().UTC()
	}
	return intent
}

// The host owns cancellation and joins every observer before releasing its
// binding. Multiple hosts share the already-selected Messaging instance.
type deliveryLifetime struct {
	hosts     int
	observers sync.WaitGroup
	cancels   map[string]*deliveryObserver
	active    map[*deliveryObserver]struct{}
}

type deliveryObserver struct{ cancel context.CancelFunc }

func (e *Engine) BindHost() func() {
	e.mu.Lock()
	if !e.enabled {
		e.mu.Unlock()
		return func() {}
	}
	if e.deliveryLife == nil {
		e.deliveryLife = &deliveryLifetime{cancels: map[string]*deliveryObserver{}, active: map[*deliveryObserver]struct{}{}}
	}
	life := e.deliveryLife
	life.hosts++
	if life.hosts == 1 {
		e.resumeRecipientObservers(context.Background())
	}
	e.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { e.releaseDeliveryHost(life) }) }
}

func (e *Engine) releaseDeliveryHost(life *deliveryLifetime) {
	e.mu.Lock()
	life.hosts--
	last := life.hosts == 0
	if last {
		for observer := range life.active {
			observer.cancel()
		}
		e.deliveryLife = nil
	}
	e.mu.Unlock()
	if last {
		life.observers.Wait()
	}
}

// Called under the coordinator lock after ADMITTED has flushed. Subscribe
// atomically drains retained events before live events, including provider
// output that beat the admission reply. A gap never counts as confirmation.
func (e *Engine) observeDelivery(ctx context.Context, intent store.DeliveryIntent) {
	life := e.deliveryLife
	if life == nil || e.events == nil || life.cancels[intent.RequestID] != nil {
		return
	}
	life.observers.Add(1)
	// The admitted effect outlives the HTTP caller; the host owns its observer.
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	observer := &deliveryObserver{cancel: cancel}
	life.cancels[intent.RequestID] = observer
	life.active[observer] = struct{}{}
	go func(ctx context.Context) {
		defer life.observers.Done()
		defer cancel()
		defer e.forgetDeliveryObserver(life, intent.RequestID, observer)
		topic := workersessions.Topic(intent.SuccessorWorkerSessionID)
		sub, err := e.events.Subscribe(ctx, events.SubscribeRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: workersessions.DefaultObservationStreamLimit})
		if err != nil {
			e.settleDelivery(ctx, intent, store.IntentUncertain, "REVIVE_FAILED")
			return
		}
		e.consumeDelivery(ctx, intent, sub)
	}(ctx)
}

func (e *Engine) forgetDeliveryObserver(life *deliveryLifetime, requestID string, observer *deliveryObserver) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(life.active, observer)
	if life.cancels[requestID] == observer {
		delete(life.cancels, requestID)
	}
}

func (e *Engine) consumeDelivery(ctx context.Context, intent store.DeliveryIntent, sub events.Subscription) {
	for {
		delivery := sub.Next(ctx)
		if ctx.Err() != nil {
			return // shutdown preserves durable ADMITTED for startup reconciliation
		}
		if delivery.Kind != events.DeliveryRecord {
			state, reason := e.providerTerminal(ctx, intent)
			e.settleDelivery(ctx, intent, state, reason)
			return
		}
		var draft providerRecord
		if delivery.Record.SchemaID != "workers.draft.v1" || json.Unmarshal(delivery.Record.Payload, &draft) != nil {
			continue
		}
		if providerProgress(draft) {
			e.settleDelivery(ctx, intent, store.IntentAccepted, "")
			return
		}
		if draft.Kind == "SESSION" && (draft.Phase == "FAILED" || draft.Phase == "COMPLETED" || draft.Phase == "CANCELED") {
			state, reason := e.providerTerminal(ctx, intent)
			e.settleDelivery(ctx, intent, state, reason)
			return
		}
	}
}

// Source closure and terminal records are wakeups, not guessed classifications.
// Get retains the authoritative typed result even when descriptive observation
// enrichment has no provider-failure fields. Missing facts remain uncertain.
func (e *Engine) providerTerminal(ctx context.Context, intent store.DeliveryIntent) (string, string) {
	session, err := e.readSession(ctx, workersessions.GetRequest{ID: intent.SuccessorWorkerSessionID, FactorySessionID: intent.FactorySessionID})
	if err != nil || session.ID != intent.SuccessorWorkerSessionID || !session.State.Terminal() {
		return store.IntentUncertain, "REVIVE_FAILED"
	}
	if session.State == workersessions.StateFailed && session.Result != nil && session.Result.Cause != nil && unsupportedCause(session.Result.Cause) {
		return store.IntentFailed, "REVIVE_UNSUPPORTED"
	}
	return store.IntentFailed, "REVIVE_FAILED"
}

// providerRecord projects only classification fields from the public
// source-native workers.draft.v1 envelope. It never decodes message bodies or
// consumes Workers execution policy.
type providerRecord struct {
	Kind       string `json:"kind"`
	Phase      string `json:"phase"`
	Provenance struct {
		Provider        string `json:"provider"`
		Delivery        string `json:"delivery"`
		NativeEventType string `json:"nativeEventType"`
	} `json:"provenance"`
}

func providerProgress(draft providerRecord) bool {
	if draft.Provenance.Provider == "" || draft.Provenance.NativeEventType == "" || (draft.Provenance.Delivery != "NATIVE_STREAM" && draft.Provenance.Delivery != "NATIVE_FINAL") {
		return false
	}
	// Session discovery and dispatch lifecycle do not prove resumed work.
	if strings.HasPrefix(draft.Provenance.NativeEventType, "session.") {
		return false
	}
	switch draft.Phase {
	case "STARTED", "DELTA", "UPDATED", "COMPLETED":
	default:
		return false
	}
	switch draft.Kind {
	case "RUN", "TURN", "MESSAGE", "REASONING", "TOOL", "FILE_CHANGE", "PLAN", "USAGE":
		return true
	default:
		return false
	}
}

func (e *Engine) settleDelivery(ctx context.Context, intent store.DeliveryIntent, state, reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	intents, err := e.ledger.DeliveryIntents()
	if err != nil || ctx.Err() != nil {
		return
	}
	for _, current := range intents {
		if current.RequestID != intent.RequestID || current.State != store.IntentAdmitted {
			continue
		}
		current.State, current.Reason = state, reason
		if state != store.IntentAccepted {
			current.AcceptedAt = time.Time{}
		}
		entries := make([]store.Entry, 0, len(current.MessageIDs))
		for _, id := range current.MessageIDs {
			entry, found, _, err := e.ledger.LookupMessage(id)
			if err != nil || !found {
				return
			}
			entries = append(entries, entry)
		}
		_, err = e.deliveryOutcome(ctx, current, entries, current.RequestID)
		if err != nil && e.logger != nil {
			e.logger.Warn("Agent Message revival outcome unavailable", "code", "MESSAGE_STORE_UNAVAILABLE")
		}
		if err == nil {
			e.resumeRecipientObservers(ctx)
		}
		return
	}
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
	return unsupportedCause(cause)
}

func unsupportedCause(cause *workersessions.FailureCause) bool {
	return cause.ProviderFailureKind == providers.ExecuteFailureKindSessionNotFound ||
		cause.ProviderContinuationOutcome == providers.ContinuationOutcomeUnsupported ||
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
