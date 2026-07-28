package factory

import (
	"encoding/json"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Runtime-owned Factory Event vocabulary published at the Factory Runtime root.
// Recordings lifecycle recorder edges consume these aliases rather than nested
// Runtime implementation packages or loose helper paths.
type (
	FactoryEvent            = interfaces.FactoryEvent
	FactoryEventContext     = interfaces.FactoryEventContext
	FactoryEventType        = interfaces.FactoryEventType
	FactoryState            = interfaces.FactoryState
	RunResponseEventPayload = interfaces.RunResponseEventPayload
	RunEventWallClock       = interfaces.RunEventWallClock
)

const (
	FactoryEventSchemaVersionV1 = interfaces.FactoryEventSchemaVersionV1
	FactoryEventTypeRunResponse = interfaces.FactoryEventTypeRunResponse
	FactoryEventTypeWorkRequest = interfaces.FactoryEventTypeWorkRequest
	FactoryStateCompleted       = interfaces.FactoryStateCompleted
)

// RunFinishedFactoryEventID is the stable identity for terminal run completion
// events emitted through the Runtime root recording vocabulary.
const RunFinishedFactoryEventID = "factory-event/run-finished"

// RunFinishedFactoryEvent constructs the terminal RUN_RESPONSE event emitted
// when a Factory Runtime instance completes. Recordings lifecycle recorder
// edges publish this shape through the Runtime root contract.
func RunFinishedFactoryEvent(startedAt, finishedAt time.Time) FactoryEvent {
	state := FactoryStateCompleted
	startedAtUTC := startedAt.UTC()
	finishedAtUTC := finishedAt.UTC()
	payload, err := json.Marshal(RunResponseEventPayload{
		State: &state,
		WallClock: &RunEventWallClock{
			StartedAt:  &startedAtUTC,
			FinishedAt: &finishedAtUTC,
		},
	})
	if err != nil {
		payload = json.RawMessage(`{}`)
	}
	return FactoryEvent{
		Id:            RunFinishedFactoryEventID,
		SchemaVersion: FactoryEventSchemaVersionV1,
		Type:          FactoryEventTypeRunResponse,
		Context: FactoryEventContext{
			EventTime: finishedAtUTC,
		},
		Payload: payload,
	}
}
