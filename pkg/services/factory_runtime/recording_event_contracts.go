package factory

import interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// Runtime-owned Factory Event vocabulary published at the Factory Runtime root
// so Runtime callers name event shapes here rather than through nested Runtime
// implementation packages or loose helper paths.
//
// These are aliases of the authored definition vocabulary, not a second event
// taxonomy. The terminal run-finished event is owned by pkg/services/recordings
// with the rest of the canonical Factory Event ledger.
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
