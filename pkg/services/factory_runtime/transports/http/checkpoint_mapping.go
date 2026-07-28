package http

import (
	"encoding/json"
	"io"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

type runtimeCheckpointHTTP struct {
	CheckpointID  string `json:"checkpointId"`
	SchemaVersion int    `json:"schemaVersion"`
	StrategyKind  string `json:"strategyKind,omitempty"`
	Payload       []byte `json:"payload,omitempty"`
}

type runtimeCaptureCheckpointHTTPRequest struct {
	CheckpointID string `json:"checkpointId"`
}

type runtimeCaptureCheckpointHTTPResponse struct {
	Outcome    factoryruntime.CheckpointOutcome `json:"outcome"`
	Checkpoint runtimeCheckpointHTTP            `json:"checkpoint"`
}

type runtimeLoadCheckpointHTTPRequest struct {
	CheckpointID          string `json:"checkpointId"`
	ExpectedSchemaVersion int    `json:"expectedSchemaVersion,omitempty"`
}

type runtimeLoadCheckpointHTTPResponse struct {
	Outcome    factoryruntime.CheckpointOutcome `json:"outcome"`
	Checkpoint runtimeCheckpointHTTP            `json:"checkpoint"`
	Compatible bool                             `json:"compatible"`
}

type runtimeRestoreCheckpointHTTPRequest struct {
	Checkpoint runtimeCheckpointHTTP `json:"checkpoint"`
}

type runtimeRestoreCheckpointHTTPResponse struct {
	Outcome      factoryruntime.CheckpointOutcome `json:"outcome"`
	CheckpointID string                           `json:"checkpointId"`
}

func decodeCaptureCheckpointRequest(body io.Reader) (factoryruntime.CaptureCheckpointRequest, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return factoryruntime.CaptureCheckpointRequest{}, err
	}
	if len(payload) == 0 {
		return factoryruntime.CaptureCheckpointRequest{}, errRequestBodyRequired
	}
	var req runtimeCaptureCheckpointHTTPRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return factoryruntime.CaptureCheckpointRequest{}, err
	}
	return captureCheckpointRequestFromHTTP(req), nil
}

func decodeLoadCheckpointRequest(body io.Reader) (factoryruntime.LoadCheckpointRequest, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return factoryruntime.LoadCheckpointRequest{}, err
	}
	if len(payload) == 0 {
		return factoryruntime.LoadCheckpointRequest{}, errRequestBodyRequired
	}
	var req runtimeLoadCheckpointHTTPRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return factoryruntime.LoadCheckpointRequest{}, err
	}
	return loadCheckpointRequestFromHTTP(req), nil
}

func decodeRestoreCheckpointRequest(body io.Reader) (factoryruntime.RestoreCheckpointRequest, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return factoryruntime.RestoreCheckpointRequest{}, err
	}
	if len(payload) == 0 {
		return factoryruntime.RestoreCheckpointRequest{}, errRequestBodyRequired
	}
	var req runtimeRestoreCheckpointHTTPRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return factoryruntime.RestoreCheckpointRequest{}, err
	}
	return restoreCheckpointRequestFromHTTP(req), nil
}

func captureCheckpointRequestFromHTTP(
	req runtimeCaptureCheckpointHTTPRequest,
) factoryruntime.CaptureCheckpointRequest {
	return factoryruntime.CaptureCheckpointRequest{
		CheckpointID: strings.TrimSpace(req.CheckpointID),
	}
}

func loadCheckpointRequestFromHTTP(
	req runtimeLoadCheckpointHTTPRequest,
) factoryruntime.LoadCheckpointRequest {
	return factoryruntime.LoadCheckpointRequest{
		CheckpointID:          strings.TrimSpace(req.CheckpointID),
		ExpectedSchemaVersion: req.ExpectedSchemaVersion,
	}
}

func restoreCheckpointRequestFromHTTP(
	req runtimeRestoreCheckpointHTTPRequest,
) factoryruntime.RestoreCheckpointRequest {
	return factoryruntime.RestoreCheckpointRequest{
		Checkpoint: checkpointFromHTTP(req.Checkpoint),
	}
}

func checkpointFromHTTP(checkpoint runtimeCheckpointHTTP) factoryruntime.Checkpoint {
	return factoryruntime.Checkpoint{
		CheckpointID:  strings.TrimSpace(checkpoint.CheckpointID),
		SchemaVersion: checkpoint.SchemaVersion,
		StrategyKind:  strings.TrimSpace(checkpoint.StrategyKind),
		Payload:       checkpoint.Payload,
	}
}

func checkpointToHTTP(checkpoint factoryruntime.Checkpoint) runtimeCheckpointHTTP {
	return runtimeCheckpointHTTP{
		CheckpointID:  checkpoint.CheckpointID,
		SchemaVersion: checkpoint.SchemaVersion,
		StrategyKind:  checkpoint.StrategyKind,
		Payload:       checkpoint.Payload,
	}
}

func captureCheckpointResponseFromResult(
	result factoryruntime.CaptureCheckpointResult,
) runtimeCaptureCheckpointHTTPResponse {
	return runtimeCaptureCheckpointHTTPResponse{
		Outcome:    result.Outcome,
		Checkpoint: checkpointToHTTP(result.Checkpoint),
	}
}

func loadCheckpointResponseFromResult(
	result factoryruntime.LoadCheckpointResult,
) runtimeLoadCheckpointHTTPResponse {
	return runtimeLoadCheckpointHTTPResponse{
		Outcome:    result.Outcome,
		Checkpoint: checkpointToHTTP(result.Checkpoint),
		Compatible: result.Compatible,
	}
}

func restoreCheckpointResponseFromResult(
	result factoryruntime.RestoreCheckpointResult,
) runtimeRestoreCheckpointHTTPResponse {
	return runtimeRestoreCheckpointHTTPResponse{
		Outcome:      result.Outcome,
		CheckpointID: result.CheckpointID,
	}
}
