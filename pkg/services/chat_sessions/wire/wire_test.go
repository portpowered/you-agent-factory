package wire

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// stubEventsAppender is a minimal EventsAppender double for tests that
// construct a Service but do not themselves exercise Sequence.
type stubEventsAppender struct{ calls *int }

func (s stubEventsAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	if s.calls != nil {
		*s.calls++
	}
	return events.AppendResult{}, nil
}

// stubEventsReader is a minimal EventsReader double for tests that construct
// a Service but do not themselves exercise AcknowledgeAttachment.
type stubEventsReader struct{ calls *int }

func (s stubEventsReader) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
	if s.calls != nil {
		*s.calls++
	}
	return events.ReadResult{}, nil
}

func sequentialIDs(prefix string) IDGenerator {
	n := 0
	return func() string {
		n++
		return prefix + "-" + strconv.Itoa(n)
	}
}

func fixedClock(at time.Time) Clock {
	return func() time.Time { return at }
}

func validCreateRequest() chatsessions.CreateSessionRequest {
	return chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-1"},
		WorkingRoot:   "/workspace/project",
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
	}
}

func TestNewService_RequiresIDGenerator(t *testing.T) {
	service, err := NewService(nil, fixedClock(time.Now()), stubEventsAppender{}, stubEventsReader{})
	if err == nil || service != nil {
		t.Fatalf("NewService(nil id generator) = (%v, %v), want construction failure", service, err)
	}
	if err.Error() != "construct chat sessions: id generator is required" {
		t.Fatalf("changed construction error: %v", err)
	}
}

func TestNewService_RequiresClock(t *testing.T) {
	service, err := NewService(sequentialIDs("session"), nil, stubEventsAppender{}, stubEventsReader{})
	if err == nil || service != nil {
		t.Fatalf("NewService(nil clock) = (%v, %v), want construction failure", service, err)
	}
	if err.Error() != "construct chat sessions: clock is required" {
		t.Fatalf("changed construction error: %v", err)
	}
}

func TestNewService_RequiresEventsAppender(t *testing.T) {
	service, err := NewService(sequentialIDs("session"), fixedClock(time.Now()), nil, stubEventsReader{})
	if err == nil || service != nil {
		t.Fatalf("NewService(nil events appender) = (%v, %v), want construction failure", service, err)
	}
	if err.Error() != "construct chat sessions: events appender is required" {
		t.Fatalf("changed construction error: %v", err)
	}
}

func TestNewService_RequiresEventsReader(t *testing.T) {
	service, err := NewService(sequentialIDs("session"), fixedClock(time.Now()), stubEventsAppender{}, nil)
	if err == nil || service != nil {
		t.Fatalf("NewService(nil events reader) = (%v, %v), want construction failure", service, err)
	}
	if err.Error() != "construct chat sessions: events reader is required" {
		t.Fatalf("changed construction error: %v", err)
	}
}

// TestNewService_ConstructsAWorkingService proves the canonically
// constructed Service round-trips a session create/get through the public
// chatsessions.Service interface, without depending on any Store-internal
// detail.
func TestNewService_ConstructsAWorkingService(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"omitted", "nil", "noop", "capture", "capture-first", "nil-first"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
			first, later := &serviceCaptureLogger{}, &serviceCaptureLogger{}
			var loggers []logging.Logger
			switch mode {
			case "nil":
				loggers = []logging.Logger{nil}
			case "noop":
				loggers = []logging.Logger{logging.NoopLogger{}}
			case "capture":
				loggers = []logging.Logger{first}
			case "capture-first":
				loggers = []logging.Logger{first, later}
			case "nil-first":
				loggers = []logging.Logger{nil, later}
			}
			idCalls, clockCalls, eventCalls := 0, 0, 0
			ids := sequentialIDs("session")
			service, err := NewService(func() string { idCalls++; return ids() }, func() time.Time { clockCalls++; return at }, stubEventsAppender{calls: &eventCalls}, stubEventsReader{calls: &eventCalls}, loggers...)
			if err != nil {
				t.Fatal(err)
			}
			if idCalls != 0 || clockCalls != 0 || eventCalls != 0 || len(first.calls) != 0 || len(later.calls) != 0 {
				t.Fatal("construction invoked effects")
			}
			got := exerciseServiceLoggerOperations(t, service)
			quiet, err := NewService(sequentialIDs("session"), fixedClock(at), stubEventsAppender{}, stubEventsReader{})
			if err != nil {
				t.Fatal(err)
			}
			want := exerciseServiceLoggerOperations(t, quiet)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s changed operation facts: %+v / %+v", mode, got, want)
			}
			if len(later.calls) != 0 {
				t.Fatalf("later logger selected: %+v", later.calls)
			}
			if mode == "capture" || mode == "capture-first" {
				if len(first.calls) != 18 {
					t.Fatalf("selected logger received %d calls, want 18", len(first.calls))
				}
				for i, call := range first.calls {
					wantLevel, wantMessage := "debug", "chat_sessions operation start"
					if i%2 != 0 {
						wantLevel, wantMessage = "info", "chat_sessions operation outcome"
					}
					if call.level != wantLevel || call.msg != wantMessage {
						t.Fatalf("unexpected diagnostic: %+v", call)
					}
				}
			} else if len(first.calls) != 0 {
				t.Fatal("quiet construction emitted diagnostics")
			}
		})
	}
}

// TestNewService_InstancesShareNoState proves two independently constructed
// Service instances are fully isolated, matching the process-scoped-owner
// guarantee the canonical provider must uphold.
func TestNewService_InstancesShareNoState(t *testing.T) {
	t.Parallel()
	a, b := &serviceCaptureLogger{}, &serviceCaptureLogger{}
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	first, err := NewService(sequentialIDs("first"), fixedClock(at), stubEventsAppender{}, stubEventsReader{}, a)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewService(sequentialIDs("second"), fixedClock(at.Add(time.Hour)), stubEventsAppender{}, stubEventsReader{}, b)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	one, err := first.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	two, err := second.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	read, err := first.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: one.Session.ID})
	if err != nil || read.Session != one.Session {
		t.Fatalf("first read: %+v, %v", read, err)
	}
	if _, err := second.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: one.Session.ID}); !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("peer lookup: %v", err)
	}
	if len(a.calls) != 4 || len(b.calls) != 4 {
		t.Fatalf("diagnostic attribution crossed: %+v / %+v", a.calls, b.calls)
	}
	read, err = second.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: two.Session.ID})
	if err != nil || read.Session != two.Session || one.Session.CreatedAt != at || two.Session.CreatedAt != at.Add(time.Hour) {
		t.Fatalf("instance facts crossed: %+v, %v", read, err)
	}
}

// These existing provider tests are retained wiring integration, not component unit tests.
type serviceLogCall struct {
	level, msg string
	kv         []any
}
type serviceCaptureLogger struct{ calls []serviceLogCall }

func (l *serviceCaptureLogger) Debug(msg string, kv ...any) {
	l.calls = append(l.calls, serviceLogCall{"debug", msg, kv})
}
func (l *serviceCaptureLogger) Info(msg string, kv ...any) {
	l.calls = append(l.calls, serviceLogCall{"info", msg, kv})
}
func (l *serviceCaptureLogger) Warn(msg string, kv ...any) {
	l.calls = append(l.calls, serviceLogCall{"warn", msg, kv})
}
func (l *serviceCaptureLogger) Error(msg string, kv ...any) {
	l.calls = append(l.calls, serviceLogCall{"error", msg, kv})
}
func (l *serviceCaptureLogger) Verbose(msg string, kv ...any) {
	l.calls = append(l.calls, serviceLogCall{"verbose", msg, kv})
}

// stubOperatorSettingsService is a minimal Operator Settings root double.
// NewFactoryTargetCatalogService is a pure delegating constructor that calls
// no method on either injected root before returning, so a structurally
// satisfying zero value is enough to prove the delegation.
type stubOperatorSettingsService struct {
	operatorsettings.Service
}

// stubFactoryDefinitionsService is a minimal Factory Definitions root
// double, for the same reason as stubOperatorSettingsService.
type stubFactoryDefinitionsService struct {
	factorydefinitions.Service
}

func (stubFactoryDefinitionsService) ResolveCurrentFactoryLocation(
	context.Context,
	factorydefinitions.ResolveCurrentFactoryLocationRequest,
) (factorydefinitions.ResolveCurrentFactoryLocationResult, error) {
	return factorydefinitions.ResolveCurrentFactoryLocationResult{}, nil
}

// TestNewFactoryTargetCatalogService_ConstructsFromInjectedRoots proves this
// package's NewFactoryTargetCatalogService is the thin delegation its doc
// comment claims: it forwards the injected Operator Settings and Factory
// Definitions roots straight through to internalservice.New and returns a
// working, non-nil catalog service.
func TestNewFactoryTargetCatalogService_ConstructsFromInjectedRoots(t *testing.T) {
	service, err := NewFactoryTargetCatalogService(stubOperatorSettingsService{}, stubFactoryDefinitionsService{}, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewFactoryTargetCatalogService: unexpected error: %v", err)
	}
	if service == nil {
		t.Fatal("NewFactoryTargetCatalogService returned a nil service with a nil error")
	}
}

// stubResponseBridgeSequencer is structurally sufficient because bridge
// construction stores, but does not invoke, its injected collaborator.
type stubResponseBridgeSequencer struct{ chatsessions.Service }

type stubResponseBridgeFactoryTarget struct {
	factorysessions.Service
}

func TestNewResponseBridgeConstructsFromInjectedSequencer(t *testing.T) {
	bridge := NewResponseBridge(stubResponseBridgeSequencer{}, stubResponseBridgeFactoryTarget{}, nil, logging.NoopLogger{})
	if bridge == nil {
		t.Fatal("NewResponseBridge returned nil")
	}
}

func exerciseServiceLoggerOperations(t *testing.T, store chatsessions.Service) []any {
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
	req := chatsessions.StartTurnRequest{RequestID: validCreateRequest().RequestID, SessionID: created.Session.ID, ExpectedVersion: 1}
	started, err := store.StartTurn(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := store.StartTurn(ctx, req)
	if err != nil || !reflect.DeepEqual(retried, started) {
		t.Fatalf("turn retry: %+v, %v", retried, err)
	}
	busy := req
	busy.RequestID = chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn", JSONRPCStringID: "other"}
	busy.ExpectedVersion = started.Session.Version
	if _, err := store.StartTurn(ctx, busy); !errors.Is(err, chatsessions.ErrBusy) {
		t.Fatalf("busy: %v", err)
	}
	control := chatsessions.RequestControlRequest{RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn", JSONRPCStringID: "control"}, SessionID: created.Session.ID, ExpectedVersion: started.Session.Version, Action: chatsessions.ControlActionCancel}
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
