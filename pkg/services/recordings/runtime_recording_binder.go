package recordings

import (
	"context"
	"io"

	recordingcontracts "github.com/portpowered/infinite-you/pkg/services/recordings/internal/contracts"
)

// RecordingTargetClaim acquires exclusive destination ownership without
// changing its bytes. The returned lease outlives all reads and writes.
// Arguments are the target path and its stable coordination marker path.
type RecordingTargetClaim func(context.Context, string, string) (io.Closer, error)

type RecordingTargetOwnership = recordingcontracts.RecordingTargetOwnership
type RecordingTargetValidator = recordingcontracts.RecordingTargetValidator
type RuntimeRecordingStartup = recordingcontracts.RuntimeRecordingStartup

// RecordingTargetOptions supplies host path policy and ownership effects from Wire.
type RecordingTargetOptions struct {
	Claim           RecordingTargetClaim
	CaseInsensitive bool
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
