package wire

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

// stubEventsAppender is a minimal EventsAppender double for tests that
// construct a Service but do not themselves exercise Sequence.
type stubEventsAppender struct{}

func (stubEventsAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	return events.AppendResult{}, nil
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
	service, err := NewService(nil, fixedClock(time.Now()), stubEventsAppender{})
	if err == nil || service != nil {
		t.Fatalf("NewService(nil id generator) = (%v, %v), want construction failure", service, err)
	}
}

func TestNewService_RequiresClock(t *testing.T) {
	service, err := NewService(sequentialIDs("session"), nil, stubEventsAppender{})
	if err == nil || service != nil {
		t.Fatalf("NewService(nil clock) = (%v, %v), want construction failure", service, err)
	}
}

func TestNewService_RequiresEventsAppender(t *testing.T) {
	service, err := NewService(sequentialIDs("session"), fixedClock(time.Now()), nil)
	if err == nil || service != nil {
		t.Fatalf("NewService(nil events appender) = (%v, %v), want construction failure", service, err)
	}
}

// TestNewService_ConstructsAWorkingService proves the canonically
// constructed Service round-trips a session create/get through the public
// chatsessions.Service interface, without depending on any Store-internal
// detail.
func TestNewService_ConstructsAWorkingService(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	service, err := NewService(sequentialIDs("session"), fixedClock(now), stubEventsAppender{})
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
	first, err := NewService(sequentialIDs("session"), fixedClock(time.Now()), stubEventsAppender{})
	if err != nil {
		t.Fatalf("NewService (first): %v", err)
	}
	second, err := NewService(sequentialIDs("session"), fixedClock(time.Now()), stubEventsAppender{})
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
