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

func TestServeRespondsMethodNotFoundForAnUnimplementedMethod(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":9,"method":"session/new","params":{}}` + "\n"

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
