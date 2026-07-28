package wire

import (
	"encoding/json"
	"fmt"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func wireCompositionRunRequestEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
	recordedAt time.Time,
	generationID string,
) (recordings.CanonicalEvent, error) {
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "wire-composition-factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "ready", "type": "PROCESSING"},
				},
			},
		},
	})
	if err != nil {
		return recordings.CanonicalEvent{}, fmt.Errorf("factory snapshot: %w", err)
	}
	payload, err := json.Marshal(factorydefinitions.RunRequestEventPayload{
		Factory:    snapshot,
		RecordedAt: recordedAt,
	})
	if err != nil {
		return recordings.CanonicalEvent{}, fmt.Errorf("run request payload: %w", err)
	}
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(id),
		Kind:        recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunRequest),
		Sequence:    sequence,
		Scope:       scope,
		FactoryTick: 0,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: generationID,
			Sequence:           sequence,
		},
		RecordedAt: recordedAt,
		Payload:    string(payload),
	}, nil
}
