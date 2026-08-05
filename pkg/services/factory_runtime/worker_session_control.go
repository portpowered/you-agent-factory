package factory

// WorkerSessionControlAction identifies the lifecycle control fanned out from
// one captured Factory turn to its associated Worker Sessions.
type WorkerSessionControlAction string

const (
	WorkerSessionControlActionPause     WorkerSessionControlAction = "PAUSE"
	WorkerSessionControlActionResume    WorkerSessionControlAction = "RESUME"
	WorkerSessionControlActionCancel    WorkerSessionControlAction = "CANCEL"
	WorkerSessionControlActionTerminate WorkerSessionControlAction = "TERMINATE"
)

// WorkerSessionControlChildOutcome is the detached, per-Worker-Session
// outcome of a Factory turn control. It deliberately mirrors only the
// observable Worker Sessions control vocabulary; Factory Runtime does not
// manufacture a Worker Session state or terminal result.
type WorkerSessionControlChildOutcome string

const (
	WorkerSessionControlChildOutcomeApplied     WorkerSessionControlChildOutcome = "APPLIED"
	WorkerSessionControlChildOutcomeNoOp        WorkerSessionControlChildOutcome = "NOOP"
	WorkerSessionControlChildOutcomeUnsupported WorkerSessionControlChildOutcome = "UNSUPPORTED"
	WorkerSessionControlChildOutcomeFailed      WorkerSessionControlChildOutcome = "FAILED"
)

// WorkerSessionControlAggregateOutcome summarizes all attempted children. An
// empty target set is a NoOp. Partial means at least two non-failed child
// outcomes occurred, so callers can distinguish an all-unsupported control
// from a mixed application.
type WorkerSessionControlAggregateOutcome string

const (
	WorkerSessionControlAggregateOutcomeApplied     WorkerSessionControlAggregateOutcome = "APPLIED"
	WorkerSessionControlAggregateOutcomeNoOp        WorkerSessionControlAggregateOutcome = "NOOP"
	WorkerSessionControlAggregateOutcomeUnsupported WorkerSessionControlAggregateOutcome = "UNSUPPORTED"
	WorkerSessionControlAggregateOutcomePartial     WorkerSessionControlAggregateOutcome = "PARTIAL"
	WorkerSessionControlAggregateOutcomeFailed      WorkerSessionControlAggregateOutcome = "FAILED"
)

// WorkerSessionControlChildResult is detached evidence for one exact Worker
// Session selected from the captured Factory turn's canonical associations.
// DispatchID is present when Worker Sessions has an admitted dispatch to
// report; Factory Runtime never infers or rewrites it.
type WorkerSessionControlChildResult struct {
	WorkerSessionID string
	DispatchID      string
	Outcome         WorkerSessionControlChildOutcome
}

// WorkerSessionControlResult is the deterministic result of fanning one
// captured Factory turn control out to every selected Worker Session.
type WorkerSessionControlResult struct {
	TurnID   string
	Action   WorkerSessionControlAction
	Outcome  WorkerSessionControlAggregateOutcome
	Children []WorkerSessionControlChildResult
}
