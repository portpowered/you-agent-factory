package wire

import (
	"context"
	"errors"
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
	factorysessions.TargetExecutionService
}

func TestNewResponseBridgeConstructsFromInjectedSequencer(t *testing.T) {
	bridge := NewResponseBridge(stubResponseBridgeSequencer{}, stubResponseBridgeFactoryTarget{}, logging.NoopLogger{})
	if bridge == nil {
		t.Fatal("NewResponseBridge returned nil")
	}
}
