package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

func TestRootPullModelForScopeValidatesBeforeRuntimeResolution(t *testing.T) {
	t.Parallel()

	root := &Root{}
	if _, err := root.PullModelForScope(t.Context(), models.PullModelRequest{}); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("empty pull request error = %v, want ErrNotFound", err)
	}
	if _, err := root.PullModelForScope(t.Context(), models.PullModelRequest{Name: "voice"}); !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("unavailable scoped runtime error = %v, want ErrUnsupportedOperation", err)
	}
}

func TestRuntimeServicePullModelForScopeValidatesAndDelegates(t *testing.T) {
	t.Parallel()

	runtime := &runtimeService{}
	if _, err := runtime.PullModelForScope(context.Background(), models.PullModelRequest{}); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("empty pull request error = %v, want ErrNotFound", err)
	}
	if _, err := runtime.PullModelForScope(context.Background(), models.PullModelRequest{Name: "voice"}); err == nil {
		t.Fatal("delegated pull error = nil, want unavailable runtime failure")
	}
}

func TestRootCloseRuntimeScopePreventsConcurrentLazyRuntimeReinsertion(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:test:close-race")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	scopes := newCloseRaceRuntimeScopes()
	runtime := &closeRaceRuntime{}
	root := &Root{
		runtimeScopes:  scopes,
		runtimeByScope: make(map[models.RuntimeScopeRef]models.Service),
	}
	invokeResult := make(chan error, 1)
	go func() {
		resolved, invokeErr := root.scopedRuntimeWithBuilder(
			scope,
			func(models.RuntimeBinding) (models.Service, error) { return runtime, nil },
		)
		if invokeErr == nil {
			_, invokeErr = resolved.InvokeLocal(
				context.Background(),
				models.LocalInvocationRequest{Scope: scope},
			)
		}
		invokeResult <- invokeErr
	}()

	awaitCloseRaceSignal(t, scopes.resolveStarted, "initial scope resolution")
	if _, err := root.CloseRuntimeScope(
		context.Background(),
		models.CloseRuntimeScopeRequest{Scope: scope},
	); err != nil {
		t.Fatalf("CloseRuntimeScope() error = %v, want nil", err)
	}

	select {
	case err := <-invokeResult:
		if !errors.Is(err, models.ErrRuntimeScopeClosed) {
			t.Fatalf("concurrent InvokeLocal() error = %v, want ErrRuntimeScopeClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent InvokeLocal() did not return")
	}
	if runtime.invokeCalls != 0 {
		t.Fatalf("closed-scope runtime invocation calls = %d, want 0", runtime.invokeCalls)
	}
	root.runtimeMu.RLock()
	retained := root.runtimeByScope[scope]
	root.runtimeMu.RUnlock()
	if retained != nil {
		t.Fatal("runtime capability was reinserted after its scope closed")
	}
}

type closeRaceRuntimeScopes struct {
	mu             sync.Mutex
	resolveCalls   int
	closed         bool
	resolveStarted chan struct{}
	closeCompleted chan struct{}
	closeOnce      sync.Once
}

func newCloseRaceRuntimeScopes() *closeRaceRuntimeScopes {
	return &closeRaceRuntimeScopes{
		resolveStarted: make(chan struct{}),
		closeCompleted: make(chan struct{}),
	}
}

func (scopes *closeRaceRuntimeScopes) Open(models.RuntimeBinding) (runtimescopes.Reference, error) {
	return "", errors.New("unexpected runtime scope open")
}

func (scopes *closeRaceRuntimeScopes) Resolve(
	runtimescopes.Reference,
) (models.RuntimeBinding, error) {
	scopes.mu.Lock()
	scopes.resolveCalls++
	call := scopes.resolveCalls
	closed := scopes.closed
	scopes.mu.Unlock()
	if call == 1 {
		close(scopes.resolveStarted)
		<-scopes.closeCompleted
		return models.RuntimeBinding{}, nil
	}
	if closed {
		return models.RuntimeBinding{}, runtimescopes.ErrScopeClosed
	}
	return models.RuntimeBinding{}, nil
}

func (scopes *closeRaceRuntimeScopes) Close(runtimescopes.Reference) error {
	scopes.mu.Lock()
	scopes.closed = true
	scopes.mu.Unlock()
	scopes.closeOnce.Do(func() {
		close(scopes.closeCompleted)
	})
	return nil
}

type closeRaceRuntime struct {
	models.Service
	invokeCalls int
}

func (runtime *closeRaceRuntime) InvokeLocal(
	context.Context,
	models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	runtime.invokeCalls++
	return models.LocalInvocationResult{}, nil
}

func awaitCloseRaceSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}
