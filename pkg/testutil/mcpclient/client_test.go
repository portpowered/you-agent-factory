package mcpclient

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
)

const testTimeout = 5 * time.Second

func TestClientConnectsToRealServerThroughCallerSuppliedPipes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	serverInput, clientWriter := io.Pipe()
	clientReader, serverOutput := io.Pipe()
	server := newRealServer(t)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ServeStdio(ctx, serverInput, serverOutput)
	}()

	client, err := Connect(ctx, Pipes{Reader: clientReader, Writer: clientWriter}, Options{
		Name:    "sdk-client-test",
		Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	result := client.InitializeResult()
	if result.ProtocolVersion != "2024-11-05" {
		t.Fatalf("protocol version = %q, want 2024-11-05", result.ProtocolVersion)
	}
	if result.ServerInfo == nil || result.ServerInfo.Name != "pipe-test-server" {
		t.Fatalf("server info = %#v, want pipe-test-server", result.ServerInfo)
	}
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if err := client.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("ServeStdio() error = %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("ServeStdio() did not stop after client close: %v", ctx.Err())
	}
}

func TestClientErrorsRetainLifecycleStage(t *testing.T) {
	_, err := Connect(context.Background(), Pipes{}, Options{})
	assertErrorStage(t, err, StageSetup)

	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "not-json"}}}
	err = DecodeTextResult(result, &struct{}{})
	assertErrorStage(t, err, StageToolDecoding)
}

func TestConnectClassifiesInitializationEOFAsProtocolExchange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	serverInput, clientWriter := io.Pipe()
	clientReader, serverOutput := io.Pipe()
	go func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(serverInput, 1))
		_ = serverOutput.Close()
		_ = serverInput.Close()
	}()

	_, err := Connect(ctx, Pipes{Reader: clientReader, Writer: clientWriter}, Options{})
	assertErrorStage(t, err, StageProtocolExchange)
}

func newRealServer(t *testing.T) *mcpserver.Server {
	t.Helper()
	server, err := mcpserver.New(mcpserver.Options{
		Client:        mcpfactorysession.NewClient(),
		ServerName:    "pipe-test-server",
		ServerVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	return server
}

func assertErrorStage(t *testing.T, err error, want Stage) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want stage %q", want)
	}
	var clientErr *Error
	if !errors.As(err, &clientErr) {
		t.Fatalf("error = %T %v, want *Error", err, err)
	}
	if clientErr.Stage != want {
		t.Fatalf("error stage = %q, want %q", clientErr.Stage, want)
	}
	if !strings.Contains(err.Error(), string(want)) {
		t.Fatalf("error %q does not retain stage %q", err, want)
	}
}
