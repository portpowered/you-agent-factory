package admission

import (
	"context"
	"encoding/json"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

const followPendingBytes = 1024 * 1024

// Follow tails only new process-local observations. Durable history is obtained
// with List. Capture the head and register under the admission lock so a send
// cannot disappear between these two operations. No lock spans a customer
// callback or a wait for Events. Owner loss prevents further observations.
func (e *Engine) Follow(ctx context.Context, request agentmessages.ListRequest, observe func(agentmessages.Observation) error) error {
	if !e.enabled {
		return agentmessages.ErrDisabled
	}
	if observe == nil || request.NextToken != "" || request.MarkRead {
		return agentmessages.ErrBadRequest
	}
	request.Caller = request.Caller.Clone()
	if _, err := e.followIdentity(ctx, request); err != nil {
		return err
	}
	request, err := normalizedList(request)
	if err != nil {
		return err
	}
	subctx, cancel := context.WithCancel(ctx)
	defer cancel()
	subscription, err := e.followSubscription(subctx, request.MaxResults)
	if err != nil {
		return err
	}
	return e.consumeFollow(subctx, request, subscription, observe)
}

func (e *Engine) consumeFollow(ctx context.Context, request agentmessages.ListRequest, subscription events.Subscription, observe func(agentmessages.Observation) error) error {
	for {
		if _, err := e.followIdentity(ctx, request); err != nil {
			return err
		}
		delivery := subscription.Next(ctx)
		if err := ctx.Err(); err != nil {
			return err
		}
		if delivery.Validate() != nil {
			return agentmessages.ErrStreamUnavailable
		}
		switch delivery.Kind {
		case events.DeliveryRecord:
			if err := e.followRecord(ctx, request, delivery.Record, observe); err != nil {
				return err
			}
		case events.DeliveryClosed:
			return nil
		case events.DeliveryGap:
			return agentmessages.ErrStreamGap
		case events.DeliveryBackpressure:
			return agentmessages.ErrStreamBackpressure
		default:
			return agentmessages.ErrStreamUnavailable
		}
	}
}

func (e *Engine) followIdentity(ctx context.Context, request agentmessages.ListRequest) (Identity, error) {
	if err := ctx.Err(); err != nil {
		return Identity{}, err
	}
	if request.Caller != nil {
		return e.authority.Authenticate(ctx, request.Caller)
	}
	if request.ToMe {
		return Identity{}, agentmessages.ErrNotPermitted
	}
	return Identity{}, nil
}

func (e *Engine) followSubscription(ctx context.Context, limit int) (events.Subscription, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.events == nil {
		return nil, agentmessages.ErrStreamUnavailable
	}
	topic := agentmessages.ObservationStream
	head, err := e.events.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 1})
	if err != nil {
		return nil, agentmessages.ErrStreamUnavailable
	}
	if head.Validate() != nil || head.Outcome == events.ReadOutcomeInvalidCursor {
		return nil, agentmessages.ErrStreamUnavailable
	}
	position := head.Retained.Head
	if head.Outcome == events.ReadOutcomeGap {
		position = head.Gap.Head
	}
	subscription, err := e.events.Subscribe(ctx, events.SubscribeRequest{
		Topic: topic, From: events.Cursor{Topic: topic, Position: position},
		Limit: limit, MaxPendingBytes: followPendingBytes,
	})
	if err != nil || subscription == nil {
		return nil, agentmessages.ErrStreamUnavailable
	}
	return subscription, nil
}

func (e *Engine) followRecord(ctx context.Context, request agentmessages.ListRequest, record events.Record, observe func(agentmessages.Observation) error) error {
	identity, err := e.followIdentity(ctx, request)
	if err != nil {
		return err
	}
	observation, err := decodeObservation(record)
	if err != nil {
		return err
	}
	e.mu.Lock()
	entry, found, sequence, err := e.ledger.LookupMessage(observation.Message.MessageID)
	e.mu.Unlock()
	if err != nil {
		return err
	}
	// Retention may have removed a committed record while Events still retains
	// its observation. Never authorize content from topic possession alone.
	if !found || !visibleEntry(request, identity, entry) || !matchesList(request, observation.Message) {
		return nil
	}
	if observation.Sequence < entry.Sequence || observation.Sequence > sequence || !sameAdmittedContent(observation.Message, entry.Message) ||
		observation.Kind != observationKind(store.Entry{Message: observation.Message}) {
		return agentmessages.ErrStreamUnavailable
	}
	if err := e.readAuthority(ctx, request.Caller); err != nil {
		return err
	}
	return observe(observation)
}

func decodeObservation(record events.Record) (agentmessages.Observation, error) {
	var observation agentmessages.Observation
	if record.ID.Topic != agentmessages.ObservationStream || record.SchemaID != agentmessages.ObservationSchema || record.SourceType != "agent-message" {
		return observation, agentmessages.ErrStreamUnavailable
	}
	if json.Unmarshal(record.Payload, &observation) != nil || observation.Message.MessageID != string(record.SourceID) ||
		observation.RecordID != string(record.SourceEventID) || observation.Sequence != uint64(record.SourceSequence) {
		return agentmessages.Observation{}, agentmessages.ErrStreamUnavailable
	}
	return observation, nil
}

// Historical status changes can lag the durable head, but source observations
// must never substitute different content or addresses for an admitted ID.
func sameAdmittedContent(observed, admitted agentmessages.Message) bool {
	switch observed.Status {
	case agentmessages.Queued, agentmessages.Read, agentmessages.Replied, agentmessages.Expired:
	default:
		return false
	}
	if !observed.SentAt.Equal(admitted.SentAt) || !observed.ExpiresAt.Equal(admitted.ExpiresAt) {
		return false
	}
	observed.Status, observed.RepliedByMessageID = admitted.Status, admitted.RepliedByMessageID
	observed.SentAt, observed.ExpiresAt = admitted.SentAt, admitted.ExpiresAt
	return observed == admitted
}
