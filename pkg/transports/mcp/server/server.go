// Package server exposes an SDK-backed stdio MCP transport for Factory Session tools.
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

// ToolOperation is an owner-neutral raw MCP dispatch role. The shared server
// does not know which product service owns a tool; Wire supplies the
// precomposed operation and this package only handles protocol values.
type ToolOperation func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// Options configures one MCP server instance around the exact injected raw
// tool operation used by production and protocol tests.
type Options struct {
	ToolOperation   ToolOperation
	AdditionalTools []ToolRegistration
	ServerName      string
	ServerVersion   string
	Resources       []ResourceRegistration
	Skills          []SkillEntry
}

// ToolRegistration adds an owner-composed tool to the generated catalog.
type ToolRegistration struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Call        ToolOperation
}

// Server owns protocol and tool registration. Stream selection and lifecycle
// activation remain the responsibility of the process initializer.
type Server struct {
	sdk *mcp.Server
}

// New constructs an inert MCP server with all canonical and compatibility
// Factory Session tools registered through the official Go SDK.
func New(opts Options) (*Server, error) {
	if opts.ToolOperation == nil {
		return nil, fmt.Errorf("mcp server requires a tool operation")
	}
	sdk, err := newSDKServer(opts)
	if err != nil {
		return nil, err
	}
	return &Server{sdk: sdk}, nil
}

func newSDKServer(opts Options) (*mcp.Server, error) {
	name := strings.TrimSpace(opts.ServerName)
	if name == "" {
		name = defaultServerName
	}
	version := strings.TrimSpace(opts.ServerVersion)
	if version == "" {
		version = defaultServerVersion
	}

	capabilities := &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}}
	if len(opts.Resources) > 0 {
		capabilities.Resources = &mcp.ResourceCapabilities{}
	}
	if len(opts.Skills) > 0 {
		capabilities.AddExtension(skillsExtension, nil)
	}
	sdk := mcp.NewServer(
		&mcp.Implementation{Name: name, Version: version},
		&mcp.ServerOptions{
			Capabilities: capabilities,
		},
	)
	if err := registerResources(sdk, opts.Resources); err != nil {
		return nil, err
	}
	if err := registerTools(sdk, toolCaller(opts.ToolOperation)); err != nil {
		return nil, err
	}
	if err := registerAdditionalTools(sdk, opts.AdditionalTools); err != nil {
		return nil, err
	}
	if len(opts.Skills) > 0 {
		if err := registerSkills(sdk, opts.Skills); err != nil {
			return nil, err
		}
	}
	return sdk, nil
}

func registerResources(server *mcp.Server, resources []ResourceRegistration) error {
	for _, resource := range resources {
		if resource.Resource == nil || resource.Read == nil {
			return fmt.Errorf("mcp server resource registration requires resource and read handler")
		}
		server.AddResource(resource.Resource, resource.Read)
	}
	return nil
}

func registerAdditionalTools(server *mcp.Server, tools []ToolRegistration) error {
	for _, tool := range tools {
		if tool.Name == "" || tool.Call == nil || len(tool.InputSchema) == 0 {
			return fmt.Errorf("mcp additional tool requires name, schema, and handler")
		}
		var schema map[string]any
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil || schema["type"] != "object" {
			return fmt.Errorf("mcp additional tool %s requires an object input schema", tool.Name)
		}
		addTool(server, tool.Name, tool.Description, tool.InputSchema, toolCaller(tool.Call))
	}
	return nil
}

type toolCaller func(context.Context, string, json.RawMessage) (json.RawMessage, error)

func registerTools(server *mcp.Server, call toolCaller) error {
	for _, definition := range mcpgenerated.PrimaryDiscovery() {
		var schema map[string]any
		if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
			return fmt.Errorf("register MCP tool %s: decode generated input schema: %w", definition.Name, err)
		}
		if schema["type"] != "object" {
			return fmt.Errorf("register MCP tool %s: generated input schema must have object type", definition.Name)
		}
		addTool(server, definition.Name, definition.Description, definition.InputSchema, call)
	}
	return nil
}

func addTool(server *mcp.Server, name, description string, inputSchema json.RawMessage, call toolCaller) {
	server.AddTool(&mcp.Tool{
		Name: name, Description: description, InputSchema: inputSchema,
	}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := call(ctx, name, request.Params.Arguments)
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
