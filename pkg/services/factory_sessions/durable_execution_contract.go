package factorysessions

// Durable-execution root slice freezes start, resume, control, and inspect
// vocabulary on the singular Service (via embedded ExecutionService). Peers
// consume these plain root contracts without importing nested durable-execution
// or internal/execution implementation packages:
//
//   - Start: DurableStartRequest → DurableAsyncStartResult / DurableSyncStartResult
//   - Resume: DurableResumeRequest → DurableAsyncStartResult
//   - Control: DurableControlRequest → DurableControlResult
//   - Inspect: DurableInspectResult
//
// Typed failures peers distinguish with errors.Is / errors.As:
//   - *DurableValidationError for invalid policy/source (and related start input)
//   - ErrDurableSessionNotFound for missing durable sessions
//   - *DurableResumeError for missing checkpoint / invalid resume state
//   - *DurableControlError for rejected lifecycle transitions
//
// Durable operations remain methods on the singular root Service aggregate; this
// file does not publish a separate peer-facing durable-execution interface.
// Slice-named aliases are the peer-facing durable vocabulary; nested
// internal/execution types are not the peer-facing source of truth.

// DurableStartRequest is the plain root start request for durable session
// execution (async and sync).
type DurableStartRequest = StartRequest

// DurableAsyncStartResult is the plain root async start success shape.
type DurableAsyncStartResult = AsyncStartResult

// DurableSyncStartResult is the plain root sync start success shape.
type DurableSyncStartResult = SyncStartResult

// DurableResumeRequest is the plain root resume request for interrupted durable
// sessions.
type DurableResumeRequest = ResumeSessionRequest

// DurableControlRequest is the plain root pause/resume/cancel/terminate request
// metadata for durable lifecycle control.
type DurableControlRequest = ControlRequest

// DurableControlResult is the plain root durable lifecycle-control success shape.
type DurableControlResult = LifecycleControlResult

// DurableInspectResult is the plain root durable session inspect/read projection.
type DurableInspectResult = SessionReadResult

// DurableValidationError is the typed invalid policy/source (and related start
// input) failure published on the durable-execution root slice.
type DurableValidationError = ExecutionValidationError

// DurableControlError is the typed rejected-lifecycle-transition failure
// published on the durable-execution root slice.
type DurableControlError = ControlError

// DurableResumeError is the typed missing-checkpoint / invalid-resume failure
// published on the durable-execution root slice.
type DurableResumeError = ResumeError

// DurableResumeOutcome identifies one typed restart-resume failure class on the
// durable-execution root slice.
type DurableResumeOutcome = ResumeOutcome

const (
	// DurableResumeOutcomeMissingCheckpoint reports that no persisted checkpoint
	// summary exists for the interrupted session.
	DurableResumeOutcomeMissingCheckpoint = ResumeOutcomeMissingCheckpoint
	// DurableResumeOutcomeInvalidState reports corrupted resume metadata or a
	// session that is not eligible for restart-resume reconstruction.
	DurableResumeOutcomeInvalidState = ResumeOutcomeInvalidState
	// DurableResumeOutcomeCorruptedPersistence reports unreadable or invalid
	// persisted session snapshots required for restart-resume.
	DurableResumeOutcomeCorruptedPersistence = ResumeOutcomeCorruptedPersistence
)
