package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
	liveruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
)

type concurrentRegistryState struct {
	registry   *sessionregistry.Registry
	mu         sync.Mutex
	nextID     atomic.Int32
	openCalls  atomic.Int32
	closeCalls atomic.Int32
}

func newConcurrentRegistryState() *concurrentRegistryState {
	return &concurrentRegistryState{
		registry: sessionregistry.New(),
	}
}

func (state *concurrentRegistryState) dependencies() liveruntime.Dependencies {
	return liveruntime.Dependencies{
		OpenForTarget: state.openForTarget,
		ListSessionIDs: func() []string {
			return state.registry.IDs()
		},
		GetSession: func(id string) *livesession.LiveSession {
			return state.registry.Get(id)
		},
		RequireSession: func(id string) (*livesession.LiveSession, error) {
			if session := state.registry.Get(id); session != nil {
				return session, nil
			}
			return nil, factorysessions.ErrNotFound
		},
		BuildProjectionContext: func(_ context.Context, session *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
			return factorysessions.ProjectionContext{
				Session:          &factorysessions.ScopedLiveSessionSummary{ID: session.ID},
				FactorySessionID: session.ID,
			}, nil
		},
		SessionFactory: func(string) (factoryruntime.Service, error) {
			return &testFactoryRuntime{}, nil
		},
		StopSession: state.stopSession,
		ObserveControl: func(string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest, factorysessions.LifecycleControlOutcome, factorysessions.LifecycleStatus, error) {
		},
	}
}

// openForTarget mirrors production open semantics: each successful activation
// allocates a new session identity and registers it without logical-key dedupe.
func (state *concurrentRegistryState) openForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	state.openCalls.Add(1)
	time.Sleep(2 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sessionID := fmt.Sprintf("sess-open-%d", state.nextID.Add(1))
	session := &livesession.LiveSession{
		ID: sessionID,
		SessionState: livesession.SessionState{
			FactoryDir: target.FactoryDir,
			FolderPath: target.FolderPath,
		},
		Target: target.Ref,
	}
	state.registry.Upsert(session, state.registry.Count() == 0)
	return sessionID, nil
}

func (state *concurrentRegistryState) stopSession(sessionID string) error {
	state.closeCalls.Add(1)
	state.mu.Lock()
	defer state.mu.Unlock()
	state.registry.Remove(sessionID)
	return nil
}

func TestServiceConcurrentOpenForTargetAllocatesDistinctActivations(t *testing.T) {
	t.Parallel()

	state := newConcurrentRegistryState()
	service, err := liveruntimewire.NewService(state.dependencies())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	target := factorysessions.Target{
		Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		FactoryDir: "/tmp/factory",
		FolderPath: "/tmp",
	}

	const workers = 32
	ids := make([]string, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for index := range workers {
		go func(slot int) {
			defer wg.Done()
			opened, openErr := service.OpenForTarget(context.Background(), target)
			if openErr != nil {
				t.Errorf("OpenForTarget: %v", openErr)
				return
			}
			ids[slot] = opened
		}(index)
	}
	wg.Wait()

	seen := make(map[string]struct{}, workers)
	for index, id := range ids {
		if id == "" {
			t.Fatalf("OpenForTarget[%d] returned empty identity", index)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("OpenForTarget[%d] = %q, want distinct identity per activation", index, id)
		}
		seen[id] = struct{}{}
	}
	if state.registry.Count() != workers {
		t.Fatalf("registry count = %d, want %d distinct activations", state.registry.Count(), workers)
	}
	if state.openCalls.Load() != workers {
		t.Fatalf("activation calls = %d, want %d", state.openCalls.Load(), workers)
	}
}

func TestServiceConcurrentResolveReturnsStableActiveIdentity(t *testing.T) {
	t.Parallel()

	state := newConcurrentRegistryState()
	service, err := liveruntimewire.NewService(state.dependencies())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	target := factorysessions.Target{
		Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		FactoryDir: "/tmp/factory",
		FolderPath: "/tmp",
	}
	sessionID, err := service.OpenForTarget(context.Background(), target)
	if err != nil {
		t.Fatalf("OpenForTarget: %v", err)
	}

	const workers = 24
	handles := make([]*livesession.LiveSession, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for index := range workers {
		go func(slot int) {
			defer wg.Done()
			handles[slot] = service.Resolve(sessionID)
		}(index)
	}
	wg.Wait()

	for index, handle := range handles {
		if handle == nil || handle.ID != sessionID {
			t.Fatalf("Resolve[%d] = %#v, want %q", index, handle, sessionID)
		}
		if index > 0 && handles[0] != handle {
			t.Fatal("Resolve returned different handles for the same active session")
		}
	}
}

func TestServiceConcurrentCloseWithResolveLeavesDeterminatePostState(t *testing.T) {
	t.Parallel()

	state := newConcurrentRegistryState()
	service, err := liveruntimewire.NewService(state.dependencies())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	target := factorysessions.Target{
		Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		FactoryDir: "/tmp/factory",
		FolderPath: "/tmp",
	}
	sessionID, err := service.OpenForTarget(context.Background(), target)
	if err != nil {
		t.Fatalf("OpenForTarget: %v", err)
	}

	const workers = 24
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			if service.Resolve(sessionID) != nil {
				_ = service.Close(context.Background(), sessionID)
				return
			}
			_, _ = service.Get(context.Background(), sessionID)
		}()
	}
	wg.Wait()

	active := service.Resolve(sessionID)
	if active != nil {
		if state.registry.Count() != 1 {
			t.Fatalf("active session count = %d, want 1", state.registry.Count())
		}
		return
	}
	if state.registry.Count() != 0 {
		t.Fatalf("inactive registry count = %d, want 0", state.registry.Count())
	}
	_, getErr := service.Get(context.Background(), sessionID)
	if getErr == nil || !errors.Is(getErr, factorysessions.ErrNotFound) {
		t.Fatalf("Get after concurrent close = %v, want ErrNotFound", getErr)
	}
}

func TestServiceOpenForTargetCancellationDoesNotLeaveActiveRegistryEntry(t *testing.T) {
	t.Parallel()

	state := newConcurrentRegistryState()
	dependencies := state.dependencies()
	originalOpen := dependencies.OpenForTarget
	dependencies.OpenForTarget = func(ctx context.Context, target factorysessions.Target) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		return originalOpen(ctx, target)
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	target := factorysessions.Target{
		Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		FactoryDir: "/tmp/factory",
		FolderPath: "/tmp",
	}
	_, err = service.OpenForTarget(ctx, target)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenForTarget error = %v, want context canceled", err)
	}
	if state.registry.Count() != 0 {
		t.Fatalf("registry count after cancelled open = %d, want 0", state.registry.Count())
	}
}
