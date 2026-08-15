// Package factoryeventprojection maps public Factory events into the canonical
// Factory world-state projection boundary.
package factoryeventprojection

import (
	"encoding/json"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ReconstructFactoryWorldState converts public events at the transport
// boundary, then delegates ordering and reduction to the Factory owner.
func ReconstructFactoryWorldState(
	reconstruct recordings.WorldStateReconstructor,
	events []factoryapi.FactoryEvent,
	selectedTick int,
) (recordings.FactoryWorldState, error) {
	canonicalEvents := make([]recordings.FactoryEvent, 0, len(events))
	for _, event := range events {
		canonicalEvent, err := canonicalFactoryEvent(event)
		if err != nil {
			return recordings.FactoryWorldState{}, err
		}
		canonicalEvents = append(canonicalEvents, canonicalEvent)
	}
	return reconstruct(canonicalEvents, selectedTick)
}

// CanonicalFactoryEvent converts one public Factory event into the canonical
// event representation used by recordings and projections. Inference and
// model response payloads translate the public provider-session projection
// into the Providers-owned opaque continuation before decoding.
func CanonicalFactoryEvent(event factoryapi.FactoryEvent) (recordings.FactoryEvent, error) {
	return canonicalFactoryEvent(event)
}

func canonicalFactoryEvent(event factoryapi.FactoryEvent) (recordings.FactoryEvent, error) {
	if event.Type == factoryapi.FactoryEventTypeInferenceResponse || event.Type == factoryapi.FactoryEventTypeModelResponse {
		envelope, err := json.Marshal(event)
		if err != nil {
			return recordings.FactoryEvent{}, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(envelope, &fields); err != nil {
			return recordings.FactoryEvent{}, err
		}
		payload, err := canonicalExecutionPayload(fields["payload"])
		if err != nil {
			return recordings.FactoryEvent{}, err
		}
		fields["payload"] = payload
		return factorydefinitions.NewFactoryEvent(fields)
	}
	return factorydefinitions.NewFactoryEvent(event)
}

func canonicalExecutionPayload(payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) == 0 {
		return payload, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, err
	}
	if _, exists := fields["continuation"]; exists {
		return payload, nil
	}
	providerSessionPayload, exists := fields["providerSession"]
	if !exists {
		return payload, nil
	}
	var session providers.SessionMetadata
	if err := json.Unmarshal(providerSessionPayload, &session); err != nil {
		return nil, err
	}
	if continuation := (&session).ContinuationRef(); continuation != nil {
		encoded, err := json.Marshal(continuation)
		if err != nil {
			return nil, err
		}
		fields["continuation"] = encoded
	}
	delete(fields, "providerSession")
	return json.Marshal(fields)
}
