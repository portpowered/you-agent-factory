package responseevents

// Phase identifies the lifecycle position of one FactoryResponseEvent within its kind.
type Phase string

const (
	PhaseStarted   Phase = "STARTED"
	PhaseDelta     Phase = "DELTA"
	PhaseUpdated   Phase = "UPDATED"
	PhaseCompleted Phase = "COMPLETED"
	PhaseFailed    Phase = "FAILED"
	PhaseCanceled  Phase = "CANCELED"
)
