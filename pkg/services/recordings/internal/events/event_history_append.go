package events

import (
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// RecordWorkStateChange records a canonical marking relocation for operator or
// cascade recovery paths.
func (h *FactoryEventHistory) RecordWorkStateChange(tick int, record work.WorkStateChangeRecord, eventTime time.Time) {
	if h == nil || record.WorkID == "" || record.Source == "" {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	workTypeName := strings.TrimSpace(record.WorkTypeName)
	if workTypeName == "" {
		workTypeName = record.WorkTypeID
	}
	context := interfaces.FactoryEventContext{
		Tick:      tick,
		EventTime: eventTime,
		SessionID: stringPtrIfNotEmpty(record.SessionID),
		RequestID: stringPtrIfNotEmpty(record.RequestID),
		WorkIDs:   stringSlicePtr([]string{record.WorkID}),
	}
	if context.SessionID != nil {
		sessionSequence := h.allocateSessionLifecycleSequence()
		context.SessionSequence = &sessionSequence
	}
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeWorkStateChange,
		fmt.Sprintf("%s/%s/%d", eventIDWorkStateChangePrefix, record.WorkID, tick),
		context,
		interfaces.WorkStateChangeEventPayload{
			WorkID:        record.WorkID,
			WorkTypeName:  workTypeName,
			FromState:     record.FromState,
			ToState:       record.ToState,
			FromPlaceID:   record.FromPlaceID,
			ToPlaceID:     record.ToPlaceID,
			Source:        record.Source,
			TriggerWorkID: stringPtrIfNotEmpty(record.TriggerWorkID),
			Reason:        stringPtrIfNotEmpty(record.Reason),
		},
	))
}

// RecordFactoryStateChange records a runtime lifecycle transition.
func (h *FactoryEventHistory) RecordFactoryStateChange(tick int, previous interfaces.FactoryState, next interfaces.FactoryState, reason string, eventTime time.Time) {
	if h == nil || previous == next {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	nextState := next
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeFactoryStateResponse,
		fmt.Sprintf("%s/%d/%s", eventIDStateChangePrefix, tick, next),
		interfaces.FactoryEventContext{Tick: tick, EventTime: eventTime},
		interfaces.FactoryStateResponseEventPayload{
			PreviousState: &previous,
			State:         nextState,
			Reason:        stringPtrIfNotEmpty(reason),
		},
	))
}

func (h *FactoryEventHistory) appendEvent(event interfaces.FactoryEvent) interfaces.FactoryEvent {
	appended, _ := h.appendEventWithValidation(event, nil)
	return appended
}

func (h *FactoryEventHistory) appendEventWithValidation(
	event interfaces.FactoryEvent,
	validate func(interfaces.FactoryEvent) error,
) (interfaces.FactoryEvent, error) {
	if h == nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("factory event history is unavailable")
	}
	h.mu.Lock()
	event.SchemaVersion = interfaces.FactoryEventSchemaVersionV1
	event.Context.Sequence = len(h.events)
	h.assignLiveChangeSessionSequenceLocked(&event)
	event = enrichFactoryChangeSequence(event)
	if validate != nil {
		if err := validate(event.Clone()); err != nil {
			h.mu.Unlock()
			return interfaces.FactoryEvent{}, err
		}
	}
	h.events = append(h.events, event)
	streams := make([]*eventHistorySubscription, 0, len(h.streams))
	for _, stream := range h.streams {
		streams = append(streams, stream)
	}
	recorders := append([]func(interfaces.FactoryEvent){}, h.recorders...)
	eventTypeRecorders := append([]func(interfaces.FactoryEventType){}, h.eventTypeRecorders...)
	for _, stream := range streams {
		if stream.dispatchID != "" && !factoryEventBelongsToDispatch(event, stream.dispatchID) {
			continue
		}
		if !stream.offer(event.Clone()) {
			stream.signalOverflow()
		}
	}
	h.mu.Unlock()

	for _, recorder := range recorders {
		recorder(event.Clone())
	}
	for _, recorder := range eventTypeRecorders {
		recorder(event.Type)
	}
	return event.Clone(), nil
}
