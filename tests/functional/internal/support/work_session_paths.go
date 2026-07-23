package support

import (
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
	return strings.TrimSuffix(baseURL, "/") + DefaultSessionEventsAPIPath
}
