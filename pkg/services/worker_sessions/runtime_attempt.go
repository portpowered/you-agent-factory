package workersessions

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// RuntimeAttemptRequest asks Worker Sessions to open the durable observation
// window for an attempt whose admission and execution remain owned by
// Factory Runtime. ID is the Worker Session identity; AttemptID is the
// physical attempt identity written into lifecycle records. An empty
// AttemptID uses the request dispatch ID.
type RuntimeAttemptRequest struct {
	ID        string
	AttemptID string
	Execution workers.WorkstationDispatchRequest
}

// RuntimeAttempt is the durable lifecycle handle returned after the opening
// Worker Session record has committed. Complete is idempotent and records the
// terminal observation; Runtime remains authoritative for admission,
// cancellation, and the detached execution itself.
type RuntimeAttempt interface {
	Complete(context.Context, workers.WorkstationDispatchResult, error) error
}

// RuntimeAttemptService is an optional Worker Sessions capability used by
// Factory Runtime's detached execution path. Keeping it separate from
// Service lets narrow test doubles and alternate observation implementations
// continue to satisfy the stable Worker Sessions root contract.
type RuntimeAttemptService interface {
	BeginRuntimeAttempt(context.Context, RuntimeAttemptRequest) (RuntimeAttempt, error)
}
