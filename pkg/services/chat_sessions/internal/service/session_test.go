package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// sequentialIDs returns a deterministic IDGenerator that yields prefix-1,
// prefix-2, ... so tests can assert on exact generated identities.
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
		Cwd:           "/workspace/project",
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
	}
}

func TestStore_CreateSession_Success(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	store := NewStore(sequentialIDs("session"), fixedClock(now))

	result, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	session := result.Session
	if session.ID == "" {
		t.Fatal("CreateSession: expected a non-blank session ID")
	}
	if session.State != chatsessions.SessionStateCreated {
		t.Fatalf("State = %v, want CREATED", session.State)
	}
	if session.Cwd != "/workspace/project" {
		t.Fatalf("Cwd = %q, want /workspace/project", session.Cwd)
	}
	if session.SelectedTarget.Ref != "factory:@you/review" {
		t.Fatalf("SelectedTarget = %+v, want the requested initial target", session.SelectedTarget)
	}
	if session.TargetEpisode != 1 {
		t.Fatalf("TargetEpisode = %d, want 1", session.TargetEpisode)
	}
	if session.ActiveTurnID != "" {
		t.Fatalf("ActiveTurnID = %q, want blank", session.ActiveTurnID)
	}
	if session.Version == 0 {
		t.Fatal("Version = 0, want a non-zero initial version")
	}
	if session.CreatedAt.IsZero() || session.UpdatedAt.IsZero() {
		t.Fatalf("CreatedAt/UpdatedAt must be non-zero, got %+v", session)
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("created Session fails its own Validate: %v", err)
	}
}

// TestStore_CreateSession_InvalidInputCreatesNoSession proves a blank Cwd,
// invalid RequestID, or invalid InitialTarget reports the existing typed
// validation classification and leaves the Store empty.
func TestStore_CreateSession_InvalidInputCreatesNoSession(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mutate  func(chatsessions.CreateSessionRequest) chatsessions.CreateSessionRequest
		wantErr error
	}{
		{"blank cwd", func(r chatsessions.CreateSessionRequest) chatsessions.CreateSessionRequest {
			r.Cwd = ""
			return r
		}, chatsessions.ErrRequiredValue},
		{"invalid request identity", func(r chatsessions.CreateSessionRequest) chatsessions.CreateSessionRequest {
			r.RequestID = chatsessions.RequestIdentity{}
			return r
		}, chatsessions.ErrUnknownEnumValue},
		{"invalid initial target", func(r chatsessions.CreateSessionRequest) chatsessions.CreateSessionRequest {
			r.InitialTarget = chatsessions.ChatTargetRef{}
			return r
		}, chatsessions.ErrUnknownEnumValue},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := NewStore(sequentialIDs("session"), fixedClock(time.Now()))

			_, err := store.CreateSession(ctx, tt.mutate(validCreateRequest()))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateSession(%s): got %v, want %v", tt.name, err, tt.wantErr)
			}
			store.mu.RLock()
			count := len(store.sessions)
			store.mu.RUnlock()
			if count != 0 {
				t.Fatalf("CreateSession(%s): got %d observable sessions, want 0", tt.name, count)
			}
		})
	}
}

// TestStore_CreateSession_ReturnsDetachedValue proves mutating a returned
// Session cannot change what a later GetSession observes.
func TestStore_CreateSession_ReturnsDetachedValue(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()))

	created, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	mutated := created.Session
	mutated.SelectedTarget.Ref = "factory:@you/mutated"

	reread, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: created.Session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if reread.Session.SelectedTarget.Ref != "factory:@you/review" {
		t.Fatalf("GetSession observed a mutation made to a previously returned value: %+v", reread.Session)
	}
}

// TestStore_CreateSession_UniqueIDs proves two successful creates never
// collide on generated session identity.
func TestStore_CreateSession_UniqueIDs(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()))

	first, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession first: %v", err)
	}
	second, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession second: %v", err)
	}
	if first.Session.ID == second.Session.ID {
		t.Fatalf("two CreateSession calls returned the same ID %q", first.Session.ID)
	}
}

func TestStore_GetSession_UnknownIDIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()))

	_, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: "does-not-exist"})
	if !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("GetSession unknown id: got %v, want ErrNotFound", err)
	}
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("GetSession unknown id: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Session" || notFound.ID != "does-not-exist" {
		t.Fatalf("NotFoundError = %+v, want Value=Session ID=does-not-exist", notFound)
	}

	store.mu.RLock()
	count := len(store.sessions)
	store.mu.RUnlock()
	if count != 0 {
		t.Fatalf("GetSession on an unknown ID created %d placeholder sessions, want 0", count)
	}
}

// TestStore_InstancesShareNoState proves two independently constructed Store
// instances are fully isolated: a session created in one is invisible to the
// other.
func TestStore_InstancesShareNoState(t *testing.T) {
	ctx := context.Background()
	first := NewStore(sequentialIDs("session"), fixedClock(time.Now()))
	second := NewStore(sequentialIDs("session"), fixedClock(time.Now()))

	created, err := first.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := second.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: created.Session.ID}); !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("second Store observed the first Store's session: got %v, want ErrNotFound", err)
	}
}
