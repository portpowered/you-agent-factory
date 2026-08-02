package checkpoint

import (
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// RuntimeCheckpointHTTP is the wire checkpoint shape used by the checkpoint
// endpoints.
type RuntimeCheckpointHTTP struct {
	CheckpointID  string `json:"checkpointId"`
	SchemaVersion int    `json:"schemaVersion"`
	StrategyKind  string `json:"strategyKind,omitempty"`
	Payload       []byte `json:"payload,omitempty"`
}

type runtimeCaptureCheckpointHTTPRequest struct {
	CheckpointID string `json:"checkpointId"`
}

// RuntimeCaptureCheckpointHTTPResponse is the capture response shape retained
// for the parent adapter's compatibility surface.
type RuntimeCaptureCheckpointHTTPResponse struct {
	Outcome    factoryruntime.CheckpointOutcome `json:"outcome"`
	Checkpoint RuntimeCheckpointHTTP            `json:"checkpoint"`
}

type runtimeLoadCheckpointHTTPRequest struct {
	CheckpointID          string `json:"checkpointId"`
	ExpectedSchemaVersion int    `json:"expectedSchemaVersion,omitempty"`
}

// RuntimeLoadCheckpointHTTPResponse is the load response shape retained for
// the parent adapter's compatibility surface.
type RuntimeLoadCheckpointHTTPResponse struct {
	Outcome    factoryruntime.CheckpointOutcome `json:"outcome"`
	Checkpoint RuntimeCheckpointHTTP            `json:"checkpoint"`
	Compatible bool                             `json:"compatible"`
}

type runtimeRestoreCheckpointHTTPRequest struct {
	Checkpoint RuntimeCheckpointHTTP `json:"checkpoint"`
}

// RuntimeRestoreCheckpointHTTPResponse is the restore response shape retained
// for the parent adapter's compatibility surface.
type RuntimeRestoreCheckpointHTTPResponse struct {
	Outcome      factoryruntime.CheckpointOutcome `json:"outcome"`
	CheckpointID string                           `json:"checkpointId"`
}

func captureCheckpointRequestFromHTTP(req runtimeCaptureCheckpointHTTPRequest) factoryruntime.CaptureCheckpointRequest {
	return factoryruntime.CaptureCheckpointRequest{CheckpointID: strings.TrimSpace(req.CheckpointID)}
}

func loadCheckpointRequestFromHTTP(req runtimeLoadCheckpointHTTPRequest) factoryruntime.LoadCheckpointRequest {
	return factoryruntime.LoadCheckpointRequest{
		CheckpointID:          strings.TrimSpace(req.CheckpointID),
		ExpectedSchemaVersion: req.ExpectedSchemaVersion,
	}
}

func restoreCheckpointRequestFromHTTP(req runtimeRestoreCheckpointHTTPRequest) factoryruntime.RestoreCheckpointRequest {
	return factoryruntime.RestoreCheckpointRequest{Checkpoint: checkpointFromHTTP(req.Checkpoint)}
}

func checkpointFromHTTP(checkpoint RuntimeCheckpointHTTP) factoryruntime.Checkpoint {
	return factoryruntime.Checkpoint{
		CheckpointID:  strings.TrimSpace(checkpoint.CheckpointID),
		SchemaVersion: checkpoint.SchemaVersion,
		StrategyKind:  strings.TrimSpace(checkpoint.StrategyKind),
		Payload:       checkpoint.Payload,
	}
}

func checkpointToHTTP(checkpoint factoryruntime.Checkpoint) RuntimeCheckpointHTTP {
	return RuntimeCheckpointHTTP{
		CheckpointID:  checkpoint.CheckpointID,
		SchemaVersion: checkpoint.SchemaVersion,
		StrategyKind:  checkpoint.StrategyKind,
		Payload:       checkpoint.Payload,
	}
}

func captureCheckpointResponseFromResult(result factoryruntime.CaptureCheckpointResult) RuntimeCaptureCheckpointHTTPResponse {
	return RuntimeCaptureCheckpointHTTPResponse{Outcome: result.Outcome, Checkpoint: checkpointToHTTP(result.Checkpoint)}
}

func loadCheckpointResponseFromResult(result factoryruntime.LoadCheckpointResult) RuntimeLoadCheckpointHTTPResponse {
	return RuntimeLoadCheckpointHTTPResponse{
		Outcome:    result.Outcome,
		Checkpoint: checkpointToHTTP(result.Checkpoint),
		Compatible: result.Compatible,
	}
}

func restoreCheckpointResponseFromResult(result factoryruntime.RestoreCheckpointResult) RuntimeRestoreCheckpointHTTPResponse {
	return RuntimeRestoreCheckpointHTTPResponse{Outcome: result.Outcome, CheckpointID: result.CheckpointID}
}
