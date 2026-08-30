package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// RunFinishedFactoryEventID is the stable identity of the terminal run
// completion event in the canonical Factory Event ledger.
const RunFinishedFactoryEventID = "factory-event/run-finished"

// RunFinishedFactoryEvent constructs the terminal RUN_RESPONSE event appended
// to a recording when the Factory Runtime instance it records completes.
//
// The shape belongs to Recordings because Recordings owns the canonical
// Factory Event ledger: this is the event a replay reads back to learn that a
// run reached a terminal state, and this package already owns every other
// Factory Event identity. Producers take the definition from here rather than
// restating the identity, schema version, and payload shape of a ledger entry.
//
// Both instants are normalized to UTC so a recording made in one host zone
// replays identically in another. A payload that somehow fails to marshal
// degrades to an empty JSON object rather than dropping the terminal event,
// because a recording that never reports completion is worse than one whose
// terminal event carries no wall-clock detail.
func RunFinishedFactoryEvent(startedAt, finishedAt time.Time) recordings.FactoryEvent {
	state := recordings.FactoryStateCompleted
	startedAtUTC := startedAt.UTC()
	finishedAtUTC := finishedAt.UTC()
	payload, err := json.Marshal(recordings.RunResponseEventPayload{
		State: &state,
		WallClock: &recordings.RunEventWallClock{
			StartedAt:  &startedAtUTC,
			FinishedAt: &finishedAtUTC,
		},
	})
	if err != nil {
		payload = json.RawMessage(`{}`)
	}
	return recordings.FactoryEvent{
		Id:            RunFinishedFactoryEventID,
		SchemaVersion: recordings.FactoryEventSchemaVersionV1,
		Type:          recordings.FactoryEventTypeRunResponse,
		Context: recordings.FactoryEventContext{
			EventTime: finishedAtUTC,
		},
		Payload: payload,
	}
}

// NewRuntimeLedger exposes the concrete event ledger through its public port.
func NewRuntimeLedger(
	topology recordings.InitialStructureSource,
	now func() time.Time,
	streamGenerationID string,
	definitions interfaces.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	history := NewFactoryEventHistory(topology, now, streamGenerationID, definitions)
	if history == nil {
		return nil
	}
	return history
}

var _ recordings.RuntimeEventLedger = (*FactoryEventHistory)(nil)
var _ recordings.CompletedFlushWatermarkReader = (*FactoryEventHistory)(nil)
var _ recordings.DispatchWorkerSessionAssociationRecorder = (*FactoryEventHistory)(nil)
var _ recordings.DispatchResultIgnoredRecorder = (*FactoryEventHistory)(nil)
var _ recordings.WorkerEventRecorder = (*FactoryEventHistory)(nil)
var _ recordings.SessionProjectionReader = (*FactoryEventHistory)(nil)

// CurrentSessionProjectionFacts returns detached event-derived facts maintained
// while canonical events are appended. It never reads the canonical event
// slice, so live session requests remain independent of history length.
func (h *FactoryEventHistory) CurrentSessionProjectionFacts() (recordings.SessionProjectionFacts, error) {
	if h == nil {
		return recordings.SessionProjectionFacts{}, nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.sessionProjectionErr != nil {
		return recordings.SessionProjectionFacts{}, fmt.Errorf("incremental session projection: %w", h.sessionProjectionErr)
	}
	if h.sessionProjection == nil {
		return recordings.SessionProjectionFacts{}, nil
	}
	return h.sessionProjection.SnapshotSessionProjectionFacts(), nil
}

func cloneExpectedArtifactTemplateContext(
	context *work.ExpectedArtifactTemplateContext,
) *work.ExpectedArtifactTemplateContext {
	return context.Clone()
}

// AppendRecordedEvent appends one already-shaped canonical domain event so
// runtime owners can bridge their events into this history without depending
// on a transport representation.
func (h *FactoryEventHistory) AppendRecordedEvent(event interfaces.FactoryEvent) {
	_, _ = h.AppendRecordedEventWithResult(event)
}

// AppendRecordedEventWithResult appends one already-shaped canonical domain
// event and returns the detached event after the history assigns its sequence.
// Live-change admission uses the returned ordering fact to publish the
// effective revision boundary without predicting a concurrent append.
func (h *FactoryEventHistory) AppendRecordedEventWithResult(event interfaces.FactoryEvent) (interfaces.FactoryEvent, error) {
	if h == nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("factory event history is unavailable")
	}
	event.Context.EventTime = interfaces.CanonicalEventTime(event.Context.EventTime)
	return h.appendEvent(event), nil
}

// AppendRecordedEventWithValidation assigns the canonical sequence and lets
// the owner validate the detached event before this history publishes it.
// The validation callback runs while the history write lock is held, so a
// rejected owner proposal cannot leave a partially appended event behind.
func (h *FactoryEventHistory) AppendRecordedEventWithValidation(
	event interfaces.FactoryEvent,
	validate func(interfaces.FactoryEvent) error,
) (interfaces.FactoryEvent, error) {
	if h == nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("factory event history is unavailable")
	}
	event.Context.EventTime = interfaces.CanonicalEventTime(event.Context.EventTime)
	return h.appendEventWithValidation(event, validate)
}

func (h *FactoryEventHistory) assignLiveChangeSessionSequenceLocked(event *interfaces.FactoryEvent) {
	if h == nil || event == nil || event.Context.SessionID == nil ||
		strings.TrimSpace(*event.Context.SessionID) == "" ||
		event.Context.SessionSequence != nil || !isLiveChangeEvent(event.Type) {
		return
	}
	if h.nextSessionSequence == 0 {
		for _, existing := range h.events {
			if existing.Context.SessionID == nil || existing.Context.SessionSequence == nil ||
				strings.TrimSpace(*existing.Context.SessionID) != strings.TrimSpace(*event.Context.SessionID) {
				continue
			}
			if next := *existing.Context.SessionSequence + 1; next > h.nextSessionSequence {
				h.nextSessionSequence = next
			}
		}
	}
	sequence := h.nextSessionSequence
	h.nextSessionSequence++
	event.Context.SessionSequence = &sequence
}

func isLiveChangeEvent(eventType interfaces.FactoryEventType) bool {
	switch eventType {
	case interfaces.FactoryEventTypeFactoryChangeRequest,
		interfaces.FactoryEventTypeFactoryChange,
		interfaces.FactoryEventTypeFactoryChangeFailed:
		return true
	default:
		return false
	}
}

func enrichFactoryChangeSequence(event interfaces.FactoryEvent) interfaces.FactoryEvent {
	if event.Type != interfaces.FactoryEventTypeFactoryChange {
		return event
	}
	var payload interfaces.FactoryChangeEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.ChangeID == "" {
		return event
	}
	sequence := event.Context.Sequence
	payload.EffectiveSequence = &sequence
	encoded, err := json.Marshal(payload)
	if err != nil {
		return event
	}
	event.Payload = encoded
	return event
}
