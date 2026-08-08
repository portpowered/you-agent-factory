package sessionpath

import (
	"fmt"
	"net/url"
)

const DefaultFactorySessionID = "~default"

func CurrentFactoryPath(sessionID string) string {
	return fmt.Sprintf("/factory-sessions/%s/factory", escapedSessionID(sessionID))
}

// WorkCollectionPath returns the work collection route for one factory session.
func WorkCollectionPath(sessionID string) string {
	return fmt.Sprintf("/factory-sessions/%s/work", escapedSessionID(sessionID))
}

// FactoryEventsPath returns the canonical Factory Event stream route for one
// factory session.
func FactoryEventsPath(sessionID string) string {
	return fmt.Sprintf("/factory-sessions/%s/events", escapedSessionID(sessionID))
}

// WorkerSessionsCollectionPath returns the Worker Sessions observation route
// for one factory session.
func WorkerSessionsCollectionPath(sessionID string) string {
	return fmt.Sprintf("/factory-sessions/%s/worker-sessions", escapedSessionID(sessionID))
}

// WorkerSessionsDetailPath returns the exact Worker Session observation route
// for one factory session.
func WorkerSessionsDetailPath(sessionID string) string {
	return WorkerSessionsCollectionPath(sessionID) + "/detail"
}

// WorkerSessionsEventsPath returns the retained/live Worker Session event
// stream route for one factory session.
func WorkerSessionsEventsPath(sessionID string) string {
	return WorkerSessionsCollectionPath(sessionID) + "/events"
}

// WorkItemPath returns the route for one work item in a factory session.
func WorkItemPath(sessionID, workID string) string {
	return fmt.Sprintf("%s/%s", WorkCollectionPath(sessionID), url.PathEscape(workID))
}

// WorkMovePath returns the move route for one work item in a factory session.
func WorkMovePath(sessionID, workID string) string {
	return WorkItemPath(sessionID, workID) + "/move"
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
