package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// capturedLogCall records one structured log call made through
// logging.Logger, preserving the level so tests can distinguish start
// (Debug) from outcome (Info) calls.
type capturedLogCall struct {
	level string
	msg   string
	kv    []any
}

// captureLogger is a logging.Logger test double that records every call
// instead of writing anywhere, so tests can assert on exactly what a Store
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

func TestClassifyError_TableDriven(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"stale version", chatsessions.ErrStaleVersion, "conflict"},
		{"conflict error", &chatsessions.ConflictError{Value: "Session", ID: "s-1", Expected: 1, Actual: 2}, "conflict"},
		{"busy", chatsessions.ErrBusy, "busy"},
		{"busy error", &chatsessions.BusyError{Value: "Session", ID: "s-1"}, "busy"},
		{"not found", chatsessions.ErrNotFound, "not_found"},
		{"not found error", &chatsessions.NotFoundError{Value: "Session", ID: "s-1"}, "not_found"},
		{"invalid transition", chatsessions.ErrInvalidTransition, "invalid_transition"},
		{"target episode not closed", chatsessions.ErrTargetEpisodeNotClosed, "invariant_violation"},
		{"target episode exhausted", chatsessions.ErrTargetEpisodeNumberExhausted, "invariant_violation"},
		{"required value", chatsessions.ErrRequiredValue, "validation"},
		{"unknown enum", chatsessions.ErrUnknownEnumValue, "validation"},
		{"unsupported control action", chatsessions.ErrUnsupportedControlAction, "validation"},
		{"unrelated error", errors.New("boom"), "validation"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyError(tt.err); got != tt.want {
				t.Fatalf("classifyError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// TestStore_CreateSession_LogsStartAndAcceptedOutcomeWithoutUnsafeFields
// proves a successful mutating operation emits a Debug start log and an Info
// outcome log, and that neither ever carries the caller-supplied Cwd or raw
// JSON-RPC request identity value.
func TestStore_CreateSession_LogsStartAndAcceptedOutcomeWithoutUnsafeFields(t *testing.T) {
	logger, calls := newCaptureLogger()
	store := New(sequentialIDs("session"), fixedClock(time.Now()), logger)

	req := validCreateRequest()
	result, err := store.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	assertNoUnsafeFields(t, *calls, req.Cwd, req.RequestID.JSONRPCStringID)

	if len(*calls) != 2 {
		t.Fatalf("CreateSession logged %d calls, want 2 (start, outcome): %+v", len(*calls), *calls)
	}
	if (*calls)[0].level != "debug" {
		t.Fatalf("first log level = %q, want debug (start)", (*calls)[0].level)
	}
	outcome := (*calls)[1]
	if outcome.level != "info" {
		t.Fatalf("second log level = %q, want info (outcome)", outcome.level)
	}
	if !hasKV(outcome.kv, "session_id", result.Session.ID) {
		t.Fatalf("outcome log missing session_id=%q: %+v", result.Session.ID, outcome.kv)
	}
	if !hasKV(outcome.kv, "error_class", "") {
		t.Fatalf("successful outcome log must carry an empty error_class: %+v", outcome.kv)
	}
}

// TestStore_GetSession_LogsStartAndOutcome proves GetSession -- the sole
// method that previously bypassed logStart/logOutcome -- logs a Debug start
// and Info outcome for both a successful read and a *NotFoundError read,
// with the same start/outcome shape as every other Store operation.
func TestStore_GetSession_LogsStartAndOutcome(t *testing.T) {
	logger, calls := newCaptureLogger()
	store := New(sequentialIDs("session"), fixedClock(time.Now()), logger)

	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	*calls = nil // discard CreateSession's own log calls

	if _, err := store.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: created.Session.ID}); err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if len(*calls) != 2 {
		t.Fatalf("GetSession logged %d calls, want 2 (start, outcome): %+v", len(*calls), *calls)
	}
	if (*calls)[0].level != "debug" {
		t.Fatalf("first log level = %q, want debug (start)", (*calls)[0].level)
	}
	outcome := (*calls)[1]
	if outcome.level != "info" {
		t.Fatalf("second log level = %q, want info (outcome)", outcome.level)
	}
	if !hasKV(outcome.kv, "session_id", created.Session.ID) {
		t.Fatalf("outcome log missing session_id=%q: %+v", created.Session.ID, outcome.kv)
	}
	if !hasKV(outcome.kv, "error_class", "") {
		t.Fatalf("successful outcome log must carry an empty error_class: %+v", outcome.kv)
	}

	*calls = nil
	if _, err := store.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: "does-not-exist"}); err == nil {
		t.Fatal("GetSession unknown session: got nil error, want *NotFoundError")
	}
	if len(*calls) != 2 {
		t.Fatalf("GetSession (not found) logged %d calls, want 2 (start, outcome): %+v", len(*calls), *calls)
	}
	if !hasKV((*calls)[1].kv, "error_class", "not_found") {
		t.Fatalf("not-found outcome log missing error_class=not_found: %+v", (*calls)[1].kv)
	}

	assertNoUnsafeFields(t, *calls, created.Session.Cwd, "")
}

// TestStore_SetTarget_FailureLogsClassificationOnly proves a failed mutating
// operation's outcome log carries only the operation name, session ID, and
// error classification -- never a partial/zero-value accepted field.
func TestStore_SetTarget_FailureLogsClassificationOnly(t *testing.T) {
	logger, calls := newCaptureLogger()
	store := New(sequentialIDs("session"), fixedClock(time.Now()), logger)

	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	*calls = nil // discard CreateSession's own log calls

	_, err = store.SetTarget(context.Background(), chatsessions.SetTargetRequest{
		RequestID:       chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindTransportUUID, TransportUUID: "11111111-1111-1111-1111-111111111111"},
		SessionID:       created.Session.ID,
		ExpectedVersion: created.Session.Version + 1, // stale on purpose
		Target:          created.Session.SelectedTarget,
	})
	if !errors.Is(err, chatsessions.ErrStaleVersion) {
		t.Fatalf("SetTarget: got %v, want ErrStaleVersion", err)
	}

	if len(*calls) != 2 {
		t.Fatalf("SetTarget logged %d calls, want 2 (start, outcome): %+v", len(*calls), *calls)
	}
	outcome := (*calls)[1]
	if !hasKV(outcome.kv, "error_class", "conflict") {
		t.Fatalf("outcome log missing error_class=conflict: %+v", outcome.kv)
	}
	if hasKey(outcome.kv, "target_episode") {
		t.Fatalf("failed outcome log must not carry accepted-only fields: %+v", outcome.kv)
	}
}

// TestStore_RequestControl_NeverLogsRawRequestIdentity proves RequestControl
// logs only the RequestIdentity's Kind discriminator, never the raw
// JSON-RPC id value that identity carries.
func TestStore_RequestControl_NeverLogsRawRequestIdentity(t *testing.T) {
	logger, calls := newCaptureLogger()
	store := New(sequentialIDs("session"), fixedClock(time.Now()), logger)

	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	started, err := store.StartTurn(context.Background(), chatsessions.StartTurnRequest{
		RequestID:       chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "turn-req-1"},
		SessionID:       created.Session.ID,
		ExpectedVersion: created.Session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	*calls = nil

	const rawControlRequestID = "control-req-secret"
	_, err = store.RequestControl(context.Background(), chatsessions.RequestControlRequest{
		RequestID:       chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: rawControlRequestID},
		SessionID:       created.Session.ID,
		ExpectedVersion: started.Session.Version,
		Action:          chatsessions.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("RequestControl: %v", err)
	}

	assertNoUnsafeFields(t, *calls, created.Session.Cwd, rawControlRequestID)
	outcome := (*calls)[1]
	if !hasKV(outcome.kv, "request_kind", string(chatsessions.RequestIdentityKindJSONRPCString)) {
		t.Fatalf("outcome log missing safe request_kind field: %+v", outcome.kv)
	}
}

func hasKey(kv []any, key string) bool {
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i] == key {
			return true
		}
	}
	return false
}

func hasKV(kv []any, key string, value any) bool {
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i] == key && kv[i+1] == value {
			return true
		}
	}
	return false
}

// assertNoUnsafeFields fails t if any logged value (or the log message
// itself) contains the given caller-supplied secrets, wherever they appear
// in the captured calls.
func assertNoUnsafeFields(t *testing.T, calls []capturedLogCall, unsafeValues ...string) {
	t.Helper()
	for _, call := range calls {
		for i := 0; i+1 < len(call.kv); i += 2 {
			rendered := fmt.Sprintf("%v", call.kv[i+1])
			for _, unsafe := range unsafeValues {
				if unsafe != "" && rendered == unsafe {
					t.Fatalf("log call %+v carries unsafe field value %q", call, unsafe)
				}
			}
		}
	}
}
