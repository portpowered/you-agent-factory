package checkpoint_recovery

import (
	"encoding/json"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const (
	// RuntimeOpaqueCheckpointSchemaVersion is the published schema version for
	// Runtime-owned opaque execution checkpoints captured in IMP-RUN-04.
	RuntimeOpaqueCheckpointSchemaVersion = 1
	// RuntimeOpaqueCheckpointStrategyKind is the opaque strategy discriminator
	// for Runtime-owned execution checkpoints. Peers treat it as an opaque
	// string and must not interpret it as Petri or JavaScript strategy types.
	RuntimeOpaqueCheckpointStrategyKind = "runtime"
)

// ExecutionCaptureFacts are the minimal execution facts encoded into opaque
// checkpoint payload bytes during capture. Nested codecs own meaning; peers only
// observe the resulting opaque Payload on the Runtime root.
type ExecutionCaptureFacts struct {
	FactoryState string
}

// CaptureRequest captures one versioned opaque checkpoint envelope.
type CaptureRequest struct {
	CheckpointID string
	Payload      []byte
	StrategyKind string
}

// CaptureResult reports the stored opaque checkpoint envelope.
type CaptureResult struct {
	Envelope Envelope
}

// Service owns Runtime-private checkpoint capture against the opaque store
// port. Load and restore remain separate root operations in later stories.
type Service interface {
	Capture(CaptureRequest) (CaptureResult, error)
}

// EncodeRuntimeOpaquePayload serializes minimal execution facts into opaque
// strategy bytes suitable for Runtime root checkpoint capture.
func EncodeRuntimeOpaquePayload(facts ExecutionCaptureFacts) ([]byte, error) {
	state := strings.TrimSpace(facts.FactoryState)
	if state == "" {
		return nil, ErrCorruptCheckpoint
	}
	return json.Marshal(struct {
		FactoryState string `json:"factoryState"`
	}{FactoryState: state})
}

// RootCheckpointFromEnvelope maps a private envelope to the published Runtime
// root checkpoint vocabulary.
func RootCheckpointFromEnvelope(envelope Envelope) factoryruntime.Checkpoint {
	return factoryruntime.Checkpoint{
		CheckpointID:  envelope.CheckpointID,
		SchemaVersion: envelope.SchemaVersion,
		StrategyKind:  envelope.StrategyKind,
		Payload:       append([]byte(nil), envelope.Payload...),
	}
}
