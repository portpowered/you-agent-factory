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

func TestRead_LogsProgressOutcomeWithoutPayload(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := New(logger)
	ctx := context.Background()
	req := validAppendRequest()
	req.Topic = readTestTopic
	if _, err := st.Append(ctx, req); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	*calls = nil

	result, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if len(*calls) != 1 {
		t.Fatalf("Read() logged %d calls, want 1 (outcome only): %+v", len(*calls), *calls)
	}
	outcome := (*calls)[0]
	if outcome.level != "info" {
		t.Fatalf("Read() outcome log level = %q, want info", outcome.level)
	}
	if !hasKV(outcome.kv, "outcome", "progress") {
		t.Fatalf("outcome log missing outcome=progress: %+v", outcome.kv)
	}
	if !hasKV(outcome.kv, "next", uint64(result.Next.Position)) {
		t.Fatalf("outcome log missing next=%d: %+v", result.Next.Position, outcome.kv)
	}
	for _, call := range *calls {
		for i := 0; i+1 < len(call.kv); i += 2 {
			if fmt.Sprintf("%v", call.kv[i+1]) == string(req.Payload) {
				t.Fatalf("Read() log call %+v carries raw payload content", call)
			}
		}
	}
}

func TestRead_RejectedRequestLogsNothing(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := New(logger)

	_, err := st.Read(context.Background(), events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic}, Limit: 0})
	if !errors.Is(err, events.ErrInvalidReadLimit) {
		t.Fatalf("Read() error = %v, want ErrInvalidReadLimit", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("rejected Read() logged %d calls, want 0 (no log side effect before validation succeeds): %+v", len(*calls), *calls)
	}
}

func TestSubscribe_AcceptedLogsOutcomeWithoutPayload(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := New(logger)

	_, err := st.Subscribe(context.Background(), events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 10})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if len(*calls) != 1 {
		t.Fatalf("Subscribe() logged %d calls, want 1 (accepted outcome only): %+v", len(*calls), *calls)
	}
	if !hasKV((*calls)[0].kv, "outcome", "accepted") {
		t.Fatalf("outcome log missing outcome=accepted: %+v", (*calls)[0].kv)
	}
}

func TestSubscribe_UnresolvableCursorLogsRejectedOutcome(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := New(logger)
	ctx := context.Background()
	appendOne(t, st, ctx, subscribeTestTopic, 1)
	*calls = nil

	_, err := st.Subscribe(ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic, Position: 99}, Limit: 10})
	if !errors.Is(err, events.ErrUnresolvableCursor) {
		t.Fatalf("Subscribe() error = %v, want ErrUnresolvableCursor", err)
	}

	if len(*calls) != 1 {
		t.Fatalf("rejected Subscribe() logged %d calls, want 1: %+v", len(*calls), *calls)
	}
	if !hasKV((*calls)[0].kv, "outcome", "rejected") {
		t.Fatalf("outcome log missing outcome=rejected: %+v", (*calls)[0].kv)
	}
	if !hasKV((*calls)[0].kv, "error_class", "unresolvable_cursor") {
		t.Fatalf("outcome log missing error_class=unresolvable_cursor: %+v", (*calls)[0].kv)
	}
}

func TestSubscribe_RejectedRequestLogsNothing(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := New(logger)

	_, err := st.Subscribe(context.Background(), events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 0})
	if !errors.Is(err, events.ErrInvalidReadLimit) {
		t.Fatalf("Subscribe() error = %v, want ErrInvalidReadLimit", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("rejected Subscribe() logged %d calls, want 0: %+v", len(*calls), *calls)
	}
}

func TestSubscribe_GapLogsSafeFacts(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := NewWithRetention(2, logger)
	ctx := context.Background()
	appendN(t, st, ctx, subscribeTestTopic, 5)
	*calls = nil

	_, err := st.Subscribe(ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic, Position: 1}, Limit: 10})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	var found bool
	for _, call := range *calls {
		if call.msg == "events subscribe gap" {
			found = true
			if !hasKV(call.kv, "earliest_retained", uint64(4)) {
				t.Fatalf("gap log missing earliest_retained=4: %+v", call.kv)
			}
		}
	}
	if !found {
		t.Fatalf("no gap log emitted: %+v", *calls)
	}
}

func TestSubscribe_BackpressureLogsSafeTopicContextOnce(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := New(logger)
	ctx := context.Background()

	sub, err := st.Subscribe(ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 1})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	*calls = nil

	appendN(t, st, ctx, subscribeTestTopic, 5)

	var backpressureLogs int
	for _, call := range *calls {
		if call.msg == "events subscribe backpressure" {
			backpressureLogs++
			if !hasKV(call.kv, "topic", string(subscribeTestTopic)) {
				t.Fatalf("backpressure log missing topic: %+v", call.kv)
			}
		}
	}
	if backpressureLogs != 1 {
		t.Fatalf("backpressure logged %d times, want exactly 1", backpressureLogs)
	}

	// Draining and repeated observation must not log backpressure again.
	for range 5 {
		sub.Next(ctx)
	}
	backpressureLogs = 0
	for _, call := range *calls {
		if call.msg == "events subscribe backpressure" {
			backpressureLogs++
		}
	}
	if backpressureLogs != 1 {
		t.Fatalf("backpressure logged %d times after repeated Next(), want still exactly 1", backpressureLogs)
	}
}

func TestSubscribe_CloseLogsTopicClosedAndStoreClosed(t *testing.T) {
	logger, calls := newCaptureLogger()
	st := New(logger)
	ctx := context.Background()

	if _, err := st.Subscribe(ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 10}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	*calls = nil

	st.Close()

	var sawTopicClosed, sawStoreClosed bool
	for _, call := range *calls {
		switch call.msg {
		case "events subscribe topic closed":
			sawTopicClosed = true
			if !hasKV(call.kv, "subscriber_count", 1) {
				t.Fatalf("topic closed log missing subscriber_count=1: %+v", call.kv)
			}
		case "events store closed":
			sawStoreClosed = true
		}
	}
	if !sawTopicClosed {
		t.Fatalf("no topic closed log emitted: %+v", *calls)
	}
	if !sawStoreClosed {
		t.Fatalf("no store closed log emitted: %+v", *calls)
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
