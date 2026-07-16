package apisurface

import (
	"context"
	"errors"
	"fmt"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ErrInvalidNamedFactoryName retains the public compatibility identity while
// canonical named-factory validation remains owned by config.
var ErrInvalidNamedFactoryName = factoryconfig.ErrInvalidNamedFactoryName

// ErrFactoryResponseEventStreamExpired reports that the completed session's
// ephemeral response-event retention window elapsed before subscription.
var ErrFactoryResponseEventStreamExpired = errors.New("factory response event stream expired")

// FactoryResponseEventRecord is one transport-neutral serialized observation
// returned by a session-owned ephemeral response-event subscription.
type FactoryResponseEventRecord struct {
	Sequence int64
	Kind     string
	Data     []byte
}

// FactoryResponseEventSubscription is the transport-independent cursor exposed
// by one session-owned ephemeral response-event store. The HTTP transport owns
// detachment; canceling that observer must not cancel the Factory Session run.
type FactoryResponseEventSubscription interface {
	Next(ctx context.Context) ([]FactoryResponseEventRecord, error)
	Detach()
}

// DurableSessionAPI groups the durable application capabilities exposed to
// transport mapping and retained compatibility facades.
type DurableSessionAPI interface {
	DurableSessionExecutionAPI
	DurableSessionListingAPI
	DurableSessionProjectionAPI
	DurableSessionLifecycleAPI
}

// FactoryEventToAPI decodes one detached canonical Factory event into the
// generated OpenAPI union used at public transport boundaries.
func FactoryEventToAPI(event interfaces.FactoryEvent) (factoryapi.FactoryEvent, error) {
	var mapped factoryapi.FactoryEvent
	if err := event.Decode(&mapped); err != nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("map canonical Factory event %q to API: %w", event.Id, err)
	}
	return mapped, nil
}

// FactoryEventsToAPI maps canonical Factory events in their existing order.
func FactoryEventsToAPI(events []interfaces.FactoryEvent) ([]factoryapi.FactoryEvent, error) {
	if events == nil {
		return nil, nil
	}
	mapped := make([]factoryapi.FactoryEvent, len(events))
	for index, event := range events {
		converted, err := FactoryEventToAPI(event)
		if err != nil {
			return nil, err
		}
		mapped[index] = converted
	}
	return mapped, nil
}

// ValidateWritableNamedFactoryName enforces the public named-factory contract
// for create/import paths. The reserved default-current identifier is valid for
// readback only and must never be persisted as a customer-named factory.
func ValidateWritableNamedFactoryName(name factoryapi.FactoryName) error {
	if err := factoryconfig.ValidateNamedFactoryName(string(name)); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidNamedFactoryName, err)
	}
	if name == DefaultCurrentFactoryName {
		return fmt.Errorf("%w: %q is reserved for current-factory readback", ErrInvalidNamedFactoryName, name)
	}
	return nil
}
