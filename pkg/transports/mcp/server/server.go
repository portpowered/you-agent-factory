// Package server exposes an SDK-backed, owner-neutral MCP transport.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformstdio "github.com/portpowered/infinite-you/pkg/platform/stdio"
	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
)

const (
	defaultServerName    = "you-agent-factory"
	defaultServerVersion = "dev"
)

// Options configures one MCP server instance around a precomposed tool
// registry. ToolOperation and the generated catalog are retained as a small
// compatibility path for callers that have not yet moved their owner registry
// into Wire.
type Options struct {
	Registry      ToolRegistry
	ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)
	ServerName    string
	ServerVersion string
}

// Server owns protocol and tool registration. Stream selection and lifecycle
// activation remain the responsibility of the process initializer.
type Server struct {
	sdk *mcp.Server
}

// New constructs an inert MCP server with the supplied registry registered
// through the official Go SDK. The generic fallback uses the generated catalog
// and is intentionally kept only for the staged owner-transport migration.
func New(opts Options) (*Server, error) {
	registry, err := resolveRegistry(opts)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(opts.ServerName)
	if name == "" {
		name = defaultServerName
	}
	version := strings.TrimSpace(opts.ServerVersion)
	if version == "" {
		version = defaultServerVersion
	}

	sdk := mcp.NewServer(
		&mcp.Implementation{Name: name, Version: version},
		&mcp.ServerOptions{
			Capabilities: &mcp.ServerCapabilities{
				Tools: &mcp.ToolCapabilities{},
			},
		},
	)
	if err := registerTools(sdk, registry); err != nil {
		return nil, err
	}
	return &Server{sdk: sdk}, nil
}

func resolveRegistry(opts Options) (ToolRegistry, error) {
	if opts.Registry != nil {
		if len(opts.Registry.Definitions()) == 0 {
			return nil, fmt.Errorf("mcp server requires a non-empty tool registry")
		}
		return opts.Registry, nil
	}
	if opts.ToolOperation == nil {
		return nil, fmt.Errorf("mcp server requires a tool registry or tool operation")
	}

	definitions := mcpgenerated.PrimaryDiscovery()
	tools := make([]ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, ToolDefinition{
			Name:        definition.Name,
			Description: definition.Description,
			InputSchema: append(json.RawMessage(nil), definition.InputSchema...),
			Operation:   opts.ToolOperation,
		})
	}
	return NewRegistry(tools)
}

func registerTools(server *mcp.Server, registry ToolRegistry) error {
	for _, definition := range registry.Definitions() {
		var schema map[string]any
		if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
			return fmt.Errorf("register MCP tool %s: decode generated input schema: %w", definition.Name, err)
		}
		if schema["type"] != "object" {
			return fmt.Errorf("register MCP tool %s: generated input schema must have object type", definition.Name)
		}
		addTool(server, definition.Name, definition.Description, definition.InputSchema, registry)
	}
	return nil
}

func addTool(server *mcp.Server, name, description string, inputSchema json.RawMessage, registry ToolRegistry) {
	server.AddTool(&mcp.Tool{
		Name: name, Description: description, InputSchema: inputSchema,
	}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := registry.Call(ctx, name, request.Params.Arguments)
		if err != nil {
			return textResult(err.Error(), true), nil
		}
		return textResult(string(raw), false), nil
	})
}

func textResult(text string, isError bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: isError,
	}
}

// ServeStdio runs the already-constructed server over caller-owned streams.
func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	if s == nil || s.sdk == nil {
		return fmt.Errorf("serve MCP stdio: server is required")
	}
	if in == nil {
		return fmt.Errorf("serve MCP stdio: input is required")
	}
	if out == nil {
		return fmt.Errorf("serve MCP stdio: output is required")
	}
	reader, writer := platformstdio.DrainJSONRPCResponses(ctx, in, out)
	return s.Serve(ctx, &mcp.IOTransport{
		Reader: reader,
		Writer: writer,
	})
}

// Serve runs the server over an SDK transport. It is useful for process-owned
// transports other than stdio and for protocol-level integration tests.
func (s *Server) Serve(ctx context.Context, transport mcp.Transport) error {
	if s == nil || s.sdk == nil {
		return fmt.Errorf("serve MCP: server is required")
	}
	if transport == nil {
		return fmt.Errorf("serve MCP: transport is required")
	}
	err := s.sdk.Run(ctx, transport)
	if errors.Is(err, io.EOF) || (err != nil && strings.HasSuffix(err.Error(), ": EOF")) {
		return nil
	}
	return err
}
