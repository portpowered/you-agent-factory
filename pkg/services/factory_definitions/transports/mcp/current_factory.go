package factorydefinition

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorydefinitionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorydefinition"
)

// GetCurrentInput is the MCP request shape for you.factory_definition.get_current.
type GetCurrentInput struct {
	SessionID string `json:"sessionId"`
}

// SaveCurrentInput is the MCP request shape for you.factory_definition.save_current.
type SaveCurrentInput struct {
	SessionID string                      `json:"sessionId"`
	Mode      *factoryapi.FactorySaveMode   `json:"mode,omitempty"`
	Factory   factoryapi.Factory            `json:"factory"`
}

// GetCurrent returns the current Factory for one Factory Session through the
// you.factory_definition.get_current MCP tool.
func GetCurrent(ctx context.Context, root DefinitionsRoot, input GetCurrentInput) ToolResponse[factoryapi.Factory] {
	if ctx == nil {
		envelope := decodeInputErrorEnvelope("get current factory", errMissingRequestContext)
		return ToolResponse[factoryapi.Factory]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.Factory](ctx); done {
		return response
	}
	if root == nil {
		envelope := unavailableDefinitionsErrorEnvelope()
		return ToolResponse[factoryapi.Factory]{Error: &envelope}
	}

	factory, err := factorydefinitionmapping.GetCurrentFactoryForSession(ctx, root, input.SessionID)
	if err != nil {
		envelope := currentFactoryErrorEnvelope(input.SessionID, "get", err)
		return ToolResponse[factoryapi.Factory]{Error: &envelope}
	}
	return ToolResponse[factoryapi.Factory]{Result: &factory}
}

// SaveCurrent persists the current Factory for one Factory Session through the
// you.factory_definition.save_current MCP tool.
func SaveCurrent(ctx context.Context, root DefinitionsRoot, input SaveCurrentInput) ToolResponse[factoryapi.Factory] {
	if ctx == nil {
		envelope := decodeInputErrorEnvelope("save current factory", errMissingRequestContext)
		return ToolResponse[factoryapi.Factory]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.Factory](ctx); done {
		return response
	}
	if root == nil {
		envelope := unavailableDefinitionsErrorEnvelope()
		return ToolResponse[factoryapi.Factory]{Error: &envelope}
	}

	mode := factoryapi.FactorySaveModeReplaceCurrent
	if input.Mode != nil {
		mode = *input.Mode
	}
	saved, err := factorydefinitionmapping.New(root).Save(ctx, input.SessionID, mode, input.Factory)
	if err != nil {
		envelope := currentFactoryErrorEnvelope(input.SessionID, "save", err)
		return ToolResponse[factoryapi.Factory]{Error: &envelope}
	}
	return ToolResponse[factoryapi.Factory]{Result: &saved}
}
