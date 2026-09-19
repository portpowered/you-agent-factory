package events

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func (h *FactoryEventHistory) restoreSeedEventStateLocked(event interfaces.FactoryEvent) {
	if h.sessionLifecycles == nil {
		h.sessionLifecycles = make(map[string]sessionLifecycleState)
	}
	sessionID := sessionLifecycleEventSessionID(event)
	h.restoreSeedEventFlagsLocked(event)
	state := restoreSeedLifecycleState(event, h.sessionLifecycles[sessionID])
	if isSessionLifecycleEvent(event.Type) {
		h.sessionLifecycles[sessionID] = state
	}
	h.restoreSeedEventSessionIDLocked(event)
	h.restoreSeedEventSequenceLocked(event)
}

func (h *FactoryEventHistory) restoreSeedEventFlagsLocked(event interfaces.FactoryEvent) {
	switch event.Type {
	case interfaces.FactoryEventTypeInitialStructureRequest:
		h.hasInitialStructure = true
	case interfaces.FactoryEventTypeRunRequest:
		h.hasRunRequest = true
		h.runRecordedAt = interfaces.CanonicalEventTime(event.Context.EventTime)
	case interfaces.FactoryEventTypeRunResponse:
		h.hasRunResponse = true
	}
}

func restoreSeedLifecycleState(event interfaces.FactoryEvent, state sessionLifecycleState) sessionLifecycleState {
	switch event.Type {
	case interfaces.FactoryEventTypeSessionStarted:
		if !state.hasStarted {
			state.hasStarted = true
			state.startedAt = interfaces.CanonicalEventTime(event.Context.EventTime)
		}
		state.legacyEventIDs = state.legacyEventIDs || event.Id == eventIDSessionStarted
	case interfaces.FactoryEventTypeSessionCompleted:
		state.hasCompleted = true
		state.legacyEventIDs = state.legacyEventIDs || event.Id == eventIDSessionCompleted
	}
	return state
}

func isSessionLifecycleEvent(eventType interfaces.FactoryEventType) bool {
	return eventType == interfaces.FactoryEventTypeSessionStarted || eventType == interfaces.FactoryEventTypeSessionCompleted
}

func (h *FactoryEventHistory) restoreSeedEventSessionIDLocked(event interfaces.FactoryEvent) {
	if event.Context.SessionID != nil {
		if sessionID := strings.TrimSpace(*event.Context.SessionID); sessionID != "" {
			h.sessionID = sessionID
		}
	}
}

func (h *FactoryEventHistory) restoreSeedEventSequenceLocked(event interfaces.FactoryEvent) {
	if sequence := event.Context.SessionSequence; sequence != nil && *sequence >= h.nextSessionSequence {
		h.nextSessionSequence = *sequence + 1
	}
}
