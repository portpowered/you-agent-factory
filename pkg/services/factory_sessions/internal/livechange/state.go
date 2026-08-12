package livechange

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// ProjectState derives the durable live-change revision and effective Factory
// snapshot from canonical events. It intentionally ignores request/failure
// records for revision purposes and accepts legacy replacement events without
// inventing a revision for them.
func ProjectState(sessionID string, events []interfaces.FactoryEvent) factorysessions.LiveChangeSessionState {
	state := factorysessions.LiveChangeSessionState{SessionID: sessionID}
	for _, event := range events {
		switch event.Type {
		case interfaces.FactoryEventTypeInitialStructureRequest:
			var payload interfaces.InitialStructureRequestEventPayload
			if event.DecodePayload(&payload) == nil && payload.Factory != nil {
				state.Factory = payload.Factory.Clone()
			}
		case interfaces.FactoryEventTypeRunRequest:
			var payload interfaces.RunRequestEventPayload
			if event.DecodePayload(&payload) == nil && payload.Factory != nil {
				state.Factory = payload.Factory.Clone()
			}
		case interfaces.FactoryEventTypeFactoryChange:
			if !eventBelongsToSession(event, sessionID) {
				continue
			}
			var payload interfaces.FactoryChangeEventPayload
			if event.DecodePayload(&payload) != nil {
				continue
			}
			if payload.Factory != nil {
				state.Factory = payload.Factory.Clone()
			}
			if payload.NewRevision != nil {
				state.EffectiveRevision = *payload.NewRevision
				if payload.EffectiveSequence != nil {
					state.EffectiveSequence = *payload.EffectiveSequence
				} else {
					state.EffectiveSequence = event.Context.Sequence
				}
			}
		}
	}
	return state
}
