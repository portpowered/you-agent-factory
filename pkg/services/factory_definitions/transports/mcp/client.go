package factorydefinition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

var errMissingRequestContext = errors.New("MCP request context is required")

// ToolOperation is the single injected execution role used by every MCP
// protocol server. Production binds its Factory Definitions dependencies once;
// protocol tests replace this exact function role.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// Bind constructs the canonical ToolOperation from explicit Definitions root
// dependencies. Adapter tests replace Definitions or Validation with
// root-shaped fakes without constructing real catalog or distribute graphs.
func Bind(binding RootBinding) ToolOperation {
	return func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		return CallTool(ctx, binding, name, input)
	}
}

type toolHandler func(context.Context, RootBinding, json.RawMessage) (json.RawMessage, error)

var registeredToolHandlers = map[string]toolHandler{
	ToolValidate:    callValidateTool,
	ToolGetCurrent:  callGetCurrentTool,
	ToolSaveCurrent: callSaveCurrentTool,
}

// CallTool invokes one Factory Definition tool against explicitly supplied
// root-shaped roles. Protocol servers receive the bound ToolOperation rather
// than choosing between construction paths.
func CallTool(
	ctx context.Context,
	binding RootBinding,
	name string,
	input json.RawMessage,
) (json.RawMessage, error) {
	if ctx == nil {
		return nil, fmt.Errorf("call MCP tool: %w", errMissingRequestContext)
	}
	handler, ok := registeredToolHandlers[name]
	if !ok {
		return nil, fmt.Errorf("unsupported tool %q", name)
	}
	return handler(ctx, binding, input)
}

func callValidateTool(
	ctx context.Context,
	binding RootBinding,
	input json.RawMessage,
) (json.RawMessage, error) {
	var factory factoryapi.Factory
	if err := json.Unmarshal(input, &factory); err != nil {
		envelope := decodeInputErrorEnvelope("decode validate input", err)
		return json.Marshal(ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope})
	}
	validation, err := resolveSubmittedDefinitionValidation(binding)
	if err != nil {
		envelope := unavailableValidationErrorEnvelope()
		return json.Marshal(ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope})
	}
	return json.Marshal(Validate(ctx, validation, factory))
}

func callGetCurrentTool(
	ctx context.Context,
	binding RootBinding,
	input json.RawMessage,
) (json.RawMessage, error) {
	var request GetCurrentInput
	if err := json.Unmarshal(input, &request); err != nil {
		envelope := decodeInputErrorEnvelope("decode get current input", err)
		return json.Marshal(ToolResponse[factoryapi.Factory]{Error: &envelope})
	}
	return json.Marshal(GetCurrent(ctx, binding.Definitions, request))
}

func callSaveCurrentTool(
	ctx context.Context,
	binding RootBinding,
	input json.RawMessage,
) (json.RawMessage, error) {
	var request SaveCurrentInput
	if err := json.Unmarshal(input, &request); err != nil {
		envelope := decodeInputErrorEnvelope("decode save current input", err)
		return json.Marshal(ToolResponse[factoryapi.Factory]{Error: &envelope})
	}
	return json.Marshal(SaveCurrent(ctx, binding.Definitions, request))
}
