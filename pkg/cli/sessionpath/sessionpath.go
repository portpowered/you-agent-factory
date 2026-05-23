package sessionpath

import (
	"fmt"
	"net/url"
)

const DefaultFactorySessionID = "~default"

func ScopedPath(legacyPath string, sessionID string) string {
	if legacyPath == "/factory/~current" {
		return fmt.Sprintf("/factory-sessions/%s/factory", escapedSessionID(sessionID))
	}
	if legacyPath == "/factory/~current/editable-definition" {
		return fmt.Sprintf("/factory-sessions/%s/factory/editable-definition", escapedSessionID(sessionID))
	}
	return fmt.Sprintf("/factory-sessions/%s%s", escapedSessionID(sessionID), legacyPath)
}

func escapedSessionID(sessionID string) string {
	if sessionID == "" {
		sessionID = DefaultFactorySessionID
	}
	return url.PathEscape(sessionID)
}
