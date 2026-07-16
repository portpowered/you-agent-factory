package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
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

func TestClientNegotiatesDiscoversAndCorrelatesToolTrafficOverRealStdio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	client, requests, responses, serverErr := connectRecordingClient(t, ctx)
	assertInitializeResult(t, client.InitializeResult())
	listResult, callResult := runConcurrentOperations(t, ctx, client)
	assertDiscoveredTool(t, listResult, mcpfactorysession.ToolListSessions)
	assertSuccessfulTextToolResult(t, callResult)
	assertConversationFrames(t, requests.frames(), responses.frames())
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

func connectRecordingClient(t *testing.T, ctx context.Context) (*Client, *frameRecorder, *frameRecorder, <-chan error) {
	t.Helper()
	serverInput, pipeWriter := io.Pipe()
	proxyInput, serverOutput := io.Pipe()
	pipeReader, proxyOutput := io.Pipe()
	requests := newFrameRecorder(2)
	responses := newFrameRecorder(0)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- newRealServer(t).ServeStdio(ctx, serverInput, serverOutput)
		_ = serverOutput.Close()
	}()
	go forwardResponsesAfterInitialize(proxyInput, proxyOutput, requests.ready)
	client, err := Connect(ctx, Pipes{
		Reader: &recordingReader{source: pipeReader, recorder: responses, gate: requests.ready},
		Writer: &recordingWriter{destination: pipeWriter, recorder: requests},
	}, Options{Name: "sdk-conversation-test", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	return client, requests, responses, serverErr
}

func assertInitializeResult(t *testing.T, initialize *mcp.InitializeResult) {
	t.Helper()
	if initialize.ProtocolVersion != "2024-11-05" {
		t.Fatalf("protocol version = %q, want 2024-11-05", initialize.ProtocolVersion)
	}
	if initialize.ServerInfo == nil || initialize.ServerInfo.Name != "pipe-test-server" || initialize.ServerInfo.Version != "1.0.0" {
		t.Fatalf("server info = %#v, want pipe-test-server 1.0.0", initialize.ServerInfo)
	}
	if initialize.Capabilities == nil || initialize.Capabilities.Tools == nil {
		t.Fatalf("server capabilities = %#v, want tools capability", initialize.Capabilities)
	}
}

type operationResult struct {
	list *mcp.ListToolsResult
	call *mcp.CallToolResult
	err  error
}

func runConcurrentOperations(t *testing.T, ctx context.Context, client *Client) (*mcp.ListToolsResult, *mcp.CallToolResult) {
	t.Helper()
	results := make(chan operationResult, 2)
	go func() {
		result, err := client.ListTools(ctx)
		results <- operationResult{list: result, err: err}
	}()
	go func() {
		result, err := client.CallTool(ctx, mcpfactorysession.ToolListSessions, map[string]any{})
		results <- operationResult{call: result, err: err}
	}()
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent SDK operation errors = %v, %v", first.err, second.err)
	}
	if first.list != nil {
		return first.list, second.call
	}
	return second.list, first.call
}

func assertSuccessfulTextToolResult(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if result == nil || result.IsError {
		t.Fatalf("CallTool() result = %#v, want isError false", result)
	}
	var toolResponse mcpfactorysession.ToolResponse[json.RawMessage]
	if err := DecodeTextResult(result, &toolResponse); err != nil {
		t.Fatalf("DecodeTextResult() error = %v", err)
	}
	if toolResponse.Error == nil || !strings.Contains(toolResponse.Error.Message, "unavailable") {
		t.Fatalf("tool text payload = %#v, want current text-only Factory Session response", toolResponse)
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

type frameRecorder struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	recorded  [][]byte
	operation int
	target    int
	ready     chan struct{}
	once      sync.Once
}

func newFrameRecorder(operationTarget int) *frameRecorder {
	recorder := &frameRecorder{target: operationTarget, ready: make(chan struct{})}
	if operationTarget == 0 {
		close(recorder.ready)
	}
	return recorder
}

func (r *frameRecorder) record(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = r.buffer.Write(data)
	for {
		line, err := r.buffer.ReadBytes('\n')
		if err != nil {
			_, _ = r.buffer.Write(line)
			return
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		r.recorded = append(r.recorded, bytes.Clone(line))
		var frame rpcFrame
		if json.Unmarshal(line, &frame) == nil && (frame.Method == "tools/list" || frame.Method == "tools/call") {
			r.operation++
			if r.operation == r.target {
				r.once.Do(func() { close(r.ready) })
			}
		}
	}
}

func (r *frameRecorder) frames() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]byte(nil), r.recorded...)
}

func (r *frameRecorder) responseCount() int {
	count := 0
	for _, encoded := range r.frames() {
		var frame rpcFrame
		if json.Unmarshal(encoded, &frame) == nil && frame.ID != nil && frame.Method == "" {
			count++
		}
	}
	return count
}

type recordingWriter struct {
	destination io.WriteCloser
	recorder    *frameRecorder
}

func (w *recordingWriter) Write(data []byte) (int, error) {
	w.recorder.record(data)
	return w.destination.Write(data)
}

func (w *recordingWriter) Close() error { return w.destination.Close() }

type recordingReader struct {
	source   io.ReadCloser
	recorder *frameRecorder
	gate     <-chan struct{}
}

func (r *recordingReader) Read(data []byte) (int, error) {
	n, err := r.source.Read(data)
	if n > 0 {
		r.recorder.record(data[:n])
		if r.gate != nil && r.recorder.responseCount() > 1 {
			select {
			case <-r.gate:
			case <-time.After(testTimeout):
				return 0, context.DeadlineExceeded
			}
		}
	}
	return n, err
}

func (r *recordingReader) Close() error { return r.source.Close() }

type rpcFrame struct {
	ID     any             `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
}

func assertDiscoveredTool(t *testing.T, result *mcp.ListToolsResult, name string) {
	t.Helper()
	if result == nil {
		t.Fatal("ListTools() result = nil")
	}
	for _, tool := range result.Tools {
		if tool.Name == name {
			return
		}
	}
	t.Fatalf("ListTools() missing canonical tool %q", name)
}

func assertConversationFrames(t *testing.T, requests, responses [][]byte) {
	t.Helper()
	requestIDs := map[string]string{}
	initialized := 0
	for _, encoded := range requests {
		var frame rpcFrame
		if err := json.Unmarshal(encoded, &frame); err != nil {
			t.Fatalf("decode SDK request %q: %v", encoded, err)
		}
		if frame.Method == "notifications/initialized" {
			initialized++
		}
		if frame.Method == "tools/list" || frame.Method == "tools/call" {
			requestIDs[fmt.Sprint(frame.ID)] = frame.Method
		}
	}
	if initialized != 1 {
		t.Fatalf("initialized notification count = %d, want 1", initialized)
	}
	if len(requestIDs) != 2 {
		t.Fatalf("operation request identities = %#v, want two distinct IDs", requestIDs)
	}
	responseIDs := map[string]bool{}
	for _, encoded := range responses {
		var frame rpcFrame
		if err := json.Unmarshal(encoded, &frame); err != nil {
			t.Fatalf("decode server response %q: %v", encoded, err)
		}
		responseIDs[fmt.Sprint(frame.ID)] = true
	}
	for id, method := range requestIDs {
		if !responseIDs[id] {
			t.Errorf("response missing request identity %s for %s", id, method)
		}
	}
	if len(responses) != 3 {
		t.Fatalf("server response count = %d, want initialize plus two operations and no notification response", len(responses))
	}
}
