package stdio

import (
	"bytes"
	"context"
	"errors"
	"io"
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
