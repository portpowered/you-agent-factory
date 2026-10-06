package support

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// GetFactoryEventsInvalidCursorErrorForSessionAt reads a typed cursor error
// from the public retained-history endpoint of one explicit Factory Session.
func GetFactoryEventsInvalidCursorErrorForSessionAt(t testing.TB, baseURL, sessionID string, cursor FactoryEventReadCursor) FactoryEventsInvalidCursorError {
	t.Helper()
	return readFactoryEventsInvalidCursorErrorFromURL(t, SessionEventsURLWithCursor(baseURL, sessionID, cursor))
}

// ProbeFactoryEventStreamRecoveryForSessionAt probes reconnect recovery through
// the public endpoint of one explicit Factory Session.
func ProbeFactoryEventStreamRecoveryForSessionAt(t testing.TB, baseURL, sessionID string, cursor FactoryEventReadCursor) factoryapi.FactorySessionEventStreamRecovery {
	t.Helper()
	return readFactoryEventStreamRecoveryFromURL(t, SessionEventsURLWithCursor(baseURL, sessionID, cursor))
}
