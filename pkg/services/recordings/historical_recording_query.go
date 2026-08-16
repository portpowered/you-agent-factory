package recordings

// HistoricalRecordingQuery is the Recordings-owned read-only capability for
// reconstructing one finalized recording from its published artifact.
// Callers provide only opaque recording identity; artifact bytes, storage, and
// projection implementations remain inside Recordings.
type HistoricalRecordingQuery interface {
	QueryHistoricalRecording(HistoricalRecordingQueryRequest) (HistoricalRecordingQueryResult, error)
}

// HistoricalRecordingIdentity identifies one durable recording and its
// requested Factory Session scope.
type HistoricalRecordingIdentity struct {
	RecordingID RecordingID
	Artifact    RecordingArtifactReference
	Scope       CanonicalEventScope
}

// HistoricalRecordingQueryRequest selects one immutable recording artifact.
type HistoricalRecordingQueryRequest struct {
	Recording HistoricalRecordingIdentity
}

// HistoricalDispatchWorkerSessionAssociation records the canonical event
// that associated a dispatch with a Worker Session.
type HistoricalDispatchWorkerSessionAssociation struct {
	ID              CanonicalEventID
	WorkerSessionID string
	RequestID       string
	Cursor          CanonicalEventCursor
}

// HistoricalDispatch is the detached latest lifecycle projection for one
// dispatch. The result preserves first-seen dispatch order.
type HistoricalDispatch struct {
	ID           string
	Status       FactoryDispatchStatus
	DispatchKind FactoryDispatchKind
	TransitionID string
	FirstCursor  CanonicalEventCursor
	LastCursor   CanonicalEventCursor
	Association  *HistoricalDispatchWorkerSessionAssociation
}

// HistoricalRecordingQueryResult contains detached canonical history,
// selected-tick state, recording status, and dispatch facts.
type HistoricalRecordingQueryResult struct {
	Recording  HistoricalRecordingIdentity
	Status     RecordingStatusFacts
	Events     []CanonicalEvent
	WorldState WorldStateView
	// WorkstationRequests is the selected-tick workstation read model derived
	// from the same historical world state, keeping HTTP and MCP on one owner
	// projection result.
	WorkstationRequests WorkstationFactoryWorldWorkstationRequestProjectionSlice
	Dispatches          []HistoricalDispatch
}

// HistoricalRecordingQueryErrorKind classifies durable-history outcomes
// without requiring callers to parse diagnostic strings.
type HistoricalRecordingQueryErrorKind string

const (
	HistoricalRecordingQueryErrorInvalidRequest HistoricalRecordingQueryErrorKind = "INVALID_REQUEST"
	HistoricalRecordingQueryErrorMissingHistory HistoricalRecordingQueryErrorKind = "MISSING_HISTORY"
	HistoricalRecordingQueryErrorCorruptHistory HistoricalRecordingQueryErrorKind = "CORRUPT_HISTORY"
	HistoricalRecordingQueryErrorUnavailable    HistoricalRecordingQueryErrorKind = "UNAVAILABLE"
)

// HistoricalRecordingQueryError retains only safe recording and event identity
// in its public presentation. Cause remains available for errors.Is/errors.As.
type HistoricalRecordingQueryError struct {
	Kind        HistoricalRecordingQueryErrorKind
	RecordingID RecordingID
	EventID     CanonicalEventID
	Cause       error
}

func (e *HistoricalRecordingQueryError) Error() string {
	if e == nil {
		return ""
	}
	message := "historical recording query " + string(e.Kind)
	if e.RecordingID != "" {
		message += " recording=" + string(e.RecordingID)
	}
	if e.EventID != "" {
		message += " event=" + string(e.EventID)
	}
	return message
}

func (e *HistoricalRecordingQueryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
