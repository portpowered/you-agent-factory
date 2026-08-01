package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ToolOperation is the protocol-neutral operation used by an MCP tool. The
// server deliberately knows nothing about the service that owns the tool; it
// forwards the caller context, stable tool name, and raw JSON arguments.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// ToolDefinition describes one tool exposed by a composed MCP registry.
// InputSchema is kept as raw JSON because schema interpretation belongs to the
// owner transport that published the definition.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Operation   ToolOperation
}

// ToolRegistry is the immutable registry consumed by the generic MCP server.
// Definitions returns detached values in registration order. Call forwards
// raw arguments to the operation selected by the tool name.
type ToolRegistry interface {
	Definitions() []ToolDefinition
	Call(context.Context, string, json.RawMessage) (json.RawMessage, error)
}

// Registry is a process-scoped, immutable MCP tool registry. Construct it in
// Wire and reuse it for each protocol session; no session opening or service
// lookup occurs during dispatch.
type Registry struct {
	definitions []ToolDefinition
	operations  map[string]ToolOperation
}

var _ ToolRegistry = (*Registry)(nil)

// NewRegistry validates and copies the supplied tool definitions. Tool names
// are unique within one registry and must be non-empty after trimming. The
// operation and schema bytes are copied so later caller mutations cannot alter
// the composed protocol surface.
func NewRegistry(definitions []ToolDefinition) (*Registry, error) {
	if len(definitions) == 0 {
		return nil, fmt.Errorf("mcp tool registry requires at least one tool")
	}

	registry := &Registry{
		definitions: make([]ToolDefinition, 0, len(definitions)),
		operations:  make(map[string]ToolOperation, len(definitions)),
	}
	for index, definition := range definitions {
		name := strings.TrimSpace(definition.Name)
		if name == "" {
			return nil, fmt.Errorf("mcp tool registry tool %d has no name", index)
		}
		if definition.Operation == nil {
			return nil, fmt.Errorf("mcp tool registry tool %q has no operation", name)
		}
		if len(definition.InputSchema) == 0 {
			return nil, fmt.Errorf("mcp tool registry tool %q has no input schema", name)
		}
		if _, exists := registry.operations[name]; exists {
			return nil, fmt.Errorf("mcp tool registry contains duplicate tool %q", name)
		}

		copied := ToolDefinition{
			Name:        name,
			Description: definition.Description,
			InputSchema: append(json.RawMessage(nil), definition.InputSchema...),
			Operation:   definition.Operation,
		}
		registry.definitions = append(registry.definitions, copied)
		registry.operations[name] = definition.Operation
	}
	return registry, nil
}

// Definitions returns a detached snapshot of the registry catalog.
func (registry *Registry) Definitions() []ToolDefinition {
	if registry == nil {
		return nil
	}
	definitions := make([]ToolDefinition, len(registry.definitions))
	for index, definition := range registry.definitions {
		definitions[index] = definition
		definitions[index].InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
	}
	return definitions
}

// Call forwards an MCP call without decoding or replacing its arguments.
func (registry *Registry) Call(
	ctx context.Context,
	name string,
	arguments json.RawMessage,
) (json.RawMessage, error) {
	if registry == nil {
		return nil, fmt.Errorf("mcp tool registry is required")
	}
	operation, ok := registry.operations[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", name)
	}
	return operation(ctx, name, arguments)
}
