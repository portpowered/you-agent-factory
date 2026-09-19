package events

import interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// AddEventTypeRecorderWithReady registers a type recorder and invokes ready
// while the history lock still excludes concurrent appends. Runtime consumers
// use this boundary to arm live-tail behavior without leaving a replay-to-live
// handoff window.
func (h *FactoryEventHistory) AddEventTypeRecorderWithReady(
	recorder func(interfaces.FactoryEventType),
	ready func(),
) {
	if h == nil || recorder == nil {
		return
	}

	h.mu.Lock()
	eventTypes := make([]interfaces.FactoryEventType, len(h.events))
	for index, event := range h.events {
		eventTypes[index] = event.Type
	}
	h.eventTypeRecorders = append(h.eventTypeRecorders, recorder)
	for _, eventType := range eventTypes {
		recorder(eventType)
	}
	if ready != nil {
		ready()
	}
	h.mu.Unlock()
}
