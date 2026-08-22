package workers

// DispatchCancellationReason identifies why a dispatch stopped before it
// produced a business Work result. The reason is deliberately separate from
// WorkOutcome so a superseded execution cannot be mistaken for a failure.
type DispatchCancellationReason string

const (
	DispatchCancellationReasonCanceled   DispatchCancellationReason = "CANCELED"
	DispatchCancellationReasonSuperseded DispatchCancellationReason = "SUPERSEDED"
)

// DispatchCancellation is the explicit lifecycle fact carried across
// execution, Worker Session, and Factory Runtime result boundaries.
type DispatchCancellation struct {
	Reason DispatchCancellationReason `json:"reason"`
}

func NewDispatchCancellation(reason DispatchCancellationReason) *DispatchCancellation {
	if reason != DispatchCancellationReasonSuperseded {
		reason = DispatchCancellationReasonCanceled
	}
	return &DispatchCancellation{Reason: reason}
}

func (value *DispatchCancellation) Clone() *DispatchCancellation {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
