package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
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
	sessions   map[string]*livesession.LiveSession
	mu         sync.Mutex
	openCalls  atomic.Int32
	closeCalls atomic.Int32
}

func newConcurrentRegistryState() *concurrentRegistryState {
	return &concurrentRegistryState{
		registry: sessionregistry.New(),
		sessions: make(map[string]*livesession.LiveSession),
	}
}

func legacyKeyFromTarget(target factorysessions.Target) string {
	folderPath := filepath.Clean(strings.TrimSpace(target.FolderPath))
	if folderPath == "." {
		folderPath = ""
	}
	folderPath = filepath.ToSlash(folderPath)
	targetKind := strings.TrimSpace(string(target.Ref.Kind))
	targetName := strings.TrimSpace(target.Ref.Name)
	if targetKind == "" {
		targetKind = string(factorysessions.TargetKindDefault)
	}
	return strings.Join([]string{folderPath, targetKind, targetName}, "::")
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

func (state *concurrentRegistryState) openForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if existing := state.registry.FindByLogicalSessionKeyID(legacyKeyFromTarget(target)); existing != nil {
		state.registry.Select(existing.ID)
		return existing.ID, nil
	}
	state.openCalls.Add(1)
	time.Sleep(2 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sessionID := "sess-concurrent-open"
	session := &livesession.LiveSession{
		ID: sessionID,
		SessionState: livesession.SessionState{
			FactoryDir: target.FactoryDir,
			FolderPath: target.FolderPath,
		},
		Target: target.Ref,
	}
	state.registry.Upsert(session, true)
	state.sessions[sessionID] = session
	return sessionID, nil
}

func (state *concurrentRegistryState) stopSession(sessionID string) error {
	state.closeCalls.Add(1)
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.sessions, sessionID)
	state.registry.Remove(sessionID)
	return nil
}

func TestServiceConcurrentOpenForTargetConvergesOnSingleRegistryIdentity(t *testing.T) {
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

	for index, id := range ids {
		if id != "sess-concurrent-open" {
			t.Fatalf("OpenForTarget[%d] = %q, want sess-concurrent-open", index, id)
		}
	}
	if state.registry.Count() != 1 {
		t.Fatalf("registry count = %d, want 1", state.registry.Count())
	}
	if state.openCalls.Load() != 1 {
		t.Fatalf("activation calls = %d, want 1", state.openCalls.Load())
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
	if _, err := service.OpenForTarget(context.Background(), target); err != nil {
		t.Fatalf("OpenForTarget: %v", err)
	}

	const workers = 24
	handles := make([]*livesession.LiveSession, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for index := range workers {
		go func(slot int) {
			defer wg.Done()
			handles[slot] = service.Resolve("sess-concurrent-open")
		}(index)
	}
	wg.Wait()

	for index, handle := range handles {
		if handle == nil || handle.ID != "sess-concurrent-open" {
			t.Fatalf("Resolve[%d] = %#v, want sess-concurrent-open", index, handle)
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
	if _, err := service.OpenForTarget(context.Background(), target); err != nil {
		t.Fatalf("OpenForTarget: %v", err)
	}

	const workers = 24
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			if service.Resolve("sess-concurrent-open") != nil {
				_ = service.Close(context.Background(), "sess-concurrent-open")
				return
			}
			_, _ = service.Get(context.Background(), "sess-concurrent-open")
		}()
	}
	wg.Wait()

	active := service.Resolve("sess-concurrent-open")
	if active != nil {
		if state.registry.Count() != 1 {
			t.Fatalf("active session count = %d, want 1", state.registry.Count())
		}
		return
	}
	if state.registry.Count() != 0 {
		t.Fatalf("inactive registry count = %d, want 0", state.registry.Count())
	}
	_, getErr := service.Get(context.Background(), "sess-concurrent-open")
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
	if service.Resolve("sess-concurrent-open") != nil {
		t.Fatal("cancelled open left an active registry identity")
	}
}
