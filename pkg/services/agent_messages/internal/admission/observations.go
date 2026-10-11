package admission

import (
	"context"
	"encoding/json"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// persist is called under the admission lock so observations preserve commit
// order, including atomic reply and batch inbox changes. The journal alone
// decides success: an unavailable transient stream cannot undo a durable send
// or turn a successful request retry into another admission.
func (e *Engine) persist(ctx context.Context, t store.Transaction) error {
	if err := e.ledger.Commit(t); err != nil {
		return err
	}
	if e.deliveryLife != nil {
		select {
		case e.deliveryLife.expiryWake <- struct{}{}:
		default:
		}
	}
	if e.events == nil {
		return nil
	}
	// A disconnected client may leave a durable success. Publish that success
	// independently of its request cancellation; there are no external calls.
	publication := context.WithoutCancel(ctx)
	for _, entry := range t.Messages {
		observation := agentmessages.Observation{RecordID: t.RecordID, Sequence: t.Sequence,
			Kind: observationKind(t.Kind, entry), Message: entry.Message}
		payload, err := json.Marshal(observation)
		if err == nil {
			topic := entry.Message.To.ObservationTopic()
			_, attachErr := e.events.AttachSource(publication, events.AttachSourceRequest{
				Destination: agentmessages.ObservationStream, Source: topic,
				StartAt: events.Cursor{Topic: topic}, Mode: events.AttachModeLiveOnly,
			})
			if attachErr != nil && e.logger != nil {
				e.logger.Warn("Agent Message aggregate stream unavailable")
			}
		}
		if err == nil {
			_, err = e.events.Append(publication, events.AppendRequest{
				Topic: entry.Message.To.ObservationTopic(), SourceType: "agent-message",
				SourceID: events.SourceID(entry.Message.MessageID), SourceSequence: events.SourceSequence(t.Sequence),
				SourceEventID: events.SourceEventID(t.RecordID), SchemaID: agentmessages.ObservationSchema, Payload: payload,
			})
		}
		if err != nil && e.logger != nil {
			// Collaborator diagnostics may contain credentials or payloads. Only
			// the stable outcome is safe to publish to operational telemetry.
			e.logger.Warn("Agent Message observation unavailable")
		}
	}
	return nil
}

// Called under the coordinator lock. Durable entries identify subscriptions;
// retained/live Events and an authoritative head read close startup/end races.
func (e *Engine) resumeRecipientObservers(ctx context.Context) {
	entries, _, err := e.ledger.Entries()
	if err != nil {
		return
	}
	intents, err := e.ledger.DeliveryIntents()
	if err != nil {
		return
	}
	assigned := map[string]bool{}
	for _, intent := range intents {
		for _, id := range intent.MessageIDs {
			assigned[id] = true
		}
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !e.eligibleRecipient(entry) || assigned[entry.Message.MessageID] || seen[entry.RecipientChainIdentity] {
			continue
		}
		seen[entry.RecipientChainIdentity] = true
		recipient, err := e.deliveryHead(ctx, entry)
		if err == nil {
			e.observeRecipient(ctx, entry, recipient)
		}
	}
}

func (e *Engine) eligibleRecipient(entry store.Entry) bool {
	return entry.Message.Status == agentmessages.Queued && entry.Message.IfEnded == "REVIVE" && entry.Message.ExpiresAt.After(e.now().UTC())
}

// The same host lifetime owns provider confirmation and recipient-end streams.
// Registration precedes the goroutine so concurrent sends share one observer.
func (e *Engine) observeRecipient(ctx context.Context, entry store.Entry, recipient Identity) {
	life := e.deliveryLife
	key := "end:" + recipient.Observation.FactorySessionID + ":" + recipient.Observation.WorkerSessionID
	if life == nil || e.events == nil || life.cancels[key] != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	observer := &deliveryObserver{cancel: cancel}
	life.cancels[key] = observer
	life.active[observer] = struct{}{}
	life.observers.Add(1)
	go func() {
		defer life.observers.Done()
		defer cancel()
		defer e.forgetDeliveryObserver(life, key, observer)
		scope := recipient.Observation.FactorySessionID
		if recipient.Observation.Direct {
			scope = ""
		}
		topic := workersessions.Topic(recipient.Observation.WorkerSessionID, scope)
		e.watchRecipient(ctx, entry, topic, key, observer)
	}()
}

func (e *Engine) watchRecipient(ctx context.Context, entry store.Entry, topic events.Topic, key string, observer *deliveryObserver) {
	sub, err := e.events.Subscribe(ctx, events.SubscribeRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: workersessions.DefaultObservationStreamLimit})
	// Even unavailable/retention-gapped streams may have an authoritative
	// ended head. No event payload alone grants continuation authority.
	if e.deliverEndedRecipient(ctx, entry, key, observer) || err != nil {
		return
	}
	for {
		delivery := sub.Next(ctx)
		if ctx.Err() != nil || e.deliverEndedRecipient(ctx, entry, key, observer) {
			return
		}
		if delivery.Kind == events.DeliveryGap && delivery.Gap != nil && delivery.Gap.Validate() == nil && delivery.Gap.Topic == topic {
			continue // Events resumes retained catch-up, then live delivery.
		}
		if delivery.Kind == events.DeliveryBackpressure {
			// Only the authoritative head determines completion. A bounded
			// subscription can overflow on retained or live progress; resume
			// at the current Events head and check completion after registering
			// so an end between the read and subscription is never lost.
			sub, err = e.recipientLiveSubscription(ctx, topic)
			if e.deliverEndedRecipient(ctx, entry, key, observer) || err != nil {
				return
			}
			continue
		}
		if delivery.Kind != events.DeliveryRecord {
			return
		}
	}
}

func (e *Engine) recipientLiveSubscription(ctx context.Context, topic events.Topic) (events.Subscription, error) {
	head, err := e.events.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 1})
	if err != nil || head.Validate() != nil || head.Outcome == events.ReadOutcomeInvalidCursor {
		return nil, agentmessages.ErrStreamUnavailable
	}
	position := head.Retained.Head
	if head.Outcome == events.ReadOutcomeGap {
		position = head.Gap.Head
	}
	sub, err := e.events.Subscribe(ctx, events.SubscribeRequest{Topic: topic,
		From: events.Cursor{Topic: topic, Position: position}, Limit: workersessions.DefaultObservationStreamLimit})
	if err != nil || sub == nil {
		return nil, agentmessages.ErrStreamUnavailable
	}
	return sub, nil
}

// Returns true once the observed head has ended. A new head is observed by
// admission/confirmation; the durable chain reservation prevents branching.
func (e *Engine) deliverEndedRecipient(ctx context.Context, entry store.Entry, key string, observer *deliveryObserver) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ctx.Err() != nil || e.deliveryLife == nil {
		return true
	}
	recipient, err := e.deliveryHead(ctx, entry)
	if err != nil || !recipient.Observation.State.Terminal() {
		return false
	}
	// Remove only this registration before attempting delivery, so admission
	// confirmation can observe the next head even if this goroutine is exiting.
	if e.deliveryLife.cancels[key] == observer {
		delete(e.deliveryLife.cancels, key)
	}
	entries, _, err := e.ledger.Entries()
	if err != nil {
		return true
	}
	for _, candidate := range entries {
		if candidate.RecipientChainIdentity == entry.RecipientChainIdentity && candidate.Message.To.FactorySessionID == entry.Message.To.FactorySessionID && e.eligibleRecipient(candidate) {
			_, err = e.deliver(ctx, nil, candidate)
			if err != nil && e.logger != nil {
				e.logger.Warn("Agent Message end delivery unavailable", "code", "MESSAGE_STORE_UNAVAILABLE")
			}
			break
		}
	}
	return true
}

func observationKind(kind string, entry store.Entry) string {
	if kind == store.ReviveOutcome && entry.Message.RevivedWorkerSessionID != "" {
		return string(agentmessages.DeliveredRevive)
	}
	if (kind == store.DeliveryUpdated || kind == store.ReviveOutcome) && entry.Message.Status == agentmessages.Queued {
		return store.DeliveryUpdated
	}
	if entry.Message.Status == agentmessages.Queued {
		return store.Sent
	}
	return string(entry.Message.Status)
}
