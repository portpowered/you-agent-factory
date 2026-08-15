package apisurface

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ErrInvalidNamedFactoryName retains the public compatibility identity of the
// Factory Definitions contract.
var ErrInvalidNamedFactoryName = interfaces.ErrInvalidNamedFactoryName

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
	if payload, err := factoryEventPayloadToAPI(event); err != nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("map canonical Factory event %q payload to API: %w", event.Id, err)
	} else if payload != nil {
		event.Payload = payload
	}
	var mapped factoryapi.FactoryEvent
	if err := event.Decode(&mapped); err != nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("map canonical Factory event %q to API: %w", event.Id, err)
	}
	return mapped, nil
}

// factoryEventPayloadToAPI translates the Worker-owned opaque continuation
// carried by canonical execution events into the public event projection. The
// public event contract intentionally retains only provider-session identity;
// Workers never regains that metadata shape.
func factoryEventPayloadToAPI(event interfaces.FactoryEvent) ([]byte, error) {
	switch event.Type {
	case interfaces.FactoryEventTypeInferenceResponse, interfaces.FactoryEventTypeModelResponse:
	default:
		return nil, nil
	}
	if len(event.Payload) == 0 {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &fields); err != nil {
		return nil, err
	}
	continuationPayload, ok := fields["continuation"]
	if !ok {
		return nil, nil
	}
	var continuation providers.ContinuationRef
	if err := json.Unmarshal(continuationPayload, &continuation); err != nil {
		return nil, err
	}
	if session := providers.SessionMetadataFromContinuation(&continuation); session != nil {
		encoded, err := json.Marshal(session)
		if err != nil {
			return nil, err
		}
		fields["providerSession"] = encoded
	}
	delete(fields, "continuation")
	return json.Marshal(fields)
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
