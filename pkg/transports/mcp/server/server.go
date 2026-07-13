// Package server exposes a stdio MCP transport for Factory Session tools.
package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
)

const protocolVersion = "2024-11-05"

// Options configures one stdio MCP server instance.
type Options struct {
	Client        *mcpfactorysession.Client
	ServerName    string
	ServerVersion string
}

// Server serves Factory Session MCP tools over newline-delimited JSON-RPC.
type Server struct {
	client        *mcpfactorysession.Client
	serverName    string
	serverVersion string
}

// New constructs one MCP server backed by the supplied Factory Session client.
func New(opts Options) (*Server, error) {
	if opts.Client == nil {
		return nil, fmt.Errorf("mcp server requires a factory session client")
	}
	name := strings.TrimSpace(opts.ServerName)
	if name == "" {
		name = "you-agent-factory"
	}
	version := strings.TrimSpace(opts.ServerVersion)
	if version == "" {
		version = "dev"
	}
	return &Server{
		client:        opts.Client,
		serverName:    name,
		serverVersion: version,
	}, nil
}

// ServeStdio reads MCP JSON-RPC requests from in and writes responses to out.
func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	writer := bufio.NewWriter(out)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read mcp request: %w", err)
			}
			return nil
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := s.handleLine(ctx, []byte(line), writer); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return fmt.Errorf("flush mcp response: %w", err)
		}
	}
}

func (s *Server) handleLine(ctx context.Context, line []byte, out *bufio.Writer) error {
	_ = ctx
	var request jsonRPCRequest
	if err := json.Unmarshal(line, &request); err != nil {
		return writeResponse(out, jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   protocolError(-32700, "parse error"),
		})
	}
	if request.JSONRPC != "2.0" {
		return writeResponse(out, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      request.ID,
			Error:   protocolError(-32600, "invalid request"),
		})
	}
	if request.Method == "" {
		return writeResponse(out, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      request.ID,
			Error:   protocolError(-32600, "invalid request"),
		})
	}
	if len(request.ID) == 0 {
		return s.handleNotification(request.Method, request.Params)
	}
	result, errEnvelope, err := s.handleRequest(request.Method, request.Params)
	if err != nil {
		return err
	}
	response := jsonRPCResponse{JSONRPC: "2.0", ID: request.ID, Result: result}
	if errEnvelope != nil {
		response.Result = nil
		response.Error = errEnvelope
	}
	return writeResponse(out, response)
}

func (s *Server) handleNotification(method string, params json.RawMessage) error {
	switch method {
	case "notifications/initialized", "notifications/cancelled":
		return nil
	default:
		_ = params
		return nil
	}
}

func (s *Server) handleRequest(method string, params json.RawMessage) (any, *jsonRPCError, error) {
	switch method {
	case "initialize":
		return s.handleInitialize(params)
	case "ping":
		return map[string]any{}, nil, nil
	case "tools/list":
		return s.handleToolsList(params)
	case "tools/call":
		return s.handleToolsCall(params)
	default:
		return nil, protocolError(-32601, fmt.Sprintf("method not found: %s", method)), nil
	}
}

func (s *Server) handleInitialize(params json.RawMessage) (any, *jsonRPCError, error) {
	_ = params
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    s.serverName,
			"version": s.serverVersion,
		},
	}, nil, nil
}

func (s *Server) handleToolsList(params json.RawMessage) (any, *jsonRPCError, error) {
	_ = params
	tools := make([]map[string]any, 0, len(mcpfactorysession.DiscoverTools())+len(mcpfactorysession.DiscoverCompatibilityAliases()))
	for _, tool := range mcpfactorysession.DiscoverTools() {
		tools = append(tools, mcpToolDescriptor(tool.Name, tool.Description, tool.InputSchema))
	}
	for _, alias := range mcpfactorysession.DiscoverCompatibilityAliases() {
		canonical, ok := mcpfactorysession.ToolByName(alias.CanonicalName)
		if !ok {
			continue
		}
		tools = append(tools, mcpToolDescriptor(alias.Name, alias.Description, canonical.InputSchema))
	}
	return map[string]any{"tools": tools}, nil, nil
}

func mcpToolDescriptor(name, description string, inputSchema map[string]any) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": inputSchema,
	}
}

type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolsCall(params json.RawMessage) (any, *jsonRPCError, error) {
	var request toolsCallParams
	if err := json.Unmarshal(params, &request); err != nil {
		return nil, protocolError(-32602, "invalid params"), nil
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return nil, protocolError(-32602, "tool name is required"), nil
	}
	raw, err := s.client.CallTool(name, request.Arguments)
	if err != nil {
		return mcpToolErrorResult(err.Error()), nil, nil
	}
	return mcpToolSuccessResult(raw), nil, nil
}

func mcpToolSuccessResult(raw json.RawMessage) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": string(raw),
			},
		},
		"isError": false,
	}
}

func mcpToolErrorResult(message string) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": message,
			},
		},
		"isError": true,
	}
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

func protocolError(code int, message string) *jsonRPCError {
	return &jsonRPCError{Code: code, Message: message}
}

func writeResponse(out *bufio.Writer, response jsonRPCResponse) error {
	encoded, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal mcp response: %w", err)
	}
	if _, err := out.Write(encoded); err != nil {
		return fmt.Errorf("write mcp response: %w", err)
	}
	if err := out.WriteByte('\n'); err != nil {
		return fmt.Errorf("terminate mcp response: %w", err)
	}
	return nil
}
