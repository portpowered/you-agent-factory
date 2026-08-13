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
var _ recordings.WorkerEventRecorder = (*FactoryEventHistory)(nil)

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
