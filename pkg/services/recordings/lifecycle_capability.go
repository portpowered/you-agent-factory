package recordings

import (
	recordingcontracts "github.com/portpowered/infinite-you/pkg/services/recordings/internal/contracts"
	"time"
)

// CompletedFlushWatermarkReader is the narrow Recordings capability used by
// read projections that need to distinguish a live canonical fact from one
// covered by completed durable recording storage.
//
// The watermark is scoped by stream generation because canonical sequence
// numbers are only comparable within one generation. The returned cursor is a
// detached value. ok is false until a successful recording flush has covered
// at least one event in the requested generation.
type CompletedFlushWatermarkReader = recordingcontracts.CompletedFlushWatermarkReader

// SessionLifecycleCompletionPhaser is an optional runtime-ledger capability
// for placing durable flushes between terminal result and close events.
type SessionLifecycleCompletionPhaser = recordingcontracts.SessionLifecycleCompletionPhaser

// DeferredSessionCompletionPublisher is an optional runtime-ledger capability
// for delaying transport completion until SESSION_COMPLETED is durable.
type DeferredSessionCompletionPublisher = recordingcontracts.DeferredSessionCompletionPublisher

// RecordingLifecycle is a narrow, Recordings-owned capability for peers that
// only need to begin or bind a canonical recording, append ordered Factory
// Events, record failures, flush durable positions, stop periodic work, and
// finish with a detached terminal outcome. It deliberately excludes replay,
// artifact export, projection query, and event subscription behavior so
// peers can fake it without implementing the rest of Service.
//
// Every identity, request, result, status, and failure type used by this
// capability is defined directly in this file rather than aliased from
// recordings/internal/contracts.
type RecordingLifecycle interface {
	// Begin selects and binds one recording lifecycle. A disabled request is
	// inert: it returns a zero result and does not select a target, allocate
	// an identity, or mutate state.
	Begin(BeginRecordingRequest) (RecordingLifecycleResult, error)
	// Bind explicitly binds one recording identity to an artifact and scope.
	// Repeating an explicit RecordingID with identical Artifact and Scope is
	// idempotent and returns the existing status unchanged. Reusing it with
	// different binding facts returns a typed binding-conflict error.
	Bind(BindLifecycleRequest) (RecordingLifecycleResult, error)
	// AppendEvent appends one canonical Factory Event to the bound recording
	// in validated order. An invalid event is rejected without mutation.
	AppendEvent(AppendLifecycleEventRequest) (RecordingLifecycleResult, error)
	// RecordFailure appends one detached failure fact to the bound recording.
	RecordFailure(RecordLifecycleFailureRequest) (RecordingLifecycleResult, error)
	// Flush persists the last accepted position durably and reports the
	// FlushedThrough cursor.
	Flush(FlushLifecycleRequest) (RecordingLifecycleResult, error)
	// Stop cancels and joins active periodic lifecycle work without
	// finalizing the recording or performing a final flush.
	Stop(StopLifecycleRequest) error
	// Finish stops periodic work, applies terminal metadata once, attempts a
	// final flush, and returns a detached finalized or failed terminal
	// status. Repeated Finish calls return the first terminal outcome.
	Finish(FinishLifecycleRequest) (RecordingLifecycleResult, error)
	// Status reports the current detached status of the bound recording.
	Status(LifecycleStatusRequest) (RecordingLifecycleResult, error)
}

// LifecycleRecordingID is the Recordings-owned identity of one bound
// recording, published for peers that only consume RecordingLifecycle.
type LifecycleRecordingID string

// LifecycleArtifactReference is an opaque portable artifact reference. It
// does not grant filesystem authority or expose a writer, transaction, or
// temporary storage location.
type LifecycleArtifactReference string

// LifecycleScope identifies the Factory Session whose canonical history
// contains a recording's events. An empty FactorySessionID represents
// factory-wide scope.
type LifecycleScope struct {
	FactorySessionID string
}

// LifecycleState identifies the observable state of one recording.
type LifecycleState string

const (
	LifecycleStateActive    LifecycleState = "ACTIVE"
	LifecycleStateFinalized LifecycleState = "FINALIZED"
	LifecycleStateFailed    LifecycleState = "FAILED"
)

// LifecycleEventCursor is a portable reconnect position in global canonical
// order. StreamGenerationID distinguishes histories whose numeric sequences
// may overlap.
type LifecycleEventCursor struct {
	StreamGenerationID string
	Sequence           int64
}

// LifecycleFailure is a detached failure fact accumulated by a recording.
// Code and Message are values rather than a retained implementation error.
type LifecycleFailure struct {
	Code       string
	Message    string
	RecordedAt time.Time
}

// LifecycleStatus is a detached snapshot of recording lifecycle state.
// Implementations must return independent cursor, time, and Failures values
// so callers cannot mutate lifecycle state through the returned status.
type LifecycleStatus struct {
	RecordingID    LifecycleRecordingID
	Artifact       LifecycleArtifactReference
	Scope          LifecycleScope
	State          LifecycleState
	AcceptedEvents int
	LastEvent      *LifecycleEventCursor
	FlushedThrough *LifecycleEventCursor
	Failures       []LifecycleFailure
	FinalizedAt    *time.Time
}

// RecordingLifecycleResult is the published detached outcome of every
// RecordingLifecycle operation that reports status.
type RecordingLifecycleResult struct {
	Status LifecycleStatus
}

// BeginRecordingRequest selects and binds one recording lifecycle. Disabled
// requests are intentionally inert: they do not select a target or allocate
// an identity. Artifact takes precedence over HomeDir/CanonicalSessionID/
// ReportedSessionID
// generation; Artifact does not prescribe a file path, datastore key,
// writer, or persistence implementation.
type BeginRecordingRequest struct {
	Enabled            bool
	RecordingID        LifecycleRecordingID
	Scope              LifecycleScope
	Artifact           LifecycleArtifactReference
	HomeDir            string
	CanonicalSessionID string
	ReportedSessionID  string
	FlushInterval      time.Duration
}

// BindLifecycleRequest identifies the Factory Session and opaque artifact
// reference for one recording.
type BindLifecycleRequest struct {
	RecordingID LifecycleRecordingID
	Artifact    LifecycleArtifactReference
	Scope       LifecycleScope
}

// LifecycleEvent is a detached, Recordings-owned canonical fact. Payload is
// immutable JSON text rather than a shared byte slice, and no
// implementation, datastore, runtime, or transport handle can cross this
// boundary.
type LifecycleEvent struct {
	ID            string
	Sequence      int64
	FactoryTick   int
	Scope         LifecycleScope
	Cursor        LifecycleEventCursor
	RecordedAt    time.Time
	Kind          string
	Payload       string
	SourceContext string
}

// AppendLifecycleEventRequest associates one canonical Factory Event with a
// bound recording.
type AppendLifecycleEventRequest struct {
	RecordingID      LifecycleRecordingID
	Event            LifecycleEvent
	SecretProvenance []RecordingSecret
}

// RecordLifecycleFailureRequest appends one detached failure fact. Cause is
// preserved only for typed/standard error matching on the returned error; it
// never crosses the detached status boundary.
type RecordLifecycleFailureRequest struct {
	RecordingID LifecycleRecordingID
	Failure     LifecycleFailure
	Cause       error
}

// FlushLifecycleRequest is the plain flush request for one bound recording.
type FlushLifecycleRequest struct {
	RecordingID LifecycleRecordingID
}

// StopLifecycleRequest identifies active periodic lifecycle work to cancel
// and join. It does not finalize the recording or perform a final flush.
type StopLifecycleRequest struct {
	RecordingID LifecycleRecordingID
}

// FinishLifecycleRequest is the plain finish request for one bound
// recording.
type FinishLifecycleRequest struct {
	RecordingID LifecycleRecordingID
	FinishedAt  time.Time
}

// LifecycleStatusRequest is the plain status query for one bound recording.
type LifecycleStatusRequest struct {
	RecordingID LifecycleRecordingID
}

// LifecycleErrorKind distinguishes typed RecordingLifecycle outcomes so
// peers can branch without depending on recordings/internal/contracts
// sentinel errors.
type LifecycleErrorKind string

const (
	LifecycleErrorInvalidTarget           LifecycleErrorKind = "INVALID_TARGET"
	LifecycleErrorInvalidScope            LifecycleErrorKind = "INVALID_SCOPE"
	LifecycleErrorBindingConflict         LifecycleErrorKind = "BINDING_CONFLICT"
	LifecycleErrorInvalidEvent            LifecycleErrorKind = "INVALID_EVENT"
	LifecycleErrorInvalidFailure          LifecycleErrorKind = "INVALID_FAILURE"
	LifecycleErrorTerminal                LifecycleErrorKind = "TERMINAL"
	LifecycleErrorInvalidTerminalMetadata LifecycleErrorKind = "INVALID_TERMINAL_METADATA"
	LifecycleErrorWriteFailed             LifecycleErrorKind = "WRITE_FAILED"
)

// LifecycleError is a typed RecordingLifecycle failure peers can branch on
// via Kind or unwrap via Cause for standard errors.Is/errors.As matching.
type LifecycleError struct {
	Kind    LifecycleErrorKind
	Message string
	Cause   error
}

func (e *LifecycleError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *LifecycleError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
