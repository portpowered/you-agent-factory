// Package server exposes an SDK-backed stdio MCP transport for the subagent tool.
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
	ToolOperation ToolOperation
	ServerName    string
	ServerVersion string
	Resources     []ResourceRegistration
	Skills        []SkillEntry
}

// Server owns protocol and tool registration. Stream selection and lifecycle
// activation remain the responsibility of the process initializer.
type Server struct {
	sdk *mcp.Server
}

// New constructs an inert MCP server with the generated subagent tool registered
// through the official Go SDK.
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
		var annotations *mcp.ToolAnnotations
		if len(definition.Annotations) > 0 {
			if err := json.Unmarshal(definition.Annotations, &annotations); err != nil {
				return fmt.Errorf("register MCP tool %s: decode annotations: %w", definition.Name, err)
			}
		}
		addTool(server, definition.Name, definition.Description, definition.InputSchema, annotations, call)
	}
	return nil
}

func addTool(server *mcp.Server, name, description string, inputSchema json.RawMessage, annotations *mcp.ToolAnnotations, call toolCaller) {
	server.AddTool(&mcp.Tool{
		Name: name, Description: description, InputSchema: inputSchema, Annotations: annotations,
	}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := call(ctx, name, request.Params.Arguments)
		if err != nil {
			return textResult(err.Error(), true), nil
		}
		if result, ok := typedToolResponseErrorResult(raw); ok {
			return result, nil
		}
		return textResult(string(raw), false), nil
	})
}

// fallbackToolErrorMessage is the safe nonempty text used when a typed
// ToolResponse error envelope carries a blank or whitespace error.message.
// pkg/services/factory_sessions/transports/mcp keeps an identical constant;
// the two must stay in sync so server and inventory encoders agree.
const fallbackToolErrorMessage = "tool execution failed"

// typedToolResponseErrorResult detects a typed top-level ToolResponse.error
// payload produced by a successful ToolOperation call and maps it to an MCP
// CallToolResult: the first text content carries the safe human-readable
// error.message, IsError is true, and StructuredContent retains the complete
// typed ToolResponse so error code, sessionId, retryable, and details stay
// machine-readable. Detection is tightened to the typed envelope only: the
// error object must carry a nonempty error.code and the payload must not
// carry a result, so arbitrary {"error":{"message":...}} tool output keeps
// the plain success encoding. A blank or whitespace error.message falls back
// to fallbackToolErrorMessage so text content stays nonempty and readable.
func typedToolResponseErrorResult(raw json.RawMessage) (*mcp.CallToolResult, bool) {
	var probe struct {
		Result *json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil || probe.Error == nil {
		return nil, false
	}
	if strings.TrimSpace(probe.Error.Code) == "" || probe.Result != nil {
		return nil, false
	}
	message := probe.Error.Message
	if strings.TrimSpace(message) == "" {
		message = fallbackToolErrorMessage
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: message}},
		IsError:           true,
		StructuredContent: json.RawMessage(raw),
	}, true
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
