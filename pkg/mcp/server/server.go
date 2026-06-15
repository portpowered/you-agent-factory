// Package server exposes the canonical repo-owned MCP stdio serve path for
// dynamic workflow preview tools.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	mcpworkflow "github.com/portpowered/infinite-you/pkg/mcp/workflow"
)

const (
	serverName    = "you-agent-factory"
	serverVersion = "1.0.0"
)

// NewPreviewServer constructs one MCP server that advertises the current workflow
// preview tool catalog backed by pkg/mcp/workflow handlers.
func NewPreviewServer() *sdkmcp.Server {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, &sdkmcp.ServerOptions{
		Instructions: "Factory preview MCP tools for JavaScript orchestrator source validation and start-preview checks before Factory Session execution.",
	})

	for _, tool := range mcpworkflow.DiscoverTools() {
		registerPreviewTool(server, tool)
	}
	return server
}

func registerPreviewTool(server *sdkmcp.Server, tool mcpfactorysession.ToolDefinition) {
	toolName := tool.Name
	server.AddTool(&sdkmcp.Tool{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: tool.InputSchema,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		raw, err := mcpworkflow.CallTool(toolName, req.Params.Arguments)
		if err != nil {
			return nil, err
		}
		return previewToolResult(raw)
	})
}

func previewToolResult(raw json.RawMessage) (*sdkmcp.CallToolResult, error) {
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode preview tool response: %w", err)
	}
	result := &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: string(raw)},
		},
	}
	if envelope.Error != nil {
		result.IsError = true
	}
	return result, nil
}

// ServeStdio runs the preview MCP server over stdin/stdout until the client
// disconnects or the context is canceled.
func ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	server := NewPreviewServer()
	transport := &sdkmcp.IOTransport{
		Reader: nopCloserReader{Reader: in},
		Writer: nopCloserWriter{Writer: out},
	}
	return server.Run(ctx, transport)
}

type nopCloserReader struct {
	io.Reader
}

func (nopCloserReader) Close() error { return nil }

type nopCloserWriter struct {
	io.Writer
}

func (nopCloserWriter) Close() error { return nil }
