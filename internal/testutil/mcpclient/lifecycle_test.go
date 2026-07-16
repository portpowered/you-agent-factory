package mcpclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestClientPreservesProtocolAndToolErrorBoundariesOverRealStdio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	client, requests, responses, serverErr := connectUnblockedRecordingClient(t, ctx)

	err := client.SetLoggingLevel(ctx, mcp.LoggingLevel("info"))
	assertErrorStage(t, err, StageProtocolExchange)
	var protocolErr *jsonrpc.Error
	if !errors.As(err, &protocolErr) {
		t.Fatalf("SetLoggingLevel() error = %T %v, want *jsonrpc.Error", err, err)
	}
	if protocolErr.Code != jsonrpc.CodeMethodNotFound {
		t.Fatalf("SetLoggingLevel() protocol code = %d, want %d", protocolErr.Code, jsonrpc.CodeMethodNotFound)
	}

	toolResult, err := client.CallTool(ctx, "unsupported.tool", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool(unsupported) error = %v, want normal tool result", err)
	}
	assertRejectedTextToolResult(t, toolResult, `unsupported tool "unsupported.tool"`)
	assertErrorBoundaryFrames(t, requests.frames(), responses.frames())
	closeClientAndWaitForServer(t, ctx, client, serverErr)
}

func TestClientCancellationReleasesPendingRequestAndTearsDownRealStdio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	serverInput, pipeWriter := io.Pipe()
	proxyInput, serverOutput := io.Pipe()
	pipeReader, proxyOutput := io.Pipe()
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- newRealServer(t).ServeStdio(ctx, serverInput, serverOutput)
		_ = serverOutput.Close()
	}()

	responseGate := make(chan struct{})
	defer func() {
		select {
		case <-responseGate:
		default:
			close(responseGate)
		}
	}()
	go forwardResponsesAfterInitialize(proxyInput, proxyOutput, responseGate)
	requests := newMethodObservingWriter(pipeWriter, "ping")
	client, err := Connect(ctx, Pipes{
		Reader: pipeReader,
		Writer: requests,
	}, Options{Name: "sdk-cancellation-test", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	operationCtx, cancelOperation := context.WithCancel(ctx)
	operationErr := make(chan error, 1)
	go func() { operationErr <- client.Ping(operationCtx) }()
	waitForSignal(t, ctx, requests.observed, "SDK ping request")
	cancelOperation()
	select {
	case err := <-operationErr:
		assertErrorStage(t, err, StageProtocolExchange)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Ping() error = %v, want context cancellation", err)
		}
	case <-ctx.Done():
		t.Fatalf("Ping() did not release its pending request: %v", ctx.Err())
	}
	assertCancellationTargetsRequest(t, requests.frames(), "ping")
	close(responseGate)
	closeClientAndWaitForServer(t, ctx, client, serverErr)
}

func TestCallerInputCloseEndsRealStdioWithoutPendingClientOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	serverInput, clientWriter := io.Pipe()
	clientReader, serverOutput := io.Pipe()
	serverErr := make(chan error, 1)
	go func() { serverErr <- newRealServer(t).ServeStdio(ctx, serverInput, serverOutput) }()
	client, err := Connect(ctx, Pipes{Reader: clientReader, Writer: clientWriter}, Options{})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	if err := clientWriter.Close(); err != nil {
		t.Fatalf("close caller-owned client input: %v", err)
	}
	waitForServer(t, ctx, serverErr)
	if err := client.Close(ctx); err != nil {
		t.Fatalf("Close() after input EOF error = %v", err)
	}
}

func TestMalformedProtocolRawFrameRetainsRequestIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	serverInput, rawClientInput := io.Pipe()
	rawClientOutput, serverOutput := io.Pipe()
	serverErr := make(chan error, 1)
	go func() { serverErr <- newRealServer(t).ServeStdio(ctx, serverInput, serverOutput) }()

	const malformedProtocolFrame = `{"jsonrpc":"1.0","id":"malformed-1","method":"ping"}` + "\n"
	if _, err := io.WriteString(rawClientInput, malformedProtocolFrame); err != nil {
		t.Fatalf("write intentionally malformed raw frame: %v", err)
	}
	encoded, err := bufio.NewReader(rawClientOutput).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read malformed-protocol response: %v", err)
	}
	var response boundaryFrame
	if err := json.Unmarshal(bytes.TrimSpace(encoded), &response); err != nil {
		t.Fatalf("decode malformed-protocol response: %v", err)
	}
	if fmt.Sprint(response.ID) != "malformed-1" || response.Error == nil || response.Error.Code != jsonrpc.CodeInvalidRequest {
		t.Fatalf("malformed-protocol response = %#v, want correlated invalid-request error", response)
	}
	if err := rawClientInput.Close(); err != nil {
		t.Fatalf("close malformed-protocol input: %v", err)
	}
	waitForServer(t, ctx, serverErr)
	_ = rawClientOutput.Close()
}

func connectUnblockedRecordingClient(t *testing.T, ctx context.Context) (*Client, *frameRecorder, *frameRecorder, <-chan error) {
	t.Helper()
	serverInput, pipeWriter := io.Pipe()
	pipeReader, serverOutput := io.Pipe()
	requests := newFrameRecorder(0)
	responses := newFrameRecorder(0)
	serverErr := make(chan error, 1)
	go func() { serverErr <- newRealServer(t).ServeStdio(ctx, serverInput, serverOutput) }()
	client, err := Connect(ctx, Pipes{
		Reader: &recordingReader{source: pipeReader, recorder: responses, gate: requests.ready},
		Writer: &recordingWriter{destination: pipeWriter, recorder: requests},
	}, Options{Name: "sdk-error-boundary-test", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	return client, requests, responses, serverErr
}

func assertRejectedTextToolResult(t *testing.T, result *mcp.CallToolResult, wantText string) {
	t.Helper()
	if result == nil || !result.IsError || len(result.Content) != 1 {
		t.Fatalf("CallTool() result = %#v, want one text item with isError true", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, wantText) {
		t.Fatalf("CallTool() content = %#v, want text containing %q", result.Content, wantText)
	}
}

type boundaryFrame struct {
	ID     any    `json:"id"`
	Method string `json:"method"`
	Params struct {
		RequestID any `json:"requestId"`
	} `json:"params"`
	Error *struct {
		Code int64 `json:"code"`
	} `json:"error"`
}

func assertErrorBoundaryFrames(t *testing.T, requests, responses [][]byte) {
	t.Helper()
	requestIDs := make(map[string]any)
	for _, encoded := range requests {
		var frame boundaryFrame
		if err := json.Unmarshal(encoded, &frame); err != nil {
			t.Fatalf("decode SDK request %q: %v", encoded, err)
		}
		if frame.Method == "logging/setLevel" || frame.Method == "tools/call" {
			requestIDs[frame.Method] = frame.ID
		}
	}
	if len(requestIDs) != 2 || fmt.Sprint(requestIDs["logging/setLevel"]) == fmt.Sprint(requestIDs["tools/call"]) {
		t.Fatalf("error-boundary request identities = %#v, want distinct logging and tool IDs", requestIDs)
	}
	responseErrors := make(map[string]*struct {
		Code int64 `json:"code"`
	})
	for _, encoded := range responses {
		var frame boundaryFrame
		if err := json.Unmarshal(encoded, &frame); err != nil {
			t.Fatalf("decode server response %q: %v", encoded, err)
		}
		responseErrors[fmt.Sprint(frame.ID)] = frame.Error
	}
	if rpcErr := responseErrors[fmt.Sprint(requestIDs["logging/setLevel"])]; rpcErr == nil || rpcErr.Code != jsonrpc.CodeMethodNotFound {
		t.Fatalf("logging response error = %#v, want method-not-found", rpcErr)
	}
	if rpcErr := responseErrors[fmt.Sprint(requestIDs["tools/call"])]; rpcErr != nil {
		t.Fatalf("tool/domain response error = %#v, want normal CallToolResult", rpcErr)
	}
}

func forwardResponsesAfterInitialize(source io.ReadCloser, destination io.WriteCloser, gate <-chan struct{}) {
	defer source.Close()
	defer destination.Close()
	scanner := bufio.NewScanner(source)
	initialized := false
	for scanner.Scan() {
		if initialized {
			<-gate
		}
		if _, err := destination.Write(append(bytes.Clone(scanner.Bytes()), '\n')); err != nil {
			return
		}
		initialized = true
	}
}

type methodObservingWriter struct {
	destination io.WriteCloser
	method      string
	observed    chan struct{}
	once        sync.Once
	mu          sync.Mutex
	buffer      bytes.Buffer
	recorded    [][]byte
}

func newMethodObservingWriter(destination io.WriteCloser, method string) *methodObservingWriter {
	return &methodObservingWriter{destination: destination, method: method, observed: make(chan struct{})}
}

func (w *methodObservingWriter) Write(data []byte) (int, error) {
	w.record(data)
	return w.destination.Write(data)
}

func (w *methodObservingWriter) Close() error { return w.destination.Close() }

func (w *methodObservingWriter) record(data []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.buffer.Write(data)
	for {
		line, err := w.buffer.ReadBytes('\n')
		if err != nil {
			_, _ = w.buffer.Write(line)
			return
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		w.recorded = append(w.recorded, bytes.Clone(line))
		var frame boundaryFrame
		if json.Unmarshal(line, &frame) == nil && frame.Method == w.method {
			w.once.Do(func() { close(w.observed) })
		}
	}
}

func (w *methodObservingWriter) frames() [][]byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([][]byte(nil), w.recorded...)
}

func assertCancellationTargetsRequest(t *testing.T, frames [][]byte, method string) {
	t.Helper()
	var requestID, cancelledID any
	for _, encoded := range frames {
		var frame boundaryFrame
		if err := json.Unmarshal(encoded, &frame); err != nil {
			t.Fatalf("decode cancellation frame %q: %v", encoded, err)
		}
		switch frame.Method {
		case method:
			requestID = frame.ID
		case "notifications/cancelled":
			cancelledID = frame.Params.RequestID
		}
	}
	if requestID == nil || fmt.Sprint(cancelledID) != fmt.Sprint(requestID) {
		t.Fatalf("cancellation request ID = %v, want active %s request ID %v", cancelledID, method, requestID)
	}
}

func closeClientAndWaitForServer(t *testing.T, ctx context.Context, client *Client, serverErr <-chan error) {
	t.Helper()
	if err := client.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	waitForServer(t, ctx, serverErr)
}

func waitForServer(t *testing.T, ctx context.Context, serverErr <-chan error) {
	t.Helper()
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("ServeStdio() error = %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("ServeStdio() did not finish: %v", ctx.Err())
	}
}

func waitForSignal(t *testing.T, ctx context.Context, signal <-chan struct{}, boundary string) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s: %v", boundary, ctx.Err())
	}
}
