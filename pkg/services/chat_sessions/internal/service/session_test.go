package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
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
		WorkingRoot:   "/workspace/project",
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
	if session.WorkingRoot != "/workspace/project" {
		t.Fatalf("WorkingRoot = %q, want /workspace/project", session.WorkingRoot)
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

// TestStore_CreateSession_InvalidInputCreatesNoSession proves a blank WorkingRoot,
// invalid RequestID, or invalid InitialTarget reports the existing typed
// validation classification and leaves the Store empty.
func TestStore_CreateSession_InvalidInputCreatesNoSession(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mutate  func(chatsessions.CreateSessionRequest) chatsessions.CreateSessionRequest
		wantErr error
	}{
		{"blank working root", func(r chatsessions.CreateSessionRequest) chatsessions.CreateSessionRequest {
			r.WorkingRoot = ""
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

// TestStore_CreateSession_ConcurrentDifferentSessionsAreIndependent proves
// CreateSession is safe under concurrent calls that each create a distinct
// session, even though the injected IDGenerator here (sequentialIDs)
// mutates a plain closure variable with no synchronization of its own:
// Store must serialize its own calls to newID/now under s.mu rather than
// require every caller-supplied dependency to be concurrency-safe by
// contract. It also proves the resulting sessions are independent -- a
// concurrent SetTarget against each of the n distinct sessions afterward
// succeeds for every one with no cross-session interference.
func TestStore_CreateSession_ConcurrentDifferentSessionsAreIndependent(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()))

	const n = 25
	var wg sync.WaitGroup
	results := make([]chatsessions.CreateSessionResult, n)
	createErrs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], createErrs[i] = store.CreateSession(ctx, chatsessions.CreateSessionRequest{
				RequestID: chatsessions.RequestIdentity{
					Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: fmt.Sprintf("conn-%d", i), JSONRPCStringID: "req-1",
				},
				WorkingRoot:   fmt.Sprintf("/workspace/project-%d", i),
				InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
			})
		}(i)
	}
	wg.Wait()

	seenIDs := make(map[string]int, n)
	for i, err := range createErrs {
		if err != nil {
			t.Fatalf("CreateSession[%d]: unexpected error %v", i, err)
		}
		seenIDs[results[i].Session.ID]++
		wantWorkingRoot := fmt.Sprintf("/workspace/project-%d", i)
		if results[i].Session.WorkingRoot != wantWorkingRoot {
			t.Fatalf("CreateSession[%d]: WorkingRoot = %q, want %q", i, results[i].Session.WorkingRoot, wantWorkingRoot)
		}
	}
	for id, count := range seenIDs {
		if count != 1 {
			t.Fatalf("session ID %q was assigned to %d concurrent CreateSession calls, want a unique ID per call", id, count)
		}
	}
	if len(seenIDs) != n {
		t.Fatalf("got %d unique session IDs from %d concurrent CreateSession calls, want %d", len(seenIDs), n, n)
	}

	var mutateWG sync.WaitGroup
	setErrs := make([]error, n)
	for i := range n {
		mutateWG.Add(1)
		go func(i int) {
			defer mutateWG.Done()
			_, setErrs[i] = store.SetTarget(ctx, chatsessions.SetTargetRequest{
				RequestID:       setTargetRequestID(fmt.Sprintf("retarget-%d", i)),
				SessionID:       results[i].Session.ID,
				ExpectedVersion: results[i].Session.Version,
				Target:          otherTarget(),
			})
		}(i)
	}
	mutateWG.Wait()

	for i, err := range setErrs {
		if err != nil {
			t.Fatalf("SetTarget[%d]: unexpected error %v", i, err)
		}
	}
	for i := range n {
		final, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: results[i].Session.ID})
		if err != nil {
			t.Fatalf("GetSession[%d]: %v", i, err)
		}
		if final.Session.SelectedTarget != otherTarget() {
			t.Fatalf("GetSession[%d]: SelectedTarget = %+v, want %+v", i, final.Session.SelectedTarget, otherTarget())
		}
		if final.Session.Version != results[i].Session.Version+1 {
			t.Fatalf("GetSession[%d]: Version = %d, want %d", i, final.Session.Version, results[i].Session.Version+1)
		}
	}
}
