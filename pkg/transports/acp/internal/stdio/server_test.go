package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

type recordingLogger struct {
	entries []logEntry
}

type logEntry struct {
	level   string
	message string
	fields  map[string]any
}

func (l *recordingLogger) Debug(msg string, keysAndValues ...any) {
	l.record("debug", msg, keysAndValues...)
}
func (l *recordingLogger) Info(msg string, keysAndValues ...any) {
	l.record("info", msg, keysAndValues...)
}
func (l *recordingLogger) Warn(msg string, keysAndValues ...any) {
	l.record("warn", msg, keysAndValues...)
}
func (l *recordingLogger) Error(msg string, keysAndValues ...any) {
	l.record("error", msg, keysAndValues...)
}
func (l *recordingLogger) Verbose(msg string, keysAndValues ...any) {
	l.record("verbose", msg, keysAndValues...)
}

func (l *recordingLogger) record(level, msg string, keysAndValues ...any) {
	fields := map[string]any{}
	for index := 0; index+1 < len(keysAndValues); index += 2 {
		key, ok := keysAndValues[index].(string)
		if !ok {
			continue
		}
		fields[key] = keysAndValues[index+1]
	}
	l.entries = append(l.entries, logEntry{level: level, message: msg, fields: fields})
}

var _ logging.Logger = (*recordingLogger)(nil)

func TestNewPerformsNoIO(t *testing.T) {
	logger := &recordingLogger{}
	server := New(logger, nil, nil, nil, nil, nil, nil)
	if server == nil {
		t.Fatal("New() returned nil")
	}
	if len(logger.entries) != 0 {
		t.Fatalf("New() logged %d entries, want 0: construction must perform no I/O", len(logger.entries))
	}
}

func TestServeRejectsMissingStreams(t *testing.T) {
	server := New(nil, nil, nil, nil, nil, nil, nil)
	buf := &bytes.Buffer{}

	cases := []struct {
		name string
		in   io.Reader
		out  io.Writer
	}{
		{name: "missing input", in: nil, out: buf},
		{name: "missing output", in: strings.NewReader(""), out: nil},
		{name: "missing both", in: nil, out: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := server.Serve(context.Background(), tc.in, tc.out)
			if err == nil {
				t.Fatal("Serve() error = nil, want an actionable error for missing streams")
			}
			if !errors.Is(err, ErrStreamsRequired) {
				t.Fatalf("Serve() error = %v, want ErrStreamsRequired", err)
			}
		})
	}
}

func TestServeRejectsNilServer(t *testing.T) {
	var server *Server
	err := server.Serve(context.Background(), strings.NewReader(""), &bytes.Buffer{})
	if err == nil {
		t.Fatal("Serve() error = nil, want an error for a nil server")
	}
}

func TestServeReturnsOnCleanEOF(t *testing.T) {
	server := New(nil, nil, nil, nil, nil, nil, nil)
	err := server.Serve(context.Background(), strings.NewReader("first line\nsecond line\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Serve() error = %v, want nil on clean EOF", err)
	}
}

func TestServeRejectsAlreadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	server := New(nil, nil, nil, nil, nil, nil, nil)
	err := server.Serve(ctx, strings.NewReader(""), &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve() error = %v, want context.Canceled", err)
	}
}

// writerFunc adapts a function to io.Writer.
type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// TestServeRejectsContextCancelledBetweenReads proves the read loop's own
// per-iteration ctx.Err() check (distinct from Serve's one-time pre-check in
// TestServeRejectsAlreadyCancelledContext, and from the mid-read cancellation
// in TestServeReturnsContextErrorOnMidReadCancellation) actually stops a
// second line from ever being read once the context is cancelled between two
// requests on the same connection. It cancels ctx from inside the response
// write for the first line -- synchronous with serveConnection's own
// goroutine, so no timing or extra synchronization is needed -- and asserts
// only that first response was written before Serve returns ctx's error.
func TestServeRejectsContextCancelledBetweenReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"unknown/method"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"unknown/method"}` + "\n",
	)

	var buf bytes.Buffer
	writeCount := 0
	out := writerFunc(func(p []byte) (int, error) {
		writeCount++
		cancel()
		return buf.Write(p)
	})

	server := New(nil, nil, nil, nil, nil, nil, nil)
	err := server.Serve(ctx, in, out)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve() error = %v, want context.Canceled", err)
	}
	if writeCount != 1 {
		t.Fatalf("write count = %d, want exactly 1 (second line must never be read once ctx is cancelled)", writeCount)
	}
}

func TestServeMintsDistinctConnectionIDsPerInvocation(t *testing.T) {
	logger := &recordingLogger{}
	server := New(logger, nil, nil, nil, nil, nil, nil)

	if err := server.Serve(context.Background(), strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatalf("Serve() first call error = %v", err)
	}
	if err := server.Serve(context.Background(), strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatalf("Serve() second call error = %v", err)
	}

	first := connectionIDAt(t, logger, 0)
	second := connectionIDAt(t, logger, 2)

	if first == "" || second == "" {
		t.Fatalf("expected non-empty connection ids, got %q and %q", first, second)
	}
	if first == second {
		t.Fatalf("expected distinct connection ids across invocations, got %q both times", first)
	}
}

func TestServeLogsStartAndTerminalOutcomeWithoutPayload(t *testing.T) {
	logger := &recordingLogger{}
	server := New(logger, nil, nil, nil, nil, nil, nil)

	payload := "super-secret-prompt-content"
	if err := server.Serve(context.Background(), strings.NewReader(payload+"\n"), &bytes.Buffer{}); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	if len(logger.entries) != 2 {
		t.Fatalf("got %d log entries, want 2 (start and terminal)", len(logger.entries))
	}

	start := logger.entries[0]
	if start.message != "acp stdio connection started" {
		t.Fatalf("start message = %q, want the connection-started message", start.message)
	}
	if _, ok := start.fields["connectionId"]; !ok {
		t.Fatal("start log entry is missing connectionId")
	}

	terminal := logger.entries[1]
	if terminal.message != "acp stdio connection terminated" {
		t.Fatalf("terminal message = %q, want the connection-terminated message", terminal.message)
	}
	if terminal.fields["outcome"] != "eof" {
		t.Fatalf("terminal outcome = %v, want %q", terminal.fields["outcome"], "eof")
	}

	for _, entry := range logger.entries {
		for key, value := range entry.fields {
			if text, ok := value.(string); ok && strings.Contains(text, payload) {
				t.Fatalf("log field %q leaked request payload: %v", key, value)
			}
		}
	}
}

func connectionIDAt(t *testing.T, logger *recordingLogger, index int) string {
	t.Helper()
	if index >= len(logger.entries) {
		t.Fatalf("no log entry at index %d (have %d)", index, len(logger.entries))
	}
	value, ok := logger.entries[index].fields["connectionId"].(string)
	if !ok {
		t.Fatalf("log entry at index %d has no string connectionId field", index)
	}
	return value
}

// initializeSuccessResult is the exact expected result payload for a
// supported-version initialize request against the pinned honest P0
// text-first agent capability profile (mirrors
// internal/testutil/acpfixtures/testdata/initialization.json's accepted
// case): the pinned protocol version, an empty authentication-method list,
// and exactly the capabilities negotiation.Negotiate advertises -- no
// deferred capability.
const initializeSuccessResult = `{"protocolVersion":1,"authMethods":[],"agentCapabilities":{"_meta":{"portpowered.infinite-you/attachment-resume":true},"auth":{},"loadSession":true,"mcpCapabilities":{},"promptCapabilities":{},"sessionCapabilities":{"resume":{}}}}`

// initializeLine builds one complete newline-terminated JSON-RPC initialize
// request line carrying rawID as its id token (e.g. "1" or `"req-abc"`).
func initializeLine(rawID string) string {
	return fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%s,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":true,"writeTextFile":true},"terminal":true}}}`+"\n",
		rawID,
	)
}

// assertSingleResponseLine asserts out holds exactly one complete
// newline-terminated JSON object -- no interleaving, no partial frame -- and
// returns it decoded.
func assertSingleResponseLine(t *testing.T, out *bytes.Buffer) rpcMessage {
	t.Helper()
	lines := nonEmptyResponseLines(t, out)
	if len(lines) != 1 {
		t.Fatalf("got %d response lines, want 1: %q", len(lines), out.Bytes())
	}
	var resp rpcMessage
	if err := json.Unmarshal(lines[0], &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

// nonEmptyResponseLines splits out's contents into its newline-delimited
// frames, asserting the buffer's contents end in a trailing newline (so no
// partial trailing frame was ever written).
func nonEmptyResponseLines(t *testing.T, out *bytes.Buffer) [][]byte {
	t.Helper()
	data := out.Bytes()
	if len(data) == 0 {
		return nil
	}
	if data[len(data)-1] != '\n' {
		t.Fatalf("response output = %q, want every response newline-terminated", data)
	}
	trimmed := bytes.TrimSuffix(data, []byte("\n"))
	return bytes.Split(trimmed, []byte("\n"))
}

func TestServeRespondsToValidInitializeRequests(t *testing.T) {
	cases := []struct {
		name string
		id   string
	}{
		{name: "numeric id", id: "1"},
		{name: "string id", id: `"req-abc"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			server := New(nil, nil, nil, nil, nil, nil, nil)
			if err := server.Serve(context.Background(), strings.NewReader(initializeLine(tc.id)), out); err != nil {
				t.Fatalf("Serve() error = %v", err)
			}

			resp := assertSingleResponseLine(t, out)
			if resp.JSONRPC != "2.0" {
				t.Fatalf("jsonrpc = %q, want 2.0", resp.JSONRPC)
			}
			if string(resp.ID) != tc.id {
				t.Fatalf("id = %s, want %s", resp.ID, tc.id)
			}
			if resp.Error != nil {
				t.Fatalf("error = %+v, want a successful result", resp.Error)
			}
			assertJSONEqualStrings(t, initializeSuccessResult, string(resp.Result))
		})
	}
}

func TestServeFramesOneResponsePerCompleteInputLine(t *testing.T) {
	input := initializeLine("1") + initializeLine("2")

	out := &bytes.Buffer{}
	server := New(nil, nil, nil, nil, nil, nil, nil)
	if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	lines := nonEmptyResponseLines(t, out)
	if len(lines) != 2 {
		t.Fatalf("got %d response lines, want 2: %q", len(lines), out.Bytes())
	}
	for index, wantID := range []string{"1", "2"} {
		var resp rpcMessage
		if err := json.Unmarshal(lines[index], &resp); err != nil {
			t.Fatalf("unmarshal response %d: %v", index, err)
		}
		if string(resp.ID) != wantID {
			t.Fatalf("response %d id = %s, want %s", index, resp.ID, wantID)
		}
	}
}

func TestServeSkipsEmptyLines(t *testing.T) {
	input := "\n" + initializeLine("1") + "\n"

	out := &bytes.Buffer{}
	server := New(nil, nil, nil, nil, nil, nil, nil)
	if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	assertSingleResponseLine(t, out)
}

func TestServeIsolatesConnectionsReusingTheSameWireID(t *testing.T) {
	logger := &recordingLogger{}
	server := New(logger, nil, nil, nil, nil, nil, nil)

	firstOut := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(initializeLine("1")), firstOut); err != nil {
		t.Fatalf("first Serve() error = %v", err)
	}
	secondOut := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(initializeLine("1")), secondOut); err != nil {
		t.Fatalf("second Serve() error = %v", err)
	}

	firstConnID := connectionIDAt(t, logger, 0)
	secondConnID := connectionIDAt(t, logger, 2)
	if firstConnID == secondConnID {
		t.Fatalf("expected distinct connection ids across invocations reusing wire id 1, got %q both times", firstConnID)
	}

	for _, out := range []*bytes.Buffer{firstOut, secondOut} {
		resp := assertSingleResponseLine(t, out)
		if string(resp.ID) != "1" {
			t.Fatalf("response id = %s, want 1", resp.ID)
		}
		if resp.Error != nil {
			t.Fatalf("response error = %+v, want a successful result", resp.Error)
		}
		assertJSONEqualStrings(t, initializeSuccessResult, string(resp.Result))
	}
}

// TestServeRespondsMethodNotFoundForEveryUnimplementedMethod covers every
// deferred ACP method already listed in protocol.SupportedMethods (a
// forward-looking closed set for the whole future method surface, not what
// this transport slice actually implements -- see the Codebase Patterns
// entry on protocol.SupportedMethods) plus a method this transport never
// expects at all, proving all of them get method-not-found rather than
// being dispatched or hanging. "session/prompt" is excluded: it is now
// dispatched (for "/factory <value>" only), covered in session_prompt_test.go.
// "session/load" and "session/resume" are excluded the same way: they are
// now dispatched too, covered in session_load_test.go.
func TestServeRespondsMethodNotFoundForEveryUnimplementedMethod(t *testing.T) {
	methods := []string{
		"session/request_permission",
		"totally/unrecognized_method",
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			input := fmt.Sprintf(`{"jsonrpc":"2.0","id":9,"method":%q,"params":{}}`, method) + "\n"

			out := &bytes.Buffer{}
			server := New(nil, nil, nil, nil, nil, nil, nil)
			if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
				t.Fatalf("Serve() error = %v", err)
			}

			resp := assertSingleResponseLine(t, out)
			if string(resp.ID) != "9" {
				t.Fatalf("id = %s, want 9", resp.ID)
			}
			if resp.Error == nil || resp.Error.Code != -32601 {
				t.Fatalf("error = %+v, want method-not-found (-32601)", resp.Error)
			}
		})
	}
}

// TestServeRespondsWithParseErrorForMalformedJSON proves input that never
// parses as JSON at all gets a JSON-RPC parse error with a null id, since no
// id could ever be recovered from input this broken.
func TestServeRespondsWithParseErrorForMalformedJSON(t *testing.T) {
	out := &bytes.Buffer{}
	server := New(nil, nil, nil, nil, nil, nil, nil)
	if err := server.Serve(context.Background(), strings.NewReader("{not json\n"), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	resp := assertSingleResponseLine(t, out)
	if resp.Error == nil || resp.Error.Code != -32700 {
		t.Fatalf("error = %+v, want parse error (-32700)", resp.Error)
	}
	if string(resp.ID) != "null" {
		t.Fatalf("id = %s, want null for unparseable input", resp.ID)
	}
}

// TestServeRespondsWithInvalidRequestForStructurallyInvalidShapes proves
// input that parses as JSON but violates the JSON-RPC 2.0 request shape
// this transport requires gets an invalid-request response, correlated to
// the message's id only when that id token was itself syntactically valid.
func TestServeRespondsWithInvalidRequestForStructurallyInvalidShapes(t *testing.T) {
	cases := []struct {
		name   string
		line   string
		wantID string
	}{
		{
			name:   "wrong jsonrpc version with a valid id",
			line:   `{"jsonrpc":"1.0","id":5,"method":"initialize","params":{"protocolVersion":1}}` + "\n",
			wantID: "5",
		},
		{
			name:   "missing method with a valid id",
			line:   `{"jsonrpc":"2.0","id":6}` + "\n",
			wantID: "6",
		},
		{
			name:   "id-bearing session/cancel notification",
			line:   `{"jsonrpc":"2.0","id":7,"method":"session/cancel","params":{"sessionId":"s"}}` + "\n",
			wantID: "7",
		},
		{
			name:   "malformed id shape has no recoverable id",
			line:   `{"jsonrpc":"2.0","id":{},"method":"initialize","params":{"protocolVersion":1}}` + "\n",
			wantID: "null",
		},
		{
			name:   "valid JSON scalar, not a request object",
			line:   `1` + "\n",
			wantID: "null",
		},
		{
			name:   "valid JSON string, not a request object",
			line:   `"initialize"` + "\n",
			wantID: "null",
		},
		{
			name:   "valid JSON array, not a request object",
			line:   `[]` + "\n",
			wantID: "null",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			server := New(nil, nil, nil, nil, nil, nil, nil)
			if err := server.Serve(context.Background(), strings.NewReader(tc.line), out); err != nil {
				t.Fatalf("Serve() error = %v", err)
			}

			resp := assertSingleResponseLine(t, out)
			if resp.Error == nil || resp.Error.Code != -32600 {
				t.Fatalf("error = %+v, want invalid-request (-32600)", resp.Error)
			}
			if string(resp.ID) != tc.wantID {
				t.Fatalf("id = %s, want %s", resp.ID, tc.wantID)
			}
		})
	}
}

// TestServeRespondsWithInvalidParamsForBadInitializeParams proves missing
// and malformed initialize params both produce an invalid-params response
// correlated to the original valid request id.
func TestServeRespondsWithInvalidParamsForBadInitializeParams(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{name: "missing params", line: `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"},
		{name: "null params", line: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":null}` + "\n"},
		{name: "malformed params type", line: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":"not-an-object"}` + "\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			server := New(nil, nil, nil, nil, nil, nil, nil)
			if err := server.Serve(context.Background(), strings.NewReader(tc.line), out); err != nil {
				t.Fatalf("Serve() error = %v", err)
			}

			resp := assertSingleResponseLine(t, out)
			if resp.Error == nil || resp.Error.Code != -32602 {
				t.Fatalf("error = %+v, want invalid-params (-32602)", resp.Error)
			}
			if string(resp.ID) != "1" {
				t.Fatalf("id = %s, want 1", resp.ID)
			}
		})
	}
}

// TestServeMapsUnsupportedProtocolVersionToCorrelatedFactsWithNoCapabilities
// proves an unsupported protocol version request receives the existing
// typed negotiation rejection unwrapped: an invalid-params response
// correlated to the request id, carrying only the requested/supported
// public version facts, and no result/capabilities alongside it.
func TestServeMapsUnsupportedProtocolVersionToCorrelatedFactsWithNoCapabilities(t *testing.T) {
	line := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":99}}` + "\n"

	out := &bytes.Buffer{}
	server := New(nil, nil, nil, nil, nil, nil, nil)
	if err := server.Serve(context.Background(), strings.NewReader(line), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	resp := assertSingleResponseLine(t, out)
	if string(resp.ID) != "1" {
		t.Fatalf("id = %s, want 1", resp.ID)
	}
	if resp.Result != nil {
		t.Fatalf("result = %s, want no result/capabilities alongside a rejection", resp.Result)
	}
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("error = %+v, want invalid-params (-32602)", resp.Error)
	}
	data, ok := resp.Error.Data.(map[string]any)
	if !ok {
		t.Fatalf("error data = %#v, want an object carrying version facts", resp.Error.Data)
	}
	if data["reason"] != "unsupported_protocol_version" {
		t.Fatalf("error data reason = %v, want unsupported_protocol_version", data["reason"])
	}
	if data["requestedVersion"] != float64(99) {
		t.Fatalf("error data requestedVersion = %v, want 99", data["requestedVersion"])
	}
	if data["supportedVersion"] != float64(1) {
		t.Fatalf("error data supportedVersion = %v, want 1", data["supportedVersion"])
	}
}

// TestServeEmitsNoResponseForAValidNotification proves a well-formed
// notification (no id, a method in envelope.NotificationMethods) never
// receives any response -- success or error -- per JSON-RPC 2.0.
func TestServeEmitsNoResponseForAValidNotification(t *testing.T) {
	line := `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"s"}}` + "\n"

	out := &bytes.Buffer{}
	server := New(nil, nil, nil, nil, nil, nil, nil)
	if err := server.Serve(context.Background(), strings.NewReader(line), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no response for a valid notification", out.Bytes())
	}
}

// TestServeEmitsNoResponseForAnUnsupportedNoIDMessage proves JSON-RPC 2.0
// notification status is determined solely by the absence of an id: a
// method that is neither a known notification nor implemented by this
// transport ("initialize" is the only dispatched method) still receives no
// response -- success or error -- when it carries no id, matching
// TestServeEmitsNoResponseForAValidNotification's assertion for a known
// notification method.
func TestServeEmitsNoResponseForAnUnsupportedNoIDMessage(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{name: "unimplemented ACP method with no id", line: `{"jsonrpc":"2.0","method":"session/prompt","params":{}}` + "\n"},
		{name: "entirely unrecognized method with no id", line: `{"jsonrpc":"2.0","method":"totally/unrecognized"}` + "\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			server := New(nil, nil, nil, nil, nil, nil, nil)
			if err := server.Serve(context.Background(), strings.NewReader(tc.line), out); err != nil {
				t.Fatalf("Serve() error = %v", err)
			}
			if out.Len() != 0 {
				t.Fatalf("output = %q, want no response for an unsupported no-id message", out.Bytes())
			}
		})
	}
}

// TestServeContinuesProcessingAfterARecoverableRequestError proves a
// request-level rejection does not end the connection: the next valid
// framed request on the same connection still gets processed and answered.
func TestServeContinuesProcessingAfterARecoverableRequestError(t *testing.T) {
	input := "{not json\n" + initializeLine("1")

	out := &bytes.Buffer{}
	server := New(nil, nil, nil, nil, nil, nil, nil)
	if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	lines := nonEmptyResponseLines(t, out)
	if len(lines) != 2 {
		t.Fatalf("got %d response lines, want 2 (one rejection, one recovered success): %q", len(lines), out.Bytes())
	}

	var first rpcMessage
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatalf("unmarshal first response: %v", err)
	}
	if first.Error == nil || first.Error.Code != -32700 {
		t.Fatalf("first response error = %+v, want parse error (-32700)", first.Error)
	}

	var second rpcMessage
	if err := json.Unmarshal(lines[1], &second); err != nil {
		t.Fatalf("unmarshal second response: %v", err)
	}
	if second.Error != nil {
		t.Fatalf("second response error = %+v, want a successful result after recovering from the first rejection", second.Error)
	}
	if string(second.ID) != "1" {
		t.Fatalf("second response id = %s, want 1", second.ID)
	}
	assertJSONEqualStrings(t, initializeSuccessResult, string(second.Result))
}

// TestServeErrorResponsesAndDiagnosticsNeverLeakSeededSensitiveSentinels
// seeds a credential, an absolute filesystem path, a raw provider command,
// and an internal topology sentinel into every request-level error class
// this transport can produce -- malformed JSON, an unsupported method name,
// and malformed initialize params -- then proves none of those sentinels
// ever survive into the written response bytes or into the structured
// start/terminal diagnostics, mirroring the redaction proofs already
// required of protocol.MethodNotFound/SafeReject at the classification
// layer, but now proven at the actual wire and logging boundary.
func TestServeErrorResponsesAndDiagnosticsNeverLeakSeededSensitiveSentinels(t *testing.T) {
	sentinels := []string{
		"sk-live-credential-ABC123XYZ",
		"/home/operator/.ssh/id_rsa",
		"/usr/local/bin/agent --token=sk-live-credential-ABC123XYZ",
		"internal-dispatch-node-7.factory.internal",
	}

	for _, sentinel := range sentinels {
		lines := map[string]string{
			"malformed JSON":              fmt.Sprintf(`{not json %s`, sentinel) + "\n",
			"unsupported method name":     fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":{}}`, sentinel) + "\n",
			"malformed initialize params": fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":%q}`, sentinel) + "\n",
		}
		for name, line := range lines {
			t.Run(sentinel+"/"+name, func(t *testing.T) {
				out := &bytes.Buffer{}
				logger := &recordingLogger{}
				server := New(logger, nil, nil, nil, nil, nil, nil)
				if err := server.Serve(context.Background(), strings.NewReader(line), out); err != nil {
					t.Fatalf("Serve() error = %v", err)
				}
				if strings.Contains(out.String(), sentinel) {
					t.Fatalf("response leaked sentinel %q: %s", sentinel, out.Bytes())
				}
				for _, entry := range logger.entries {
					for key, value := range entry.fields {
						if text, ok := value.(string); ok && strings.Contains(text, sentinel) {
							t.Fatalf("log field %q leaked sentinel %q: %v", key, sentinel, value)
						}
					}
				}
			})
		}
	}
}

// assertJSONEqualStrings compares two JSON documents structurally rather
// than byte-for-byte, so object key ordering never causes a spurious
// mismatch.
func assertJSONEqualStrings(t *testing.T, want, got string) {
	t.Helper()
	var wantAny, gotAny any
	if err := json.Unmarshal([]byte(want), &wantAny); err != nil {
		t.Fatalf("decode want: %v", err)
	}
	if err := json.Unmarshal([]byte(got), &gotAny); err != nil {
		t.Fatalf("decode got: %v", err)
	}
	if !reflect.DeepEqual(wantAny, gotAny) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// readerFunc adapts a function to io.Reader.
type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// TestServeReturnsContextErrorOnMidReadCancellation proves cancellation
// unblocks Serve while the connection's read is genuinely parked waiting
// for input that will never arrive, for a stream that itself supports
// cancellation/closure: an io.Pipe's Read blocks until data is written or
// the pipe is closed, and this test closes the read side once ctx is done
// -- the same shape a real caller uses to make its context cancellable
// (pairing WithCancel with a stream close), which is exactly the "stream
// supports cancellation/closure" case this connection is required to
// honor. Serve then reports ctx's own error rather than the raw
// closed-pipe error it observes. Synchronization is entirely channel-based:
// the wrapped reader closes readStarted the first time it is called, which
// -- by Go's happens-before rule for goroutine creation -- can only happen
// after Serve has actually begun reading, so cancel() is never raced
// against Serve's own pre-check of an already-cancelled context. The single
// terminal time.After is a hang-safety net, not a poll loop.
func TestServeReturnsContextErrorOnMidReadCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	go func() {
		<-ctx.Done()
		_ = pr.Close()
	}()

	readStarted := make(chan struct{})
	var signalOnce sync.Once
	in := readerFunc(func(p []byte) (int, error) {
		signalOnce.Do(func() { close(readStarted) })
		return pr.Read(p)
	})

	server := New(nil, nil, nil, nil, nil, nil, nil)
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(ctx, in, &bytes.Buffer{})
	}()

	select {
	case <-readStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve never started reading")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}
}

// TestServeLogsCancelledOutcomeDistinctFromError proves a mid-connection
// context cancellation is reported through its own "cancelled" outcome
// label rather than the generic "error" label a real failure gets, so an
// operator watching diagnostics can distinguish a deliberate shutdown from
// a fault. As in TestServeReturnsContextErrorOnMidReadCancellation, it
// closes the read stream once ctx is done to model a caller-owned stream
// that supports cancellation/closure, and cancels only after Serve's read
// has actually started, proving the connection was genuinely mid-flight
// rather than pre-empted by the already-cancelled-context check
// (TestServeRejectsAlreadyCancelledContext covers that separate path,
// which never logs at all).
func TestServeLogsCancelledOutcomeDistinctFromError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	go func() {
		<-ctx.Done()
		_ = pr.Close()
	}()

	readStarted := make(chan struct{})
	var signalOnce sync.Once
	in := readerFunc(func(p []byte) (int, error) {
		signalOnce.Do(func() { close(readStarted) })
		return pr.Read(p)
	})

	logger := &recordingLogger{}
	server := New(logger, nil, nil, nil, nil, nil, nil)
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(ctx, in, &bytes.Buffer{})
	}()

	select {
	case <-readStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve never started reading")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}

	if len(logger.entries) != 2 {
		t.Fatalf("got %d log entries, want 2 (start and terminal)", len(logger.entries))
	}
	terminal := logger.entries[1]
	if terminal.message != "acp stdio connection terminated" {
		t.Fatalf("message = %q, want the connection-terminated message", terminal.message)
	}
	if outcome := terminal.fields["outcome"]; outcome != "cancelled" {
		t.Fatalf("outcome = %v, want %q", outcome, "cancelled")
	}
}

// TestServeRejectsPartialTrailingFrameAsProtocolFailure proves a
// non-newline-terminated remainder at EOF is treated as a deterministic
// protocol failure -- ending the connection with an error and writing no
// response -- rather than being executed as a request the way
// bufio.ScanLines' default final-token behavior would allow.
func TestServeRejectsPartialTrailingFrameAsProtocolFailure(t *testing.T) {
	partial := strings.TrimSuffix(initializeLine("1"), "\n")

	out := &bytes.Buffer{}
	server := New(nil, nil, nil, nil, nil, nil, nil)
	err := server.Serve(context.Background(), strings.NewReader(partial), out)
	if err == nil {
		t.Fatal("Serve() error = nil, want a protocol failure for a partial trailing frame")
	}
	if !errors.Is(err, errPartialTrailingFrame) {
		t.Fatalf("Serve() error = %v, want errPartialTrailingFrame", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no response written for an unexecuted partial frame", out.Bytes())
	}
}

// TestServeRejectsPartialTrailingFrameAfterACompleteLine proves a partial
// trailing frame still ends the connection with a protocol failure even
// after a preceding complete line was already answered -- the earlier
// response is not undone, but the trailing partial content is never
// dispatched.
func TestServeRejectsPartialTrailingFrameAfterACompleteLine(t *testing.T) {
	input := initializeLine("1") + `{"jsonrpc":"2.0","id":2,"method":"initialize"`

	out := &bytes.Buffer{}
	server := New(nil, nil, nil, nil, nil, nil, nil)
	err := server.Serve(context.Background(), strings.NewReader(input), out)
	if !errors.Is(err, errPartialTrailingFrame) {
		t.Fatalf("Serve() error = %v, want errPartialTrailingFrame", err)
	}

	lines := nonEmptyResponseLines(t, out)
	if len(lines) != 1 {
		t.Fatalf("got %d response lines, want 1 (only the earlier complete line): %q", len(lines), out.Bytes())
	}
	var resp rpcMessage
	if err := json.Unmarshal(lines[0], &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if string(resp.ID) != "1" {
		t.Fatalf("id = %s, want 1", resp.ID)
	}
}

// countingErrorWriter fails every Write with a fixed error and counts how
// many times Write was called, so a test can prove the connection stops
// writing after the first failure instead of retrying or continuing.
type countingErrorWriter struct {
	err   error
	calls int
}

func (w *countingErrorWriter) Write(p []byte) (int, error) {
	w.calls++
	return 0, w.err
}

// TestServeSurfacesWriterFailureAndStopsFurtherWrites proves a write
// failure is returned to the caller and ends the connection immediately --
// a second framed request in the same input never gets an attempted
// response write.
func TestServeSurfacesWriterFailureAndStopsFurtherWrites(t *testing.T) {
	wantErr := errors.New("acp test: simulated write failure")
	writer := &countingErrorWriter{err: wantErr}

	server := New(nil, nil, nil, nil, nil, nil, nil)
	input := initializeLine("1") + initializeLine("2")
	err := server.Serve(context.Background(), strings.NewReader(input), writer)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Serve() error = %v, want %v", err, wantErr)
	}
	if writer.calls != 1 {
		t.Fatalf("writer was called %d times, want exactly 1 (no writes after the first failure)", writer.calls)
	}
}

// TestServeAsyncPromptResponseWriteFailureEndsConnectionWithoutInputEOF
// proves a "session/prompt" response write failure -- discovered only
// asynchronously, on the dispatched prompt's own goroutine, after
// serveConnection's read loop has already gone back to waiting for the next
// line -- ends Serve with that write error immediately, instead of leaving
// Serve blocked forever waiting for more input that never arrives. The
// input pipe here is deliberately never closed and never written to again
// after the one "session/prompt" line: before serveConnection additionally
// selected on promptGroup's own Failed() signal concurrently with its
// blocking read, this exact "output already broken, input still open with
// nothing more coming" case had no way to unblock Serve. This server has no
// chat_sessions/factory_sessions collaborators configured, so the dispatched
// prompt fails fast with a bounded internal-error response instead of
// blocking on a real Factory call -- the only thing this test needs is that
// some response gets written and that write itself fails.
func TestServeAsyncPromptResponseWriteFailureEndsConnectionWithoutInputEOF(t *testing.T) {
	wantErr := errors.New("acp test: simulated async write failure")
	writer := &countingErrorWriter{err: wantErr}

	server := New(nil, nil, nil, nil, nil, nil, nil)

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})

	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background(), pr, writer) }()

	promptLine := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":%s}`+"\n",
		promptTextParams("session-1", "hello"))
	if _, err := pw.Write([]byte(promptLine)); err != nil {
		t.Fatalf("write session/prompt line: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, wantErr) {
			t.Fatalf("Serve() error = %v, want %v", err, wantErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the dispatched prompt's response write failed, despite input remaining open with no EOF")
	}
	if writer.calls != 1 {
		t.Fatalf("writer was called %d times, want exactly 1", writer.calls)
	}
}

// shortWriter always writes one byte fewer than it was given, without
// itself returning an error, mirroring a real short write.
type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

// TestServeTreatsShortWriteAsFailureNotSuccess proves a short write is
// reported as io.ErrShortWrite and ends the connection, so a truncated
// response can never be mistaken for a successful initialize exchange.
func TestServeTreatsShortWriteAsFailureNotSuccess(t *testing.T) {
	server := New(nil, nil, nil, nil, nil, nil, nil)
	err := server.Serve(context.Background(), strings.NewReader(initializeLine("1")), shortWriter{})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Serve() error = %v, want io.ErrShortWrite", err)
	}
}

// syncRecordingLogger is recordingLogger's concurrency-safe twin: multiple
// connections served concurrently on the same *Server share one logger, so
// TestServeHandlesConcurrentConnectionsWithoutRaces needs a logger safe for
// concurrent Info calls (recordingLogger itself is intentionally not
// synchronized, since every other test in this file drives it from a
// single goroutine).
type syncRecordingLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

func (l *syncRecordingLogger) Debug(msg string, keysAndValues ...any) {
	l.record("debug", msg, keysAndValues...)
}
func (l *syncRecordingLogger) Info(msg string, keysAndValues ...any) {
	l.record("info", msg, keysAndValues...)
}
func (l *syncRecordingLogger) Warn(msg string, keysAndValues ...any) {
	l.record("warn", msg, keysAndValues...)
}
func (l *syncRecordingLogger) Error(msg string, keysAndValues ...any) {
	l.record("error", msg, keysAndValues...)
}
func (l *syncRecordingLogger) Verbose(msg string, keysAndValues ...any) {
	l.record("verbose", msg, keysAndValues...)
}

func (l *syncRecordingLogger) record(level, msg string, keysAndValues ...any) {
	fields := map[string]any{}
	for index := 0; index+1 < len(keysAndValues); index += 2 {
		key, ok := keysAndValues[index].(string)
		if !ok {
			continue
		}
		fields[key] = keysAndValues[index+1]
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, logEntry{level: level, message: msg, fields: fields})
}

func (l *syncRecordingLogger) startedConnectionIDs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var ids []string
	for _, entry := range l.entries {
		if entry.message != "acp stdio connection started" {
			continue
		}
		if id, ok := entry.fields["connectionId"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

var _ logging.Logger = (*syncRecordingLogger)(nil)

// TestServeHandlesConcurrentConnectionsWithoutRaces runs many Serve
// invocations concurrently on one shared *Server (exercising the shared
// connection-id counter and logger under real concurrency; run with
// `go test -race` to prove there is no data race) and asserts every
// connection still gets its own distinct connection id and its own
// correct, whole-frame initialize response -- concurrent connections never
// observe each other's identity or output.
func TestServeHandlesConcurrentConnectionsWithoutRaces(t *testing.T) {
	const concurrency = 20

	logger := &syncRecordingLogger{}
	server := New(logger, nil, nil, nil, nil, nil, nil)

	outs := make([]*bytes.Buffer, concurrency)
	errs := make([]error, concurrency)
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := range concurrency {
		outs[i] = &bytes.Buffer{}
		go func(i int) {
			defer wg.Done()
			errs[i] = server.Serve(context.Background(), strings.NewReader(initializeLine("1")), outs[i])
		}(i)
	}
	wg.Wait()

	for i := range concurrency {
		if errs[i] != nil {
			t.Fatalf("connection %d: Serve() error = %v", i, errs[i])
		}
		resp := assertSingleResponseLine(t, outs[i])
		if string(resp.ID) != "1" {
			t.Fatalf("connection %d: id = %s, want 1", i, resp.ID)
		}
		if resp.Error != nil {
			t.Fatalf("connection %d: error = %+v, want a successful result", i, resp.Error)
		}
		assertJSONEqualStrings(t, initializeSuccessResult, string(resp.Result))
	}

	ids := logger.startedConnectionIDs()
	if len(ids) != concurrency {
		t.Fatalf("got %d started-connection log entries, want %d", len(ids), concurrency)
	}
	seen := make(map[string]bool, concurrency)
	for _, id := range ids {
		if id == "" {
			t.Fatal("got an empty connection id")
		}
		if seen[id] {
			t.Fatalf("connection id %q was reused across concurrent connections", id)
		}
		seen[id] = true
	}
}
