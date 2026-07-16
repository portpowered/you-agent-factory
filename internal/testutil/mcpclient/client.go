// Package mcpclient provides an SDK-backed MCP client for functional tests.
package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultClientName    = "you-functional-test"
	defaultClientVersion = "dev"
)

// Stage identifies the client lifecycle boundary at which an operation failed.
type Stage string

const (
	StageSetup             Stage = "setup"
	StageProtocolExchange  Stage = "protocol exchange"
	StageTransportShutdown Stage = "transport shutdown"
	StageToolDecoding      Stage = "tool decoding"
)

// Error preserves the lifecycle stage and operation for an SDK client failure.
type Error struct {
	Stage Stage
	Op    string
	Err   error
}

func (e *Error) Error() string {
	return fmt.Sprintf("mcp client %s during %s: %v", e.Op, e.Stage, e.Err)
}

// Unwrap exposes the underlying SDK, transport, or decoding error.
func (e *Error) Unwrap() error { return e.Err }

// Pipes are caller-supplied endpoints connected to a newline-delimited MCP server.
// Reader receives server stdout and Writer sends server stdin.
type Pipes struct {
	Reader io.ReadCloser
	Writer io.WriteCloser
}

// Options identifies the functional-test client during MCP initialization.
type Options struct {
	Name    string
	Version string
}

// Client is an initialized SDK client attached to caller-supplied pipes.
type Client struct {
	session *mcp.ClientSession
}

// Connect initializes an MCP session over pipes without constructing or owning
// a process, application service, server handler, or graph host.
func Connect(ctx context.Context, pipes Pipes, options Options) (*Client, error) {
	if ctx == nil {
		return nil, clientError(StageSetup, "connect", errors.New("context is required"))
	}
	if pipes.Reader == nil {
		return nil, clientError(StageSetup, "connect", errors.New("server output reader is required"))
	}
	if pipes.Writer == nil {
		return nil, clientError(StageSetup, "connect", errors.New("server input writer is required"))
	}
	if options.Name == "" {
		options.Name = defaultClientName
	}
	if options.Version == "" {
		options.Version = defaultClientVersion
	}

	sdkClient := mcp.NewClient(&mcp.Implementation{
		Name:    options.Name,
		Version: options.Version,
	}, nil)
	session, err := sdkClient.Connect(ctx, &mcp.IOTransport{
		Reader: pipes.Reader,
		Writer: pipes.Writer,
	}, nil)
	if err != nil {
		return nil, clientError(StageProtocolExchange, "initialize", err)
	}
	return &Client{session: session}, nil
}

// InitializeResult returns the protocol negotiation result from Connect.
func (c *Client) InitializeResult() *mcp.InitializeResult {
	return c.session.InitializeResult()
}

// Ping performs one SDK-backed MCP protocol exchange.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.session.Ping(ctx, nil); err != nil {
		return clientError(StageProtocolExchange, "ping", err)
	}
	return nil
}

// ListTools discovers the tools published by the attached MCP server.
func (c *Client) ListTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	result, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, clientError(StageProtocolExchange, "list tools", err)
	}
	return result, nil
}

// CallTool invokes one discovered tool through the SDK session.
func (c *Client) CallTool(ctx context.Context, name string, arguments any) (*mcp.CallToolResult, error) {
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, clientError(StageProtocolExchange, "call tool", err)
	}
	return result, nil
}

// SetLoggingLevel asks the attached server to change its MCP logging level.
func (c *Client) SetLoggingLevel(ctx context.Context, level mcp.LoggingLevel) error {
	if err := c.session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: level}); err != nil {
		return clientError(StageProtocolExchange, "set logging level", err)
	}
	return nil
}

// DecodeTextResult decodes the single text item used by the current server.
func DecodeTextResult(result *mcp.CallToolResult, target any) error {
	if result == nil {
		return clientError(StageToolDecoding, "decode result", errors.New("tool result is required"))
	}
	if len(result.Content) != 1 {
		return clientError(StageToolDecoding, "decode result", fmt.Errorf("content item count = %d, want 1", len(result.Content)))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return clientError(StageToolDecoding, "decode result", fmt.Errorf("content type = %T, want *mcp.TextContent", result.Content[0]))
	}
	if target == nil {
		return clientError(StageToolDecoding, "decode result", errors.New("decode target is required"))
	}
	if err := json.Unmarshal([]byte(text.Text), target); err != nil {
		return clientError(StageToolDecoding, "decode result", err)
	}
	return nil
}

// Close shuts down the SDK session and its attached transport within the
// caller's lifecycle bound.
func (c *Client) Close(ctx context.Context) error {
	if ctx == nil {
		return clientError(StageTransportShutdown, "close", errors.New("context is required"))
	}
	closed := make(chan error, 1)
	go func() { closed <- c.session.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			return clientError(StageTransportShutdown, "close", err)
		}
		return nil
	case <-ctx.Done():
		return clientError(StageTransportShutdown, "close", ctx.Err())
	}
}

func clientError(stage Stage, op string, err error) error {
	return &Error{Stage: stage, Op: op, Err: err}
}
