package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
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
func (l captureLogger) Warn(msg string, kv ...any) {
	*l.calls = append(*l.calls, capturedLogCall{level: "warn", msg: msg, kv: kv})
}
func (l captureLogger) Error(msg string, kv ...any) {
	*l.calls = append(*l.calls, capturedLogCall{level: "error", msg: msg, kv: kv})
}
func (l captureLogger) Verbose(msg string, kv ...any) {
	*l.calls = append(*l.calls, capturedLogCall{level: "verbose", msg: msg, kv: kv})
}

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
// outcome log, and that neither ever carries the caller-supplied WorkingRoot or raw
// JSON-RPC request identity value.
func TestStore_CreateSession_LogsStartAndAcceptedOutcomeWithoutUnsafeFields(t *testing.T) {
	logger, calls := newCaptureLogger()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil, logger)

	req := validCreateRequest()
	result, err := store.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	assertNoUnsafeFields(t, *calls, req.WorkingRoot, req.RequestID.JSONRPCStringID)

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
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil, logger)

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

	assertNoUnsafeFields(t, *calls, created.Session.WorkingRoot, "")
}

// TestStore_SetTarget_FailureLogsClassificationOnly proves a failed mutating
// operation's outcome log carries only the operation name, session ID, and
// error classification -- never a partial/zero-value accepted field.
func TestStore_SetTarget_FailureLogsClassificationOnly(t *testing.T) {
	logger, calls := newCaptureLogger()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil, logger)

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
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil, logger)

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

	assertNoUnsafeFields(t, *calls, created.Session.WorkingRoot, rawControlRequestID)
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
		for _, field := range append([]any{call.msg}, call.kv...) {
			rendered := fmt.Sprint(field)
			for _, unsafe := range unsafeValues {
				if unsafe != "" && strings.Contains(rendered, unsafe) {
					t.Fatalf("diagnostic contains unsafe marker %q: %+v", unsafe, call)
				}
			}
		}
	}
}

func TestStore_SelectedLoggerPreservesQuietOperationResults(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	capture, calls := newCaptureLogger()
	captured := NewStore(sequentialIDs("session"), fixedClock(at), nil, nil, capture)
	quiet := NewStore(sequentialIDs("session"), fixedClock(at), nil, nil, logging.NoopLogger{})
	if len(*calls) != 0 {
		t.Fatal("construction logged")
	}
	got := exerciseStoreLoggerOperations(t, captured)
	want := exerciseStoreLoggerOperations(t, quiet)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selected/quiet results differ: %+v / %+v", got, want)
	}
	assertDiagnosticPairs(t, *calls)
	assertSelectedOperationFields(t, *calls)
	for _, call := range *calls {
		if call.level == "info" && hasKV(call.kv, "error_class", "busy") && len(call.kv) != 6 {
			t.Fatalf("rejected operation logged accepted fields: %+v", call)
		}
	}
}

// This component witness compares complete returned facts, not logger implementation details.
func exerciseStoreLoggerOperations(t *testing.T, store *Store) []any {
	t.Helper()
	ctx := context.Background()
	created, read, started, req := startStoreLoggerOperations(t, store)
	retried, err := store.StartTurn(ctx, req)
	if err != nil || !reflect.DeepEqual(retried, started) {
		t.Fatalf("turn retry: %+v, %v", retried, err)
	}
	busy := req
	busy.RequestID = startTurnRequestID("other")
	busy.ExpectedVersion = started.Session.Version
	if _, err := store.StartTurn(ctx, busy); !errors.Is(err, chatsessions.ErrBusy) {
		t.Fatalf("busy: %v", err)
	}
	control := chatsessions.RequestControlRequest{RequestID: controlRequestID("conn", "control"), SessionID: created.Session.ID, ExpectedVersion: started.Session.Version, Action: chatsessions.ControlActionCancel}
	intent, err := store.RequestControl(ctx, control)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := store.RequestControl(ctx, control)
	if err != nil || retry != intent {
		t.Fatalf("control retry: %+v, %v", retry, err)
	}
	if intent.Intent.RequestedAt != created.Session.CreatedAt {
		t.Fatal("control ignored injected time")
	}
	advanced, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{SessionID: created.Session.ID, TurnID: started.Turn.ID, Next: chatsessions.TurnStateCanceled})
	if err != nil {
		t.Fatal(err)
	}
	final, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: created.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if final.Session.ActiveTurnID != "" || final.Session.Version != started.Session.Version+1 {
		t.Fatalf("terminal facts: %+v", final)
	}
	return []any{created, read, started, retried, intent, retry, advanced, final}
}

func assertDiagnosticPairs(t *testing.T, calls []capturedLogCall) {
	t.Helper()
	if len(calls) == 0 || len(calls)%2 != 0 {
		t.Fatalf("missing diagnostic pairs: %+v", calls)
	}
	for i := 0; i < len(calls); i += 2 {
		start, outcome := calls[i], calls[i+1]
		if start.level != "debug" || start.msg != "chat_sessions operation start" || outcome.level != "info" || outcome.msg != "chat_sessions operation outcome" {
			t.Fatalf("unexpected diagnostic pair: %+v / %+v", start, outcome)
		}
		if len(start.kv) != 4 || start.kv[0] != "op" || start.kv[2] != "session_id" || !hasKV(outcome.kv, "op", start.kv[1]) || !hasKey(outcome.kv, "error_class") {
			t.Fatalf("unexpected diagnostic fields: %+v / %+v", start, outcome)
		}
	}
}

func TestStore_SelectedDiagnosticsExcludeEmbeddedMarkers(t *testing.T) {
	t.Parallel()
	logger, calls := newCaptureLogger()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)), nil, nil, logger)
	markers := []string{"PROMPT_SECRET", "ROOT_SECRET", "CREDENTIAL_SECRET", "COMMAND_SECRET", "REQUEST_SECRET", "ERROR_SECRET"}
	req := validCreateRequest()
	req.WorkingRoot = "/workspace/" + strings.Join(markers[:4], "/")
	req.RequestID.JSONRPCStringID = "prefix-" + markers[4] + "-suffix"
	created, err := store.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.StartTurn(context.Background(), chatsessions.StartTurnRequest{RequestID: req.RequestID, SessionID: created.Session.ID, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.RequestControl(context.Background(), chatsessions.RequestControlRequest{RequestID: req.RequestID, SessionID: created.Session.ID, ExpectedVersion: started.Session.Version, Action: chatsessions.ControlActionCancel})
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []error{chatsessions.ErrStaleVersion, chatsessions.ErrBusy, chatsessions.ErrNotFound, chatsessions.ErrInvalidTransition, chatsessions.ErrRequiredValue, chatsessions.ErrUncommittedStreamPosition} {
		before := len(*calls)
		store.logStart("markerFailure", created.Session.ID)
		store.logOutcome("markerFailure", created.Session.ID, fmt.Errorf("prefix-%s-suffix: %w", markers[5], sentinel), "version", 99)
		if len((*calls)[before+1].kv) != 6 || !hasKV((*calls)[before+1].kv, "error_class", classifyError(sentinel)) {
			t.Fatalf("unsafe failure fields: %+v", (*calls)[before+1])
		}
	}
	assertDiagnosticPairs(t, *calls)
	assertNoUnsafeFields(t, *calls, markers...)
	var absent *Store
	absent.logStart("nil", "")
	absent.logOutcome("nil", "", errors.New(markers[5]))
}

func TestStore_SelectedDiagnosticsAndStateStayIsolated(t *testing.T) {
	t.Parallel()
	a, ca := newCaptureLogger()
	b, cb := newCaptureLogger()
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	first := NewStore(sequentialIDs("first"), fixedClock(at), nil, nil, a)
	second := NewStore(sequentialIDs("second"), fixedClock(at.Add(time.Hour)), nil, nil, b)
	one, err := first.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	two, err := second.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	if one.Session.CreatedAt != at || two.Session.CreatedAt != at.Add(time.Hour) {
		t.Fatal("instance clocks crossed")
	}
	_, err = first.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: one.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	firstCount := len(*ca)
	_, err = second.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: one.Session.ID})
	if !errors.Is(err, chatsessions.ErrNotFound) || len(*ca) != firstCount {
		t.Fatalf("peer lookup/state or logger crossed: %v", err)
	}
	for _, call := range *ca {
		if hasKV(call.kv, "session_id", two.Session.ID) {
			t.Fatalf("peer diagnostic in first: %+v", call)
		}
	}
	if !hasKV((*cb)[len(*cb)-1].kv, "error_class", "not_found") {
		t.Fatal("peer rejection missing from second logger")
	}
	read, err := second.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: two.Session.ID})
	if err != nil || read.Session != two.Session {
		t.Fatalf("second state changed: %+v, %v", read, err)
	}
	assertDiagnosticPairs(t, *ca)
	assertDiagnosticPairs(t, *cb)
}

func assertSelectedOperationFields(t *testing.T, calls []capturedLogCall) {
	t.Helper()
	expected := []struct {
		op    string
		extra []any
		class string
	}{
		{"CreateSession", []any{"version", uint64(1), "target_episode", uint64(1)}, ""},
		{"GetSession", []any{"version", uint64(1), "target_episode", uint64(1)}, ""},
		{"StartTurn", []any{"version", uint64(2), "turn_id", "session-2"}, ""},
		{"StartTurn", []any{"version", uint64(2), "turn_id", "session-2"}, ""},
		{"StartTurn", nil, "busy"},
		{"RequestControl", []any{"request_kind", string(chatsessions.RequestIdentityKindJSONRPCString), "action", string(chatsessions.ControlActionCancel), "turn_id", "session-2", "target_episode", uint64(1)}, ""},
		{"RequestControl", []any{"request_kind", string(chatsessions.RequestIdentityKindJSONRPCString), "action", string(chatsessions.ControlActionCancel), "turn_id", "session-2", "target_episode", uint64(1)}, ""},
		{"AdvanceTurn", []any{"turn_id", "session-2", "state", string(chatsessions.TurnStateCanceled)}, ""},
		{"GetSession", []any{"version", uint64(3), "target_episode", uint64(1)}, ""},
	}
	if len(calls) != len(expected)*2 {
		t.Fatalf("diagnostic count: %d", len(calls))
	}
	for i, want := range expected {
		startID := "session-1"
		if i == 0 {
			startID = ""
		}
		start := []any{"op", want.op, "session_id", startID}
		outcome := append([]any{"op", want.op, "session_id", "session-1", "error_class", want.class}, want.extra...)
		if !reflect.DeepEqual(calls[2*i].kv, start) || !reflect.DeepEqual(calls[2*i+1].kv, outcome) {
			t.Fatalf("%s diagnostic fields: %+v / %+v, want %+v / %+v", want.op, calls[2*i].kv, calls[2*i+1].kv, start, outcome)
		}
	}
}

func TestStore_SelectedLoggerRejectedOperationsDoNotMutate(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"validation", "conflict", "busy", "not_found", "invalid_transition", "invariant_violation"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			logger, calls := newCaptureLogger()
			store := NewStore(sequentialIDs("session"), fixedClock(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)), nil, nil, logger)
			ctx := context.Background()
			created, err := store.CreateSession(ctx, validCreateRequest())
			if err != nil {
				t.Fatal(err)
			}
			started, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{RequestID: startTurnRequestID("first"), SessionID: created.Session.ID, ExpectedVersion: 1})
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: created.Session.ID})
			if err != nil {
				t.Fatal(err)
			}
			*calls = nil
			err = rejectStoreLoggerOperation(t, store, created, started, kind)
			if err == nil {
				t.Fatal("rejected operation returned nil error")
			}
			if len(*calls) != 2 || len((*calls)[1].kv) != 6 || !hasKV((*calls)[1].kv, "error_class", kind) {
				t.Fatalf("failure diagnostics: %+v", *calls)
			}
			assertDiagnosticPairs(t, *calls)
			after, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: created.Session.ID})
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("rejection mutated session: %+v / %+v, %v", before, after, err)
			}
		})
	}
}

func startStoreLoggerOperations(t *testing.T, store *Store) (chatsessions.CreateSessionResult, chatsessions.GetSessionResult, chatsessions.StartTurnResult, chatsessions.StartTurnRequest) {
	t.Helper()
	ctx := context.Background()
	created, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	if created.Session.ID != "session-1" || created.Session.Version != 1 {
		t.Fatalf("unexpected session: %+v", created)
	}
	read, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: created.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	req := chatsessions.StartTurnRequest{RequestID: startTurnRequestID("turn"), SessionID: created.Session.ID, ExpectedVersion: 1}
	started, err := store.StartTurn(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return created, read, started, req
}

func rejectStoreLoggerOperation(t *testing.T, store *Store, created chatsessions.CreateSessionResult, started chatsessions.StartTurnResult, kind string) error {
	t.Helper()
	ctx := context.Background()
	var err error
	switch kind {
	case "validation":
		_, err = store.CreateSession(ctx, chatsessions.CreateSessionRequest{})
		var typed *chatsessions.ValidationError
		if !errors.As(err, &typed) {
			t.Fatalf("validation type: %v", err)
		}
	case "conflict":
		_, err = store.StartTurn(ctx, chatsessions.StartTurnRequest{RequestID: startTurnRequestID("second"), SessionID: created.Session.ID, ExpectedVersion: 1})
		var typed *chatsessions.ConflictError
		if !errors.As(err, &typed) {
			t.Fatalf("conflict type: %v", err)
		}
	case "busy":
		_, err = store.StartTurn(ctx, chatsessions.StartTurnRequest{RequestID: startTurnRequestID("second"), SessionID: created.Session.ID, ExpectedVersion: started.Session.Version})
		var typed *chatsessions.BusyError
		if !errors.As(err, &typed) {
			t.Fatalf("busy type: %v", err)
		}
	case "not_found":
		_, err = store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: "missing"})
		var typed *chatsessions.NotFoundError
		if !errors.As(err, &typed) {
			t.Fatalf("not-found type: %v", err)
		}
	case "invalid_transition":
		_, err = store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{SessionID: created.Session.ID, TurnID: started.Turn.ID, Next: chatsessions.TurnStateCompleted})
		if !errors.Is(err, chatsessions.ErrInvalidTransition) {
			t.Fatalf("transition: %v", err)
		}
	case "invariant_violation":
		_, err = store.AdvanceStreamHead(ctx, advanceStreamHeadRequest(created.Session.ID, 1, started.Session.Version, 1))
		var typed *chatsessions.UncommittedStreamPositionError
		if !errors.As(err, &typed) {
			t.Fatalf("invariant type: %v", err)
		}
	}
	return err
}
