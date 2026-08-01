package recordingmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

var errMissingRequestContext = errors.New("MCP request context is required")

// ToolOperation is the single injected execution role used by every MCP
// protocol server. Production binds its Recordings dependencies once; protocol
// tests replace this exact function role.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// RootDependencies are the accepted Recordings root roles consumed by the MCP
// adapter. Recordings is the singular Service root; transports inject an
// implementation or test fake rather than importing Recordings internals or
// constructing canonical state.
type RootDependencies struct {
	Recordings recordings.Service
}

// Bind constructs the canonical ToolOperation from explicit Recordings root
// dependencies. Adapter tests replace Recordings with a root-shaped fake
// without constructing real ledger, projection, lifecycle, replay, or artifact
// graphs or service-local Wire.
func Bind(deps RootDependencies) ToolOperation {
	return func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		return CallTool(ctx, deps.Recordings, name, input)
	}
}

// BindToolOperation binds the canonical tool registry to an explicit Recordings
// Service root without constructing an alternate MCP client.
func BindToolOperation(service recordings.Service) ToolOperation {
	return Bind(RootDependencies{Recordings: service})
}

func callToolJSON[Input any, Output any](
	input json.RawMessage,
	decodeErr string,
	handler func(Input) ToolResponse[Output],
) (json.RawMessage, error) {
	var request Input
	if err := json.Unmarshal(input, &request); err != nil {
		envelope := decodeInputErrorEnvelope(decodeErr, err)
		return json.Marshal(ToolResponse[Output]{Error: &envelope})
	}
	return json.Marshal(handler(request))
}

type canonicalToolHandler func(
	context.Context,
	recordings.Service,
	json.RawMessage,
) (json.RawMessage, error)

var canonicalToolHandlers = map[string]canonicalToolHandler{
	ToolQueryStatus:          handleQueryStatus,
	ToolAppendEvent:          handleAppendEvent,
	ToolLoadReplay:           handleLoadReplay,
	ToolReadPortableArtifact: handleReadPortableArtifact,
}

func handleQueryStatus(
	ctx context.Context,
	service recordings.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode query status input", func(request QueryStatusInput) ToolResponse[recordings.RecordingStatusResult] {
		return QueryStatus(ctx, service, request)
	})
}

func handleAppendEvent(
	ctx context.Context,
	service recordings.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode append event input", func(request AppendEventInput) ToolResponse[recordings.AppendRecordedEventResult] {
		return AppendEvent(ctx, service, request)
	})
}

func handleLoadReplay(
	ctx context.Context,
	service recordings.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode load replay input", func(request LoadReplayInput) ToolResponse[recordings.LoadReplayRecordingResult] {
		return LoadReplay(ctx, service, request)
	})
}

func handleReadPortableArtifact(
	ctx context.Context,
	service recordings.Service,
	input json.RawMessage,
) (json.RawMessage, error) {
	return callToolJSON(input, "decode read portable artifact input", func(request ReadPortableArtifactInput) ToolResponse[recordings.ReadPortableArtifactResult] {
		return ReadPortableArtifact(ctx, service, request)
	})
}

// CallTool invokes one Recordings tool against an explicitly supplied Service
// root. Protocol servers receive the bound ToolOperation rather than choosing
// between construction paths.
func CallTool(
	ctx context.Context,
	service recordings.Service,
	name string,
	input json.RawMessage,
) (json.RawMessage, error) {
	if ctx == nil {
		return nil, fmt.Errorf("call MCP tool: %w", errMissingRequestContext)
	}
	handler, ok := canonicalToolHandlers[name]
	if !ok {
		return nil, fmt.Errorf("unsupported tool %q", name)
	}
	return handler(ctx, service, input)
}

// LoadReplayInput is the MCP request shape for you.recording.load_replay.
type LoadReplayInput struct {
	RecordingID string `json:"recordingId"`
}

// LoadReplay loads finalized canonical replay facts through the
// you.recording.load_replay MCP tool.
func LoadReplay(
	ctx context.Context,
	service recordings.Service,
	input LoadReplayInput,
) ToolResponse[recordings.LoadReplayRecordingResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(input.RecordingID, errMissingRequestContext)
		return ToolResponse[recordings.LoadReplayRecordingResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[recordings.LoadReplayRecordingResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[recordings.LoadReplayRecordingResult]{Error: &envelope}
	}

	recordingID := recordings.RecordingID(input.RecordingID)
	result, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		envelope := loadReplayErrorEnvelope(input.RecordingID, err)
		return ToolResponse[recordings.LoadReplayRecordingResult]{Error: &envelope}
	}
	return ToolResponse[recordings.LoadReplayRecordingResult]{Result: &result}
}

// QueryStatusInput is the MCP request shape for you.recording.query_status.
type QueryStatusInput struct {
	RecordingID string `json:"recordingId"`
}

// QueryStatus returns detached recording lifecycle status through the
// you.recording.query_status MCP tool.
func QueryStatus(
	ctx context.Context,
	service recordings.Service,
	input QueryStatusInput,
) ToolResponse[recordings.RecordingStatusResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(input.RecordingID, errMissingRequestContext)
		return ToolResponse[recordings.RecordingStatusResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[recordings.RecordingStatusResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[recordings.RecordingStatusResult]{Error: &envelope}
	}

	recordingID := recordings.RecordingID(input.RecordingID)
	result, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		envelope := statusQueryErrorEnvelope(input.RecordingID, err)
		return ToolResponse[recordings.RecordingStatusResult]{Error: &envelope}
	}
	return ToolResponse[recordings.RecordingStatusResult]{Result: &result}
}

// ReadPortableArtifactInput is the MCP request shape for
// you.recording.read_portable_artifact.
type ReadPortableArtifactInput struct {
	RecordingID string `json:"recordingId"`
	Reference   string `json:"reference"`
}

// ReadPortableArtifact reads and validates one published portable artifact
// through the you.recording.read_portable_artifact MCP tool.
func ReadPortableArtifact(
	ctx context.Context,
	service recordings.Service,
	input ReadPortableArtifactInput,
) ToolResponse[recordings.ReadPortableArtifactResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(input.RecordingID, errMissingRequestContext)
		return ToolResponse[recordings.ReadPortableArtifactResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[recordings.ReadPortableArtifactResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[recordings.ReadPortableArtifactResult]{Error: &envelope}
	}

	result, err := service.ReadPortableArtifact(ctx, recordings.ReadPortableArtifactRequest{
		RecordingID: recordings.RecordingID(input.RecordingID),
		Reference:   recordings.RecordingArtifactReference(input.Reference),
	})
	if err != nil {
		envelope := readPortableArtifactErrorEnvelope(input.RecordingID, err)
		return ToolResponse[recordings.ReadPortableArtifactResult]{Error: &envelope}
	}
	return ToolResponse[recordings.ReadPortableArtifactResult]{Result: &result}
}
