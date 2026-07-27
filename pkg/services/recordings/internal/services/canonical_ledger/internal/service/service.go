package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	canonicalledger "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger"
)

// Service implements the parent-private canonical ledger owner.
type Service struct {
	ledger recordings.Ledger

	appendMu sync.Mutex
}

var _ canonicalledger.Service = (*Service)(nil)

// New constructs the canonical ledger owner over the runtime ledger seam.
func New(ledger recordings.Ledger) *Service {
	if ledger == nil {
		return nil
	}
	return &Service{ledger: ledger}
}

func (service *Service) Append(
	request recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	if !canonical.ValidAppendEvent(request.Event) {
		return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
	}
	service.appendMu.Lock()
	defer service.appendMu.Unlock()

	generationID := service.ledger.StreamGenerationID()
	if retained, ok := retainedEventByID(service.ledger.CanonicalEvents(), string(request.Event.ID)); ok {
		return acceptedAppendResult(retained, generationID), nil
	}

	legacy := canonical.FactoryEventFromCanonical(request.Event)
	legacy.Context.Sequence = 0
	if request.Event.Scope.FactorySessionID != "" {
		sequence := int(nextScopedSequence(service.ledger.CanonicalEvents(), request.Event.Scope))
		legacy.Context.SessionSequence = &sequence
	}
	service.ledger.AppendRecordedEvent(legacy)

	if retained, ok := retainedEventByID(service.ledger.CanonicalEvents(), string(request.Event.ID)); ok {
		return acceptedAppendResult(retained, generationID), nil
	}
	return recordings.AppendRecordedEventResult{}, nil
}

func acceptedAppendResult(
	event factorydefinitions.FactoryEvent,
	generationID string,
) recordings.AppendRecordedEventResult {
	return recordings.AppendRecordedEventResult{
		Event: canonical.CanonicalEventFromFactory(event, generationID),
	}
}

func retainedEventByID(
	events []factorydefinitions.FactoryEvent,
	id string,
) (factorydefinitions.FactoryEvent, bool) {
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Id == id {
			return events[index], true
		}
	}
	return factorydefinitions.FactoryEvent{}, false
}

func (service *Service) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if request.Scope.FactorySessionID != "" &&
		strings.TrimSpace(request.Scope.FactorySessionID) == "" {
		return recordings.SubscribeResult{}, recordings.ErrInvalidSubscribeScope
	}
	if request.Cursor != nil {
		if request.Cursor.StreamGenerationID == "" || request.Cursor.Sequence < 0 {
			return recordings.SubscribeResult{}, recordings.ErrInvalidReconnectCursor
		}
		if request.Cursor.StreamGenerationID != service.ledger.StreamGenerationID() {
			return recordings.SubscribeResult{}, recordings.ErrReconnectCursorUnavailable
		}
	}
	streamContext, cancel := context.WithCancel(ctx)
	stream, err := service.ledger.Subscribe(streamContext, nil, factorydefinitions.FactoryEventReconnectScope{
		SessionID: request.Scope.FactorySessionID,
	})
	if err != nil {
		cancel()
		if errors.Is(err, recordings.ErrReconnectCursorNotFound) {
			return recordings.SubscribeResult{}, recordings.ErrReconnectCursorExpired
		}
		return recordings.SubscribeResult{}, err
	}
	subscription, err := newEventSubscription(
		stream,
		request.Scope,
		request.Cursor,
		streamContext.Done(),
		cancel,
	)
	if err != nil {
		cancel()
		return recordings.SubscribeResult{}, err
	}
	return recordings.SubscribeResult{
		Subscription: subscription,
	}, nil
}

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
