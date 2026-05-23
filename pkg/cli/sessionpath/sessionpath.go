package sessionpath

import (
	"fmt"
	"net/url"
)

const DefaultFactorySessionID = "~default"

func CurrentFactoryPath(sessionID string) string {
	return fmt.Sprintf("/factory-sessions/%s/factory", escapedSessionID(sessionID))
}

func ScopedPath(legacyPath string, sessionID string) string {
	return fmt.Sprintf("/factory-sessions/%s%s", escapedSessionID(sessionID), legacyPath)
}

func escapedSessionID(sessionID string) string {
	if sessionID == "" {
		sessionID = DefaultFactorySessionID
	}
	return url.PathEscape(sessionID)
}
