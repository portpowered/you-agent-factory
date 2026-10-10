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

// TestNewService_ConstructsAWorkingService proves the canonically
// constructed Service round-trips a session create/get through the public
// chatsessions.Service interface, without depending on any Store-internal
// detail.
func TestNewService_ConstructsAWorkingService(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"noop", "capture"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
			first := &serviceCaptureLogger{}
			var logger logging.Logger = logging.NoopLogger{}
			if mode == "capture" {
				logger = first
			}
			idCalls, clockCalls, eventCalls := 0, 0, 0
			ids := sequentialIDs("session")
			service := NewService(func() string { idCalls++; return ids() }, func() time.Time { clockCalls++; return at }, stubEventsAppender{calls: &eventCalls}, stubEventsReader{calls: &eventCalls}, logger)
			if idCalls != 0 || clockCalls != 0 || eventCalls != 0 || len(first.calls) != 0 {
				t.Fatal("construction invoked effects")
			}
			got := exerciseServiceLoggerOperations(t, service)
			quiet := NewService(sequentialIDs("session"), fixedClock(at), stubEventsAppender{}, stubEventsReader{}, logging.NoopLogger{})
			want := exerciseServiceLoggerOperations(t, quiet)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s changed operation facts: %+v / %+v", mode, got, want)
			}
			if mode == "capture" {
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
	first := NewService(sequentialIDs("first"), fixedClock(at), stubEventsAppender{}, stubEventsReader{}, a)
	second := NewService(sequentialIDs("second"), fixedClock(at.Add(time.Hour)), stubEventsAppender{}, stubEventsReader{}, b)
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

// Controlled peers for the retained catalog provider compatibility witness.
type stubOperatorSettingsService struct {
	operatorsettings.Service
	calls *int
}

func (s stubOperatorSettingsService) ResolveACPAgentProfile(string) (operatorsettings.ACPAgentProfile, error) {
	*s.calls++
	return operatorsettings.ACPAgentProfile{DefaultTarget: "factory:@you/review"}, nil
}

type stubFactoryDefinitionsService struct {
	factorydefinitions.CatalogPathsService
	calls     *int
	installed *bool
}

func (s stubFactoryDefinitionsService) ListEffectiveFactories(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	*s.calls++
	if !*s.installed {
		return factorydefinitions.ListEffectiveFactoriesResult{}, nil
	}
	location := "/private/factories/review"
	return factorydefinitions.ListEffectiveFactoriesResult{Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{Name: "@you/review", Location: &location}}}, nil
}

// Existing provider witness: retained wiring integration, not a unit test.
func TestNewFactoryTargetCatalogService_ConstructsFromInjectedRoots(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"noop", "capture"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			capture := &serviceCaptureLogger{}
			var logger logging.Logger = logging.NoopLogger{}
			if mode == "noop" {
				logger = logging.NoopLogger{}
			}
			if mode == "capture" {
				logger = capture
			}
			profileCalls, catalogCalls, installed := 0, 0, true
			service := NewFactoryTargetCatalogService(stubOperatorSettingsService{calls: &profileCalls}, stubFactoryDefinitionsService{calls: &catalogCalls, installed: &installed}, logger)
			if profileCalls != 0 || catalogCalls != 0 || len(capture.calls) != 0 {
				t.Fatal("construction invoked effects")
			}
			req := chatsessions.ResolveFactoryTargetCatalogRequest{OperatorSettingsPath: "/private/operator.json"}
			result, err := service.ResolveFactoryTargetCatalog(context.Background(), req)
			want := chatsessions.ResolveFactoryTargetCatalogResult{CurrentTarget: "factory:@you/review", Choices: []chatsessions.FactoryTargetCatalogChoice{{Value: "factory:@you/review", Name: "@you/review"}}}
			if err != nil || !reflect.DeepEqual(result, want) {
				t.Fatalf("%s success: %+v, %v", mode, result, err)
			}
			installed = false
			result, err = service.ResolveFactoryTargetCatalog(context.Background(), req)
			var typed *chatsessions.FactoryTargetCatalogError
			if !errors.Is(err, chatsessions.ErrFactoryTargetCatalogEmpty) || !errors.As(err, &typed) || typed.Target != "" || typed.Cause != nil || !reflect.DeepEqual(result, chatsessions.ResolveFactoryTargetCatalogResult{}) {
				t.Fatalf("%s failure: %+v, %v", mode, result, err)
			}
			if profileCalls != 2 || catalogCalls != 2 {
				t.Fatalf("operation observations: %d / %d", profileCalls, catalogCalls)
			}
			var wantLogs []serviceLogCall
			if mode == "capture" {
				wantLogs = []serviceLogCall{
					{"info", "chat_sessions.resolve_factory_target_catalog.started", nil},
					{"info", "chat_sessions.resolve_factory_target_catalog.finished", []any{"choice_count", 1}},
					{"info", "chat_sessions.resolve_factory_target_catalog.started", nil},
					{"warn", "chat_sessions.resolve_factory_target_catalog.failed", []any{"reason", "catalog_empty"}},
				}
			}
			if !reflect.DeepEqual(capture.calls, wantLogs) {
				t.Fatalf("%s diagnostics: %+v, want %+v", mode, capture.calls, wantLogs)
			}
		})
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
	created, read, started, req := startServiceLoggerOperations(t, store)
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

func startServiceLoggerOperations(t *testing.T, store chatsessions.Service) (chatsessions.CreateSessionResult, chatsessions.GetSessionResult, chatsessions.StartTurnResult, chatsessions.StartTurnRequest) {
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
	return created, read, started, req
}
