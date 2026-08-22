package checkpoint_recovery

import (
	"encoding/json"
	"strings"
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
// checkpoint payload bytes during capture. Nested codecs own meaning; this
// remains private recovery vocabulary with no public Runtime root exposure.
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

// Service owns Runtime-private checkpoint capture, load, and restore against the
// opaque store port.
type Service interface {
	Capture(CaptureRequest) (CaptureResult, error)
	Load(LoadRequest) (LoadResult, error)
	Restore(RestoreRequest) (RestoreResult, error)
}

// EncodeRuntimeOpaquePayload serializes minimal execution facts into opaque
// strategy bytes suitable for private Runtime checkpoint capture.
func EncodeRuntimeOpaquePayload(facts ExecutionCaptureFacts) ([]byte, error) {
	state := strings.TrimSpace(facts.FactoryState)
	if state == "" {
		return nil, ErrCorruptCheckpoint
	}
	return json.Marshal(struct {
		FactoryState string `json:"factoryState"`
	}{FactoryState: state})
}

// RestoreRuntimeOpaquePayload decodes opaque strategy bytes into execution facts
// suitable for mutable Runtime state restore.
func RestoreRuntimeOpaquePayload(payload []byte) (ExecutionCaptureFacts, error) {
	if len(payload) == 0 {
		return ExecutionCaptureFacts{}, ErrCorruptCheckpoint
	}
	var decoded struct {
		FactoryState string `json:"factoryState"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return ExecutionCaptureFacts{}, ErrCorruptCheckpoint
	}
	state := strings.TrimSpace(decoded.FactoryState)
	if state == "" {
		return ExecutionCaptureFacts{}, ErrCorruptCheckpoint
	}
	return ExecutionCaptureFacts{FactoryState: state}, nil
}
