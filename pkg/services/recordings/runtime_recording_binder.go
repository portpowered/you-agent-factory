package recordings

import (
	"context"
	"io"
)

// RecordingTargetClaim acquires exclusive destination ownership without
// changing its bytes. The returned lease outlives all reads and writes.
type RecordingTargetClaim func(context.Context, string) (io.Closer, error)

// RecordingTargetOwnership protects a live opening before it reads history.
type RecordingTargetOwnership interface {
	ClaimRecordingTarget(context.Context, string) (io.Closer, error)
}

// RuntimeRecordingStartup keeps a prepared recording read-only until the
// owning Factory Session has completed its startup transaction. Aborting
// before activation must stop without publishing terminal metadata.
type RuntimeRecordingStartup interface {
	DeferRecordingPublication()
	ActivateRecordingPublication(context.Context) error
}

// RuntimeRecordingBinder is implemented by RuntimeRecorder producers that
// bind to an already-constructed RecordingLifecycle capability once a caller
// makes one available, instead of receiving the broad Service for
// lifecycle-only use or being discovered through a caller-local type
// assertion. Binding is optional: not every RuntimeRecorder needs to bind
// (for example, a disabled or replay-only recorder), so callers type-assert
// against this published interface rather than requiring it on every
// RuntimeRecorder.
type RuntimeRecordingBinder interface {
	// BindRecordingLifecycle binds this runtime recorder to the given
	// RecordingLifecycle capability and Factory Session scope. Implementations
	// preserve RecordingLifecycle's idempotent-rebind and binding-conflict
	// semantics for repeated calls with identical or differing facts.
	BindRecordingLifecycle(RecordingLifecycle, CanonicalEventScope) error
}
