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
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// stubResponseEventSubscriber always fails to subscribe, so
// RunWithResponseBridge's background bridge goroutine exits immediately
// without needing a working Factory Sessions response-event store -- this
// test proves only pkg/wire's own thin delegation to
// factorysessionsshim.RunWithResponseBridge, not the bridge's concurrency
// semantics, which factorysessionsshim's own tests already cover in depth.
type stubResponseEventSubscriber struct{}

func (stubResponseEventSubscriber) SubscribeFactoryResponseEvents(
	context.Context,
	factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	return nil, errors.New("stub subscriber: no subscription available")
}

// stubEventsAppender is a minimal EventsAppender double for tests that
// construct a Service but do not themselves exercise Sequence.
type stubEventsAppender struct{}

func (stubEventsAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	return events.AppendResult{}, nil
}

// stubEventsReader is a minimal EventsReader double for tests that construct
// a Service but do not themselves exercise AcknowledgeAttachment.
type stubEventsReader struct{}

func (stubEventsReader) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
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
}

func TestNewService_RequiresClock(t *testing.T) {
	service, err := NewService(sequentialIDs("session"), nil, stubEventsAppender{}, stubEventsReader{})
	if err == nil || service != nil {
		t.Fatalf("NewService(nil clock) = (%v, %v), want construction failure", service, err)
	}
}

func TestNewService_RequiresEventsAppender(t *testing.T) {
	service, err := NewService(sequentialIDs("session"), fixedClock(time.Now()), nil, stubEventsReader{})
	if err == nil || service != nil {
		t.Fatalf("NewService(nil events appender) = (%v, %v), want construction failure", service, err)
	}
}

func TestNewService_RequiresEventsReader(t *testing.T) {
	service, err := NewService(sequentialIDs("session"), fixedClock(time.Now()), stubEventsAppender{}, nil)
	if err == nil || service != nil {
		t.Fatalf("NewService(nil events reader) = (%v, %v), want construction failure", service, err)
	}
}

// TestNewService_ConstructsAWorkingService proves the canonically
// constructed Service round-trips a session create/get through the public
// chatsessions.Service interface, without depending on any Store-internal
// detail.
func TestNewService_ConstructsAWorkingService(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	service, err := NewService(sequentialIDs("session"), fixedClock(now), stubEventsAppender{}, stubEventsReader{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if service == nil {
		t.Fatal("NewService returned a nil Service with a nil error")
	}

	ctx := context.Background()
	created, err := service.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	read, err := service.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: created.Session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Session.ID != created.Session.ID {
		t.Fatalf("GetSession returned %q, want %q", read.Session.ID, created.Session.ID)
	}

	if _, err := service.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: "does-not-exist"}); !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("GetSession(unknown): got %v, want ErrNotFound", err)
	}
}

// TestNewService_InstancesShareNoState proves two independently constructed
// Service instances are fully isolated, matching the process-scoped-owner
// guarantee the canonical provider must uphold.
func TestNewService_InstancesShareNoState(t *testing.T) {
	first, err := NewService(sequentialIDs("session"), fixedClock(time.Now()), stubEventsAppender{}, stubEventsReader{})
	if err != nil {
		t.Fatalf("NewService (first): %v", err)
	}
	second, err := NewService(sequentialIDs("session"), fixedClock(time.Now()), stubEventsAppender{}, stubEventsReader{})
	if err != nil {
		t.Fatalf("NewService (second): %v", err)
	}

	ctx := context.Background()
	created, err := first.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := second.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: created.Session.ID}); !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("second Service observed the first Service's session: got %v, want ErrNotFound", err)
	}
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

// stubFactoryTargetExecutionService is a minimal FactoryTargetExecutionService
// double. NewFactoryTargetService is a pure delegating constructor that
// calls no method on the injected execution service before returning, so a
// structurally satisfying zero value is enough to prove the delegation.
type stubFactoryTargetExecutionService struct {
	FactoryTargetExecutionService
}

// TestNewFactoryTargetService_WrapsExecutionServiceInTheShim proves this
// package's NewFactoryTargetService is the thin delegation its doc comment
// claims: it forwards the injected execution service straight through to
// factorysessionsshim.New and returns a non-nil FactoryTargetService.
func TestNewFactoryTargetService_WrapsExecutionServiceInTheShim(t *testing.T) {
	service := NewFactoryTargetService(stubFactoryTargetExecutionService{})
	if service == nil {
		t.Fatal("NewFactoryTargetService returned a nil FactoryTargetService")
	}
}

// TestRunWithResponseBridge_ForwardsInvokeResultUnchanged proves this
// package's RunWithResponseBridge is exactly the thin delegation its doc
// comment claims: it returns invoke's own result and error unchanged, the
// one observable behavior a caller holding only pkg/wire's re-published
// function shape can depend on.
func TestRunWithResponseBridge_ForwardsInvokeResultUnchanged(t *testing.T) {
	service, err := NewService(sequentialIDs("session"), fixedClock(time.Now()), stubEventsAppender{}, stubEventsReader{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	created, err := service.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	wantResult := factorysessions.InvocationResult{RequestID: "req-1", Status: factorysessions.InvocationTerminalStatusCompleted}
	wantErr := errors.New("invoke failed")
	invoke := func(context.Context) (factorysessions.InvocationResult, error) {
		return wantResult, wantErr
	}

	gotResult, gotErr := RunWithResponseBridge(
		context.Background(),
		service,
		stubResponseEventSubscriber{},
		created.Session.ID,
		created.Session.Version,
		"factory-session-1",
		nil,
		invoke,
	)

	if gotErr != wantErr {
		t.Fatalf("RunWithResponseBridge() error = %v, want the exact same error value %v", gotErr, wantErr)
	}
	if !reflect.DeepEqual(gotResult, wantResult) {
		t.Fatalf("RunWithResponseBridge() result = %#v, want %#v", gotResult, wantResult)
	}
}
