package service

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Infer from the canonical decoder's already-parsed stream, avoiding a second
// full artifact decode. The caller still validates every event's scope, order,
// schema and integrity before exposing any facts.
func recordedEventScope(identity recordings.HistoricalRecordingIdentity, events []factorydefinitions.FactoryEvent) (recordings.HistoricalRecordingIdentity, error) {
	var session string
	if len(events) > 0 && events[0].Context.SessionID != nil {
		session = *events[0].Context.SessionID
	}
	return recordedScope(identity, session)
}

func recordedScope(identity recordings.HistoricalRecordingIdentity, session string) (recordings.HistoricalRecordingIdentity, error) {
	if session == "" {
		return identity, historicalQueryError(recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", nil)
	}
	identity.Scope.FactorySessionID = session
	return identity, nil
}
