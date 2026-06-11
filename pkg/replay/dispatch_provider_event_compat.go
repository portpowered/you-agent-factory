package replay

import (
	"encoding/json"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	legacyDispatchRunnerIDField                 = "runnerId"
	legacyDispatchRunnerSelectionSourceField    = "runnerSelectionSource"
	dispatchProviderModelProviderField          = "modelProvider"
	dispatchProviderModelProviderSelectionField = "modelProviderSelectionSource"
)

func normalizeDispatchProviderEvents(events []factoryapi.FactoryEvent) error {
	for index := range events {
		if err := normalizeDispatchProviderEvent(&events[index]); err != nil {
			return err
		}
	}
	return nil
}

func normalizeDispatchProviderEvent(event *factoryapi.FactoryEvent) error {
	if event == nil {
		return nil
	}
	switch event.Type {
	case factoryapi.FactoryEventTypeDispatchRequest:
		return normalizeDispatchRequestProviderMetadata(event)
	case factoryapi.FactoryEventTypeDispatchQueued:
		return normalizeDispatchQueuedProviderMetadata(event)
	default:
		return nil
	}
}

func normalizeDispatchRequestProviderMetadata(event *factoryapi.FactoryEvent) error {
	payloadJSON, err := event.Payload.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal dispatch request payload for event %q: %w", event.Id, err)
	}
	var raw struct {
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(payloadJSON, &raw); err != nil {
		return fmt.Errorf("decode dispatch request payload for event %q: %w", event.Id, err)
	}
	if raw.Metadata == nil {
		return nil
	}
	if err := normalizeLegacyDispatchProviderFields(raw.Metadata); err != nil {
		return fmt.Errorf("normalize dispatch request metadata for event %q: %w", event.Id, err)
	}
	return rewriteDispatchRequestPayload(event, raw.Metadata)
}

func normalizeDispatchQueuedProviderMetadata(event *factoryapi.FactoryEvent) error {
	payloadJSON, err := event.Payload.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal dispatch queued payload for event %q: %w", event.Id, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(payloadJSON, &raw); err != nil {
		return fmt.Errorf("decode dispatch queued payload for event %q: %w", event.Id, err)
	}
	if err := normalizeLegacyDispatchProviderFields(raw); err != nil {
		return fmt.Errorf("normalize dispatch queued payload for event %q: %w", event.Id, err)
	}
	return rewriteDispatchQueuedPayload(event, raw)
}

func normalizeLegacyDispatchProviderFields(fields map[string]any) error {
	if fields == nil {
		return nil
	}
	if _, hasModelProvider := fields[dispatchProviderModelProviderField]; hasModelProvider {
		delete(fields, legacyDispatchRunnerIDField)
		delete(fields, legacyDispatchRunnerSelectionSourceField)
		return nil
	}
	legacyRunnerID, hasRunnerID := stringFieldValue(fields, legacyDispatchRunnerIDField)
	if !hasRunnerID {
		return nil
	}
	public, err := interfaces.PublicModelProviderFromLegacyRunnerID(legacyRunnerID)
	if err != nil {
		return err
	}
	fields[dispatchProviderModelProviderField] = string(public)
	delete(fields, legacyDispatchRunnerIDField)

	if legacySource, ok := stringFieldValue(fields, legacyDispatchRunnerSelectionSourceField); ok {
		publicSource := interfaces.PublicModelProviderSelectionSourceFromLegacyRunnerSelectionSource(legacySource)
		fields[dispatchProviderModelProviderSelectionField] = string(publicSource)
		delete(fields, legacyDispatchRunnerSelectionSourceField)
	}
	return nil
}

func rewriteDispatchRequestPayload(event *factoryapi.FactoryEvent, metadata map[string]any) error {
	payload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		return fmt.Errorf("decode dispatch request payload for event %q: %w", event.Id, err)
	}
	if len(metadata) == 0 {
		payload.Metadata = nil
	} else {
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("encode dispatch request metadata for event %q: %w", event.Id, err)
		}
		var normalized factoryapi.DispatchRequestEventMetadata
		if err := json.Unmarshal(encoded, &normalized); err != nil {
			return fmt.Errorf("decode normalized dispatch request metadata for event %q: %w", event.Id, err)
		}
		payload.Metadata = &normalized
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		return fmt.Errorf("encode dispatch request payload for event %q: %w", event.Id, err)
	}
	event.Payload = union
	return nil
}

func rewriteDispatchQueuedPayload(event *factoryapi.FactoryEvent, raw map[string]any) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("encode dispatch queued payload for event %q: %w", event.Id, err)
	}
	var payload factoryapi.DispatchQueuedEventPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return fmt.Errorf("decode normalized dispatch queued payload for event %q: %w", event.Id, err)
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchQueuedEventPayload(payload); err != nil {
		return fmt.Errorf("encode dispatch queued payload for event %q: %w", event.Id, err)
	}
	event.Payload = union
	return nil
}

func stringFieldValue(fields map[string]any, key string) (string, bool) {
	value, ok := fields[key]
	if !ok || value == nil {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	return text, text != ""
}
