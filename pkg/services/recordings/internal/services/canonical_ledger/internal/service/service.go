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
	legacy := canonical.FactoryEventFromCanonical(request.Event)
	if request.Event.Scope.FactorySessionID != "" {
		sequence := int(nextScopedSequence(service.ledger.CanonicalEvents(), request.Event.Scope))
		legacy.Context.SessionSequence = &sequence
	}
	service.ledger.AppendRecordedEvent(legacy)
	recorded := service.ledger.CanonicalEvents()
	for index := len(recorded) - 1; index >= 0; index-- {
		if recorded[index].Id == string(request.Event.ID) {
			return recordings.AppendRecordedEventResult{
				Event: canonical.CanonicalEventFromFactory(
					recorded[index],
					service.ledger.StreamGenerationID(),
				),
			}, nil
		}
	}
	return recordings.AppendRecordedEventResult{}, nil
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
