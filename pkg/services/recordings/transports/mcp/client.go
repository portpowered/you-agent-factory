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
	ToolQueryStatus: handleQueryStatus,
	ToolAppendEvent: handleAppendEvent,
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
