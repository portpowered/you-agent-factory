package workers

import (
	"context"
	"time"
)

// ObservationSink receives detached execution observations during one Execute
// call. Wire injects the exact sink; Workers does not depend on Recordings
// implementation types.
type ObservationSink func(context.Context, ExecutionObservation) error

// ExecutionObservationKind identifies one Workers-owned observation category.
type ExecutionObservationKind string

const (
	ExecutionObservationKindStarted    ExecutionObservationKind = "STARTED"
	ExecutionObservationKindProgress   ExecutionObservationKind = "PROGRESS"
	ExecutionObservationKindCompleted  ExecutionObservationKind = "COMPLETED"
	ExecutionObservationKindFailed     ExecutionObservationKind = "FAILED"
	ExecutionObservationKindCanceled   ExecutionObservationKind = "CANCELED"
)

// ExecutionObservation carries safe, detached progress facts for one attempt.
type ExecutionObservation struct {
	Correlation ExecutionCorrelation
	Sequence    int64
	Kind        ExecutionObservationKind
	Timestamp   time.Time
	Phase       string
	Detail      string
	Metadata    map[string]string
}

// Clone returns a detached observation copy.
func (observation ExecutionObservation) Clone() ExecutionObservation {
	clone := observation
	clone.Metadata = cloneStringMap(observation.Metadata)
	return clone
}
