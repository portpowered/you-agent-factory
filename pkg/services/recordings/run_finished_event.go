package recordings

import (
	"encoding/json"
	"time"
)

// RunFinishedFactoryEventID is the stable identity of the terminal run
// completion event in the canonical Factory Event ledger.
const RunFinishedFactoryEventID = "factory-event/run-finished"

// RunFinishedFactoryEvent constructs the terminal RUN_RESPONSE event appended
// to a recording when the Factory Runtime instance it records completes.
//
// The shape belongs to Recordings because Recordings owns the canonical
// Factory Event ledger: it is the event a replay reads back to learn that a
// run reached a terminal state, and Recordings' own replay and event-history
// paths already recognize this identity. Producers of the event -- the
// lifecycle runtime recorder, and Factory Runtime for its own emission -- take
// the definition from here rather than each restating the identity, schema
// version, and payload shape of a ledger entry.
//
// Both instants are normalized to UTC so a recording made in one host zone
// replays identically in another. A payload that somehow fails to marshal
// degrades to an empty JSON object rather than dropping the terminal event,
// because a recording that never reports completion is worse than one whose
// terminal event carries no wall-clock detail.
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
