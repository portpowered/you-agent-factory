package sessionpath

import (
	"fmt"
	"net/url"
)

const DefaultFactorySessionID = "~default"

func ScopedPath(legacyPath string, sessionID string) string {
	if sessionID == "" || sessionID == DefaultFactorySessionID {
		return legacyPath
	}

	return fmt.Sprintf("/factories/%s%s", url.PathEscape(sessionID), legacyPath)
}
