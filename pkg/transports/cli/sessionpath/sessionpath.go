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

// HumanApprovalsCollectionPath returns the read-only pending approval
// collection route for one factory session.
func HumanApprovalsCollectionPath(sessionID string) string {
	return fmt.Sprintf("/factory-sessions/%s/approvals", escapedSessionID(sessionID))
}

// HumanApprovalItemPath returns one pending approval route for one factory
// session.
func HumanApprovalItemPath(sessionID, approvalID string) string {
	return fmt.Sprintf("%s/%s", HumanApprovalsCollectionPath(sessionID), url.PathEscape(approvalID))
}

// FactoryEventsPath returns the canonical Factory Event stream route for one
// factory session.
func FactoryEventsPath(sessionID string) string {
	return fmt.Sprintf("/factory-sessions/%s/events", escapedSessionID(sessionID))
}

// FactoryInvocationsPath returns the compatibility invocation route for one
// already-open Factory Session.
func FactoryInvocationsPath(sessionID string) string {
	return fmt.Sprintf("/factory-sessions/%s/invocations", escapedSessionID(sessionID))
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

// TopLevelWorkerSessionInterruptPath returns the source-addressed interrupt
// route. The source ID is the only identity supplied to the server.
func TopLevelWorkerSessionInterruptPath(workerSessionID string) string {
	return TopLevelWorkerSessionDetailPath(workerSessionID) + "/interrupt"
}

// TopLevelWorkerSessionPausePath returns the exact source-addressed pause
// control route.
func TopLevelWorkerSessionPausePath(workerSessionID string) string {
	return TopLevelWorkerSessionDetailPath(workerSessionID) + "/pause"
}

// TopLevelWorkerSessionResumePath returns the exact source-addressed resume
// control route.
func TopLevelWorkerSessionResumePath(workerSessionID string) string {
	return TopLevelWorkerSessionDetailPath(workerSessionID) + "/resume"
}

// TopLevelWorkerSessionCancelPath returns the exact source-addressed cancel
// control route.
func TopLevelWorkerSessionCancelPath(workerSessionID string) string {
	return TopLevelWorkerSessionDetailPath(workerSessionID) + "/cancel"
}

// TopLevelWorkerSessionTerminatePath returns the exact source-addressed
// terminate control route.
func TopLevelWorkerSessionTerminatePath(workerSessionID string) string {
	return TopLevelWorkerSessionDetailPath(workerSessionID) + "/terminate"
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
