package factory

import "context"

// CheckpointOutcome is the plain success vocabulary for Factory Runtime root
// checkpoint operations. Peers branch on these values without Petri marking
// snapshots or JavaScript checkpoint strategy types.
type CheckpointOutcome string

const (
	// CheckpointOutcomeCaptured indicates a versioned checkpoint was captured.
	CheckpointOutcomeCaptured CheckpointOutcome = "CAPTURED"
	// CheckpointOutcomeLoaded indicates a checkpoint was loaded for inspect or
	// compatibility checking.
	CheckpointOutcomeLoaded CheckpointOutcome = "LOADED"
	// CheckpointOutcomeRestored indicates a compatible opaque checkpoint was
	// restored into mutable Runtime execution state.
	CheckpointOutcomeRestored CheckpointOutcome = "RESTORED"
)

// Checkpoint is the plain versioned checkpoint value published at the Runtime
// root. Payload is opaque strategy checkpoint bytes; peers must not interpret
// it as Petri marking snapshots or JavaScript checkpoint strategy records.
// Recordings remains the owner of immutable history; this value is Runtime's
// mutable execution-checkpoint vocabulary only.
type Checkpoint struct {
	CheckpointID  string
	SchemaVersion int
	// StrategyKind is an opaque strategy discriminator string. It is not a
	// peer-facing Petri or JavaScript type; nested IMP-RUN packets own codec
	// meaning.
	StrategyKind string
	Payload      []byte
}

// CaptureCheckpointRequest is the plain capture input published at the Runtime
// root.
type CaptureCheckpointRequest struct {
	CheckpointID string
}

// CaptureCheckpointResult is the plain capture success shape published at the
// Runtime root.
type CaptureCheckpointResult struct {
	Outcome    CheckpointOutcome
	Checkpoint Checkpoint
}

// LoadCheckpointRequest is the plain load/inspect-compatibility input published
// at the Runtime root.
type LoadCheckpointRequest struct {
	CheckpointID          string
	ExpectedSchemaVersion int
}

// LoadCheckpointResult is the plain load/inspect success shape published at the
// Runtime root. Compatible reports whether the loaded checkpoint matches the
// expected schema version when ExpectedSchemaVersion is non-zero.
type LoadCheckpointResult struct {
	Outcome    CheckpointOutcome
	Checkpoint Checkpoint
	Compatible bool
}

// RestoreCheckpointRequest is the plain restore input published at the Runtime
// root. Checkpoint.Payload remains opaque strategy bytes.
type RestoreCheckpointRequest struct {
	Checkpoint Checkpoint
}

// RestoreCheckpointResult is the plain restore success shape published at the
// Runtime root.
type RestoreCheckpointResult struct {
	Outcome      CheckpointOutcome
	CheckpointID string
}

// ApplyCaptureCheckpoint exercises the published capture checkpoint contract
// against Service.
func ApplyCaptureCheckpoint(ctx context.Context, runtime Service, req CaptureCheckpointRequest) (CaptureCheckpointResult, error) {
	if runtime == nil {
		return CaptureCheckpointResult{}, ErrNotFound
	}
	return runtime.CaptureCheckpoint(ctx, req)
}

// ApplyLoadCheckpoint exercises the published load/inspect checkpoint contract
// against Service.
func ApplyLoadCheckpoint(ctx context.Context, runtime Service, req LoadCheckpointRequest) (LoadCheckpointResult, error) {
	if runtime == nil {
		return LoadCheckpointResult{}, ErrNotFound
	}
	if req.CheckpointID == "" {
		return LoadCheckpointResult{}, ErrCheckpointNotFound
	}
	return runtime.LoadCheckpoint(ctx, req)
}

// ApplyRestoreCheckpoint exercises the published restore checkpoint contract
// against Service. Empty or non-positive schema versions are rejected as
// corrupt/incompatible at the Apply boundary before Service.RestoreCheckpoint.
func ApplyRestoreCheckpoint(ctx context.Context, runtime Service, req RestoreCheckpointRequest) (RestoreCheckpointResult, error) {
	if runtime == nil {
		return RestoreCheckpointResult{}, ErrNotFound
	}
	if err := validateCheckpointForRestore(req.Checkpoint); err != nil {
		return RestoreCheckpointResult{}, err
	}
	return runtime.RestoreCheckpoint(ctx, req)
}

func validateCheckpointForRestore(checkpoint Checkpoint) error {
	if checkpoint.CheckpointID == "" {
		return ErrCheckpointNotFound
	}
	if checkpoint.SchemaVersion <= 0 {
		return ErrCorruptCheckpoint
	}
	if len(checkpoint.Payload) == 0 {
		return ErrCorruptCheckpoint
	}
	return nil
}
