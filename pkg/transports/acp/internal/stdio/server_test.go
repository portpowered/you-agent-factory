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
	"testing"

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
	server := New(logger)
	if server == nil {
		t.Fatal("New() returned nil")
	}
	if len(logger.entries) != 0 {
		t.Fatalf("New() logged %d entries, want 0: construction must perform no I/O", len(logger.entries))
	}
}

func TestServeRejectsMissingStreams(t *testing.T) {
	server := New(nil)
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
	server := New(nil)
	err := server.Serve(context.Background(), strings.NewReader("first line\nsecond line\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Serve() error = %v, want nil on clean EOF", err)
	}
}

func TestServeRejectsAlreadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	server := New(nil)
	err := server.Serve(ctx, strings.NewReader(""), &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve() error = %v, want context.Canceled", err)
	}
}

func TestServeMintsDistinctConnectionIDsPerInvocation(t *testing.T) {
	logger := &recordingLogger{}
	server := New(logger)

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
	server := New(logger)

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
const initializeSuccessResult = `{"protocolVersion":1,"authMethods":[],"agentCapabilities":{"auth":{},"loadSession":true,"mcpCapabilities":{},"promptCapabilities":{},"sessionCapabilities":{"resume":{}}}}`

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
			server := New(nil)
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
	server := New(nil)
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
	server := New(nil)
	if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	assertSingleResponseLine(t, out)
}

func TestServeIsolatesConnectionsReusingTheSameWireID(t *testing.T) {
	logger := &recordingLogger{}
	server := New(logger)

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
// deferred ACP session/prompt method already listed in
// protocol.SupportedMethods (a forward-looking closed set for the whole
// future method surface, not what this transport slice actually
// implements -- see the Codebase Patterns entry on protocol.SupportedMethods)
// plus a method this transport never expects at all, proving all of them
// get method-not-found rather than being dispatched or hanging.
func TestServeRespondsMethodNotFoundForEveryUnimplementedMethod(t *testing.T) {
	methods := []string{
		"session/new",
		"session/load",
		"session/resume",
		"session/set_config_option",
		"session/prompt",
		"session/request_permission",
		"totally/unrecognized_method",
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			input := fmt.Sprintf(`{"jsonrpc":"2.0","id":9,"method":%q,"params":{}}`, method) + "\n"

			out := &bytes.Buffer{}
			server := New(nil)
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
	server := New(nil)
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			server := New(nil)
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
		{name: "malformed params type", line: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":"not-an-object"}` + "\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			server := New(nil)
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
	server := New(nil)
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
	server := New(nil)
	if err := server.Serve(context.Background(), strings.NewReader(line), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no response for a valid notification", out.Bytes())
	}
}

// TestServeContinuesProcessingAfterARecoverableRequestError proves a
// request-level rejection does not end the connection: the next valid
// framed request on the same connection still gets processed and answered.
func TestServeContinuesProcessingAfterARecoverableRequestError(t *testing.T) {
	input := "{not json\n" + initializeLine("1")

	out := &bytes.Buffer{}
	server := New(nil)
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
				server := New(logger)
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
