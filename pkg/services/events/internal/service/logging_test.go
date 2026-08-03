package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// capturedLogCall records one structured log call made through
// logging.Logger, preserving the level so tests can distinguish the
// intent (Debug) call from the outcome (Info) call.
type capturedLogCall struct {
	level string
	msg   string
	kv    []any
}

// captureLogger is a logging.Logger test double that records every call
// instead of writing anywhere, so tests can assert exactly what a Store
// operation logs.
type captureLogger struct {
	calls *[]capturedLogCall
}

func newCaptureLogger() (captureLogger, *[]capturedLogCall) {
	calls := &[]capturedLogCall{}
	return captureLogger{calls: calls}, calls
}

func (l captureLogger) Debug(msg string, kv ...any) {
	*l.calls = append(*l.calls, capturedLogCall{level: "debug", msg: msg, kv: kv})
}
func (l captureLogger) Info(msg string, kv ...any) {
	*l.calls = append(*l.calls, capturedLogCall{level: "info", msg: msg, kv: kv})
}
func (l captureLogger) Warn(msg string, kv ...any)    {}
func (l captureLogger) Error(msg string, kv ...any)   {}
func (l captureLogger) Verbose(msg string, kv ...any) {}

func hasKV(kv []any, key string, value any) bool {
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i] == key && kv[i+1] == value {
			return true
		}
	}
	return false
}

func TestClassifyAppendError_TableDriven(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"empty payload", events.ErrEmptyPayload, "invalid_payload"},
		{"malformed payload json", events.ErrMalformedPayloadJSON, "invalid_payload"},
		{"empty topic", events.ErrEmptyTopic, "validation"},
		{"unrelated error", errors.New("boom"), "validation"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyAppendError(tt.err); got != tt.want {
				t.Fatalf("classifyAppendError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestAppend_AcceptedLogsIntentAndOutcomeWithoutPayload(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := New(logger)

	req := validAppendRequest()
	result, err := st.Append(context.Background(), req)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if len(*calls) != 2 {
		t.Fatalf("Append() logged %d calls, want 2 (intent, outcome): %+v", len(*calls), *calls)
	}
	if (*calls)[0].level != "debug" {
		t.Fatalf("first log level = %q, want debug (intent)", (*calls)[0].level)
	}
	outcome := (*calls)[1]
	if outcome.level != "info" {
		t.Fatalf("second log level = %q, want info (outcome)", outcome.level)
	}
	if !hasKV(outcome.kv, "outcome", "accepted") {
		t.Fatalf("outcome log missing outcome=accepted: %+v", outcome.kv)
	}
	if !hasKV(outcome.kv, "position", uint64(result.Record.ID.Position)) {
		t.Fatalf("outcome log missing position=%d: %+v", result.Record.ID.Position, outcome.kv)
	}

	for _, call := range *calls {
		for i := 0; i+1 < len(call.kv); i += 2 {
			rendered := fmt.Sprintf("%v", call.kv[i+1])
			if rendered == string(req.Payload) {
				t.Fatalf("log call %+v carries raw payload content", call)
			}
		}
	}
}

func TestAppend_DuplicateLogsDuplicateOutcome(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := New(logger)
	req := validAppendRequest()

	if _, err := st.Append(context.Background(), req); err != nil {
		t.Fatalf("Append() first error = %v", err)
	}
	*calls = nil

	if _, err := st.Append(context.Background(), req); err != nil {
		t.Fatalf("Append() duplicate error = %v", err)
	}

	outcome := (*calls)[len(*calls)-1]
	if !hasKV(outcome.kv, "outcome", "duplicate") {
		t.Fatalf("outcome log missing outcome=duplicate: %+v", outcome.kv)
	}
}

func TestAppend_RejectedLogsClassificationOnlyWithoutIntentLog(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := New(logger)

	req := validAppendRequest()
	req.Payload = nil

	if _, err := st.Append(context.Background(), req); !errors.Is(err, events.ErrEmptyPayload) {
		t.Fatalf("Append() error = %v, want ErrEmptyPayload", err)
	}

	if len(*calls) != 1 {
		t.Fatalf("rejected Append() logged %d calls, want exactly 1 (outcome only, no intent log): %+v", len(*calls), *calls)
	}
	outcome := (*calls)[0]
	if outcome.level != "info" {
		t.Fatalf("rejected outcome log level = %q, want info", outcome.level)
	}
	if !hasKV(outcome.kv, "outcome", "rejected") {
		t.Fatalf("outcome log missing outcome=rejected: %+v", outcome.kv)
	}
	if !hasKV(outcome.kv, "error_class", "invalid_payload") {
		t.Fatalf("outcome log missing error_class=invalid_payload: %+v", outcome.kv)
	}
	if hasKV(outcome.kv, "position", uint64(1)) {
		t.Fatalf("rejected outcome log must not carry an accepted-only position field: %+v", outcome.kv)
	}
}

func TestAppend_CanceledContextLogsClassificationOnlyWithoutIntentLog(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := New(logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := st.Append(ctx, validAppendRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Append() error = %v, want context.Canceled", err)
	}

	if len(*calls) != 1 {
		t.Fatalf("canceled Append() logged %d calls, want exactly 1 (outcome only, no intent log): %+v", len(*calls), *calls)
	}
	if !hasKV((*calls)[0].kv, "outcome", "rejected") {
		t.Fatalf("outcome log missing outcome=rejected: %+v", (*calls)[0].kv)
	}
}

func TestNew_DefaultsToNoopLoggerWhenOmitted(t *testing.T) {
	st := New()
	if _, err := st.Append(context.Background(), validAppendRequest()); err != nil {
		t.Fatalf("Append() with default logger error = %v", err)
	}
}

func TestValidAppendRequestPayloadNotJSONEncodedAsAnyOtherField(t *testing.T) {
	// Guards the payload-leak assertion above: proves the fixture payload is
	// distinct from every other fixture field so a false negative can't hide
	// a real leak.
	req := validAppendRequest()
	other := []string{string(req.Topic), string(req.SourceType), string(req.SourceID), string(req.SchemaID)}
	for _, value := range other {
		if value == string(req.Payload) {
			t.Fatalf("fixture field %q collides with Payload; payload-leak assertions would be meaningless", value)
		}
	}
	_ = json.RawMessage(req.Payload)
}
