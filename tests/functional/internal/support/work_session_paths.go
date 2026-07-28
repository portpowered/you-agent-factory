package support

import (
	"net/url"
	"strconv"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// DefaultSessionWorkAPIPrefix is the HTTP prefix for work routes on the default factory session.
const DefaultSessionWorkAPIPrefix = "/factory-sessions/" + factorysessions.DefaultSessionID

// DefaultSessionEventsAPIPath is the canonical event stream for the default live factory session.
const DefaultSessionEventsAPIPath = DefaultSessionWorkAPIPrefix + "/events"

// DefaultSessionWorkPath scopes legacy work-relative paths to the default factory session.
func DefaultSessionWorkPath(path string) string {
	switch {
	case path == "/work" || strings.HasPrefix(path, "/work?"):
		return DefaultSessionWorkAPIPrefix + path
	case strings.HasPrefix(path, "/work/"):
		return DefaultSessionWorkAPIPrefix + path
	case strings.HasPrefix(path, "/work-requests/"):
		return DefaultSessionWorkAPIPrefix + path
	default:
		return path
	}
}

// DefaultSessionWorkURL joins baseURL with a default-session-scoped work path.
func DefaultSessionWorkURL(baseURL, path string) string {
	return strings.TrimSuffix(baseURL, "/") + DefaultSessionWorkPath(path)
}

// DefaultSessionEventsURL joins baseURL with the canonical default-session event stream.
func DefaultSessionEventsURL(baseURL string) string {
	return SessionEventsURL(baseURL, factorysessions.DefaultSessionID)
}

// SessionEventsURL joins baseURL with the canonical event stream for one Factory Session.
func SessionEventsURL(baseURL, sessionID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/events"
}

// SessionResponseEventsURL joins baseURL with the canonical Response Event SSE
// stream for one Factory Session.
func SessionResponseEventsURL(baseURL, sessionID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/response-events"
}

// SessionResponseEventsURLWithAfterSequence joins baseURL with one session-scoped
// Response Event stream that resumes after an acknowledged response sequence.
func SessionResponseEventsURLWithAfterSequence(baseURL, sessionID string, afterSequence int64) string {
	endpoint := SessionResponseEventsURL(baseURL, sessionID)
	if afterSequence > 0 {
		params := url.Values{}
		params.Set("after_sequence", strconv.FormatInt(afterSequence, 10))
		endpoint += "?" + params.Encode()
	}
	return endpoint
}

// SessionEventsURLWithCursor joins baseURL with one session-scoped Factory Event
// stream endpoint that resumes after an acknowledged reconnect cursor.
func SessionEventsURLWithCursor(baseURL, sessionID string, cursor FactoryEventReadCursor) string {
	endpoint := SessionEventsURL(baseURL, sessionID)
	params := url.Values{}
	if cursor.AfterEventID != "" {
		params.Set("after_event_id", cursor.AfterEventID)
	}
	if cursor.AfterSequence != nil {
		params.Set("after_sequence", strconv.Itoa(*cursor.AfterSequence))
	}
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint
}
