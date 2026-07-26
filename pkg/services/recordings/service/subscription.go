package service

import (
	"context"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func nextScopedSequence(
	events []factorydefinitions.FactoryEvent,
	scope recordings.CanonicalEventScope,
) recordings.CanonicalEventSequence {
	next := recordings.CanonicalEventSequence(0)
	for _, event := range events {
		if !factoryEventBelongsToScope(event, scope) {
			continue
		}
		if event.Context.SessionSequence == nil {
			next++
			continue
		}
		sequence := recordings.CanonicalEventSequence(*event.Context.SessionSequence)
		if sequence >= next {
			next = sequence + 1
		}
	}
	return next
}

type eventSubscription struct {
	history      []factorydefinitions.FactoryEvent
	live         <-chan factorydefinitions.FactoryEvent
	generationID string
	scope        recordings.CanonicalEventScope
	nextDelivery recordings.CanonicalEventSequence
	lastCursor   recordings.CanonicalEventCursor
	sourceDone   <-chan struct{}
	cancel       context.CancelFunc
	terminal     bool
}

func newEventSubscription(
	stream factorydefinitions.FactoryEventStream,
	scope recordings.CanonicalEventScope,
	cursor *recordings.CanonicalEventCursor,
	sourceDone <-chan struct{},
	cancel context.CancelFunc,
) (recordings.EventSubscription, error) {
	history, nextDelivery, err := scopedHistoryAfter(stream.History, scope, cursor)
	if err != nil {
		return nil, err
	}
	subscription := &eventSubscription{
		history:      history,
		live:         stream.Events,
		generationID: stream.StreamGenerationID,
		scope:        scope,
		nextDelivery: nextDelivery,
		sourceDone:   sourceDone,
		cancel:       cancel,
	}
	if cursor != nil {
		subscription.lastCursor = *cursor
	}
	return subscription.Next, nil
}

func scopedHistoryAfter(
	history []factorydefinitions.FactoryEvent,
	scope recordings.CanonicalEventScope,
	cursor *recordings.CanonicalEventCursor,
) ([]factorydefinitions.FactoryEvent, recordings.CanonicalEventSequence, error) {
	filtered := make([]factorydefinitions.FactoryEvent, 0, len(history))
	cursorFound := cursor == nil
	nextDelivery := recordings.CanonicalEventSequence(0)
	for _, event := range history {
		if !factoryEventBelongsToScope(event, scope) {
			continue
		}
		if !cursorFound {
			if recordings.CanonicalEventSequence(event.Context.Sequence) == cursor.Sequence {
				cursorFound = true
				nextDelivery = deliverySequenceForScope(event, scope) + 1
			}
			continue
		}
		filtered = append(filtered, event)
	}
	if !cursorFound {
		return nil, 0, recordings.ErrReconnectCursorExpired
	}
	return filtered, nextDelivery, nil
}

func (subscription *eventSubscription) Next(ctx context.Context) recordings.SubscriptionOutcome {
	if subscription.terminal {
		return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
	}
	for {
		event, status := subscription.next(ctx)
		if status == subscriptionReadCancelled {
			subscription.close()
			return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
		}
		if status == subscriptionReadTerminated {
			subscription.close()
			return recordings.SubscriptionOutcome{
				Kind: recordings.SubscriptionGap,
				Gap: &recordings.SubscriptionGapFacts{
					Cause:            recordings.SubscriptionBackpressure,
					ExpectedSequence: subscription.nextDelivery,
					ObservedSequence: subscription.nextDelivery,
					ReconnectFrom:    subscription.lastCursor,
				},
			}
		}
		if !factoryEventBelongsToScope(event, subscription.scope) {
			continue
		}
		deliverySequence := deliverySequenceForScope(event, subscription.scope)
		if deliverySequence < subscription.nextDelivery {
			continue
		}
		if deliverySequence > subscription.nextDelivery {
			subscription.close()
			return recordings.SubscriptionOutcome{
				Kind: recordings.SubscriptionGap,
				Gap: &recordings.SubscriptionGapFacts{
					Cause:            recordings.SubscriptionSequenceDiscontinuity,
					ExpectedSequence: subscription.nextDelivery,
					ObservedSequence: deliverySequence,
					ReconnectFrom:    subscription.lastCursor,
				},
			}
		}
		subscription.nextDelivery++
		canonical := canonicalEventFromFactory(event, subscription.generationID)
		subscription.lastCursor = canonical.Cursor
		return recordings.SubscriptionOutcome{
			Kind:  recordings.SubscriptionEvent,
			Event: canonical,
		}
	}
}

func (subscription *eventSubscription) close() {
	subscription.terminal = true
	if subscription.cancel != nil {
		subscription.cancel()
	}
}

type subscriptionReadStatus int

const (
	subscriptionReadEvent subscriptionReadStatus = iota
	subscriptionReadCancelled
	subscriptionReadTerminated
)

func (subscription *eventSubscription) next(
	ctx context.Context,
) (factorydefinitions.FactoryEvent, subscriptionReadStatus) {
	if len(subscription.history) > 0 {
		event := subscription.history[0]
		subscription.history = subscription.history[1:]
		return event, subscriptionReadEvent
	}
	select {
	case <-ctx.Done():
		return factorydefinitions.FactoryEvent{}, subscriptionReadCancelled
	case <-subscription.sourceDone:
		return factorydefinitions.FactoryEvent{}, subscriptionReadCancelled
	case event, ok := <-subscription.live:
		if !ok {
			select {
			case <-subscription.sourceDone:
				return factorydefinitions.FactoryEvent{}, subscriptionReadCancelled
			default:
				return factorydefinitions.FactoryEvent{}, subscriptionReadTerminated
			}
		}
		return event, subscriptionReadEvent
	}
}

func deliverySequenceForScope(
	event factorydefinitions.FactoryEvent,
	scope recordings.CanonicalEventScope,
) recordings.CanonicalEventSequence {
	if scope.FactorySessionID != "" && event.Context.SessionSequence != nil {
		return recordings.CanonicalEventSequence(*event.Context.SessionSequence)
	}
	return recordings.CanonicalEventSequence(event.Context.Sequence)
}

func factoryEventBelongsToScope(
	event factorydefinitions.FactoryEvent,
	scope recordings.CanonicalEventScope,
) bool {
	if scope.FactorySessionID == "" {
		return true
	}
	return event.Context.SessionID != nil &&
		strings.TrimSpace(*event.Context.SessionID) ==
			strings.TrimSpace(scope.FactorySessionID)
}
