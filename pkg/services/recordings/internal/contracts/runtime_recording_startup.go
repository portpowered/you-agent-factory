package contracts

import (
	"context"
	"io"
)

// RecordingTargetOwnership protects a live opening before it reads history.
type RecordingTargetOwnership interface {
	ClaimRecordingTarget(context.Context, string) (io.Closer, error)
}

// RecordingTargetValidator is an optional lease capability for checking that
// a retained input has not changed while startup read and reconstructed it.
// It detects changes at the check; it is not an atomic publication operation.
type RecordingTargetValidator interface {
	Validate() error
}

// RuntimeRecordingStartup keeps a prepared recording read-only until the
// owning Factory Session has completed its startup transaction. Aborting
// before activation must stop without publishing terminal metadata.
type RuntimeRecordingStartup interface {
	DeferRecordingPublication()
	ActivateRecordingPublication(context.Context) error
}
