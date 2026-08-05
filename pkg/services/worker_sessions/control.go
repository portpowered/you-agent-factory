package workersessions

// ControlAction identifies the lifecycle action requested for one Worker
// Session. The service exposes one method per action so callers cannot submit
// an invalid action string; Action is retained in ControlResult as detached,
// observable evidence of what the service attempted.
type ControlAction string

const (
	ControlActionPause     ControlAction = "PAUSE"
	ControlActionResume    ControlAction = "RESUME"
	ControlActionCancel    ControlAction = "CANCEL"
	ControlActionTerminate ControlAction = "TERMINATE"
)

// ControlOutcome classifies the result of one exact Worker Session control.
// A failed Workers boundary call is returned both as OutcomeFailed and as the
// operation error so a Factory coordinator can retain per-child evidence while
// still distinguishing it from an idempotent or unsupported result.
type ControlOutcome string

const (
	ControlOutcomeApplied     ControlOutcome = "APPLIED"
	ControlOutcomeNoop        ControlOutcome = "NOOP"
	ControlOutcomeUnsupported ControlOutcome = "UNSUPPORTED"
	ControlOutcomeFailed      ControlOutcome = "FAILED"
)

// ControlRequest identifies exactly one stable Worker Session. A supervised
// session owns at most one immutable dispatch attempt at a time; the returned
// ControlResult carries that dispatch identity without requiring callers to
// reconstruct or guess it from a Worker execution request.
type ControlRequest struct {
	ID string
}

// Validate reports whether req identifies one stable Worker Session.
func (req ControlRequest) Validate() error {
	if !validSessionID(req.ID) {
		return ErrInvalidSessionID
	}
	return nil
}

// ControlResult is detached evidence for one requested control. Session is a
// snapshot at the method's linearization point or, for Terminate, after the
// associated attempt has joined. DispatchID is empty only when no dispatch was
// ever admitted for the session.
type ControlResult struct {
	Session    Session
	Action     ControlAction
	Outcome    ControlOutcome
	DispatchID string
}
