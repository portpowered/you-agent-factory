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

// TopLevelWorkerSessionsCollectionPath returns the process-scoped Worker
// Session observation collection addressed by stable Worker Session identity.
func TopLevelWorkerSessionsCollectionPath() string {
	return "/worker-sessions"
}

// TopLevelWorkerSessionDetailPath returns the identity detail route without a
// Factory Session or Provider Session tuple.
func TopLevelWorkerSessionDetailPath(workerSessionID string) string {
	return fmt.Sprintf("%s/%s", TopLevelWorkerSessionsCollectionPath(), url.PathEscape(workerSessionID))
}

// TopLevelWorkerSessionEventsPath returns the identity event stream route.
func TopLevelWorkerSessionEventsPath(workerSessionID string) string {
	return TopLevelWorkerSessionDetailPath(workerSessionID) + "/events"
}

// TopLevelWorkerSessionContinuePath returns the source-addressed continuation
// route. The source ID is the only identity supplied to the server.
func TopLevelWorkerSessionContinuePath(workerSessionID string) string {
	return TopLevelWorkerSessionDetailPath(workerSessionID) + "/continue"
}

// TopLevelWorkerSessionTranscriptPath returns the identity transcript route.
func TopLevelWorkerSessionTranscriptPath(workerSessionID string) string {
	return TopLevelWorkerSessionDetailPath(workerSessionID) + "/transcript"
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

// WorkerSessionsTranscriptPath returns the finished Worker Session transcript
// route for one factory session.
func WorkerSessionsTranscriptPath(sessionID string) string {
	return WorkerSessionsCollectionPath(sessionID) + "/transcript"
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
