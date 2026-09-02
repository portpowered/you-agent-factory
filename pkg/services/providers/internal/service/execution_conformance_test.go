package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	executiontest "github.com/portpowered/infinite-you/pkg/services/providers/internal/testutil/execution"
)

func TestStreamingAdapterConformance(t *testing.T) {
	executiontest.Run(t, executiontest.Subject{
		NewAdapter:       newStreamingAdapter,
		NewRoot:          newConformanceRoot,
		SupportsProgress: true,
	})
}

func TestFinalOnlyAdapterConformance(t *testing.T) {
	executiontest.Run(t, executiontest.Subject{
		NewAdapter: newFinalOnlyAdapter,
		NewRoot:    newConformanceRoot,
	})
}

func newConformanceRoot(
	attempt execution.Attempt,
) (providers.Service, error) {
	catalogService, err := catalogwire.NewService()
	if err != nil {
		return nil, err
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt:  attempt,
		},
	)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalogService, executionService, logging.NoopLogger{})
}

type streamingAdapter struct {
	mu          sync.Mutex
	plan        executiontest.Plan
	started     chan struct{}
	startOnce   sync.Once
	observation executiontest.Observation
}

func newStreamingAdapter(plan executiontest.Plan) executiontest.Adapter {
	adapter := &streamingAdapter{
		plan:    plan,
		started: make(chan struct{}),
	}
	return executiontest.Adapter{
		Attempt: adapter.attempt,
		Observe: adapter.observe,
		Started: adapter.started,
	}
}

func (adapter *streamingAdapter) attempt(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	adapter.recordStart(request)
	defer adapter.recordCleanup()
	if adapter.plan.MutateRequest {
		request.UserMessage = "streaming-adapter-mutated"
	}
	if adapter.plan.WaitForContext {
		<-ctx.Done()
		return providers.ExecuteResult{}, ctx.Err()
	}
	if adapter.plan.ReturnSuccessAfterContext {
		<-ctx.Done()
		return adapter.plan.Result, nil
	}
	return adapter.plan.Result, adapter.plan.Failure
}

func (adapter *streamingAdapter) recordStart(request providers.ExecuteRequest) {
	adapter.mu.Lock()
	adapter.observation.Calls++
	adapter.observation.Requests = append(
		adapter.observation.Requests,
		request.Clone(),
	)
	adapter.mu.Unlock()
	adapter.startOnce.Do(func() { close(adapter.started) })
}

func (adapter *streamingAdapter) recordCleanup() {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	adapter.observation.Cleanups++
}

func (adapter *streamingAdapter) observe() executiontest.Observation {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	return adapter.observation.Clone()
}

type finalOnlyState struct {
	mu          sync.Mutex
	observation executiontest.Observation
}

func newFinalOnlyAdapter(plan executiontest.Plan) executiontest.Adapter {
	state := &finalOnlyState{}
	started := make(chan struct{})
	var startOnce sync.Once
	attempt := func(
		ctx context.Context,
		request providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		state.mu.Lock()
		state.observation.Calls++
		state.observation.Requests = append(
			state.observation.Requests,
			request.Clone(),
		)
		state.mu.Unlock()
		startOnce.Do(func() { close(started) })
		defer func() {
			state.mu.Lock()
			state.observation.Cleanups++
			state.mu.Unlock()
		}()
		if plan.MutateRequest {
			request.UserMessage = "final-adapter-mutated"
		}
		if plan.WaitForContext {
			<-ctx.Done()
			return providers.ExecuteResult{}, ctx.Err()
		}
		if plan.ReturnSuccessAfterContext {
			<-ctx.Done()
			return plan.Result, nil
		}
		return plan.Result, plan.Failure
	}
	return executiontest.Adapter{
		Attempt: attempt,
		Started: started,
		Observe: func() executiontest.Observation {
			state.mu.Lock()
			defer state.mu.Unlock()
			return state.observation.Clone()
		},
	}
}

// gatedAttempt lets a test hold one execution in flight (after signaling it
// started) until the test releases it, without sleep-based synchronization.
type gatedAttempt struct {
	mu       sync.Mutex
	calls    int
	started  chan struct{}
	release  chan struct{}
	gateOnce bool
}

func newGatedAttempt(gateOnce bool) *gatedAttempt {
	return &gatedAttempt{
		started:  make(chan struct{}, 8),
		release:  make(chan struct{}),
		gateOnce: gateOnce,
	}
}

func (g *gatedAttempt) attempt(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	g.mu.Lock()
	g.calls++
	call := g.calls
	g.mu.Unlock()

	if !g.gateOnce || call == 1 {
		g.started <- struct{}{}
		<-g.release
	}
	return providers.ExecuteResult{Content: request.AttemptID}, nil
}

func (g *gatedAttempt) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func mustCorrelationRootService(t *testing.T, attempt execution.Attempt) providers.Service {
	t.Helper()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{Provider: providers.IDCodex, Attempt: attempt},
		execution.Registration{Provider: providers.IDClaude, Attempt: attempt},
	)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.New(catalogService, executionService, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return root
}

func assertCollisionFailure(t *testing.T, err error) {
	t.Helper()
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) ||
		failure.Kind != providers.ExecuteFailureKindInvalidRequest ||
		!strings.Contains(failure.Message, "already live") {
		t.Fatalf("Execute() error = %#v, want invalid-request already-live collision failure", err)
	}
}

func TestExecute_CollidesOnSameCanonicalProviderAndAttemptIDWhileLive(t *testing.T) {
	t.Parallel()

	gate := newGatedAttempt(false)
	root := mustCorrelationRootService(t, gate.attempt)

	firstDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "dup-1",
		})
		firstDone <- err
	}()

	<-gate.started

	_, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "dup-1",
	})
	assertCollisionFailure(t, err)
	if calls := gate.callCount(); calls != 1 {
		t.Fatalf("adapter calls during collision = %d, want 1 (second request must not reach the adapter)", calls)
	}

	close(gate.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Execute() error = %v, want nil", err)
	}
}

func TestExecute_CollisionUsesCanonicalIdentityAcrossAcceptedAlias(t *testing.T) {
	t.Parallel()

	gate := newGatedAttempt(false)
	root := mustCorrelationRootService(t, gate.attempt)

	firstDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "dup-alias",
		})
		firstDone <- err
	}()

	<-gate.started

	_, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.ID("  CODEX  "),
		AttemptID: "dup-alias",
	})
	assertCollisionFailure(t, err)

	close(gate.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Execute() error = %v, want nil", err)
	}
}

func TestExecute_DistinctAttemptIDsUnderSameProviderAreIndependentlyLive(t *testing.T) {
	t.Parallel()

	gate := newGatedAttempt(false)
	root := mustCorrelationRootService(t, gate.attempt)

	firstDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "attempt-a",
		})
		firstDone <- err
	}()
	<-gate.started

	secondDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "attempt-b",
		})
		secondDone <- err
	}()
	<-gate.started

	close(gate.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Execute() error = %v, want nil", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second Execute() error = %v, want nil", err)
	}
	if calls := gate.callCount(); calls != 2 {
		t.Fatalf("adapter calls = %d, want 2 (distinct attempt ids must not collide)", calls)
	}
}

func TestExecute_SameAttemptIDUnderDifferentProviderIsIndependentlyLive(t *testing.T) {
	t.Parallel()

	gate := newGatedAttempt(false)
	root := mustCorrelationRootService(t, gate.attempt)

	firstDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "shared-attempt",
		})
		firstDone <- err
	}()
	<-gate.started

	secondDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDClaude,
			AttemptID: "shared-attempt",
		})
		secondDone <- err
	}()
	<-gate.started

	close(gate.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Execute() error = %v, want nil", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second Execute() error = %v, want nil", err)
	}
	if calls := gate.callCount(); calls != 2 {
		t.Fatalf("adapter calls = %d, want 2 (same attempt id under different providers must not collide)", calls)
	}
}

func TestExecute_ReleasesLiveIdentityAfterSuccess(t *testing.T) {
	t.Parallel()

	gate := newGatedAttempt(true)
	root := mustCorrelationRootService(t, gate.attempt)

	firstDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "reuse-success",
		})
		firstDone <- err
	}()
	<-gate.started
	close(gate.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Execute() error = %v, want nil", err)
	}

	if _, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "reuse-success",
	}); err != nil {
		t.Fatalf("Execute() after prior terminal success error = %v, want nil (identity must be released)", err)
	}
	if calls := gate.callCount(); calls != 2 {
		t.Fatalf("adapter calls = %d, want 2", calls)
	}
}

func TestExecute_ReleasesLiveIdentityAfterTypedFailure(t *testing.T) {
	t.Parallel()

	calls := 0
	root := mustCorrelationRootService(t, func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
		calls++
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency}
	})

	_, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "reuse-failure",
	})
	if !errors.Is(err, providers.ErrExecuteFailed) {
		t.Fatalf("first Execute() error = %v, want ErrExecuteFailed", err)
	}

	if _, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "reuse-failure",
	}); !errors.Is(err, providers.ErrExecuteFailed) {
		t.Fatalf("second Execute() error = %v, want the same typed failure, not a collision", err)
	}
	if calls != 2 {
		t.Fatalf("adapter calls = %d, want 2 (identity must be released after a typed failure)", calls)
	}
}

func TestExecute_ReleasesLiveIdentityAfterContextCancellation(t *testing.T) {
	t.Parallel()

	var calls int32
	started := make(chan struct{})
	root := mustCorrelationRootService(t, func(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(started)
			<-ctx.Done()
			return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled}
		}
		return providers.ExecuteResult{}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(ctx, providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "reuse-cancel",
		})
		firstDone <- err
	}()
	<-started
	cancel()
	if err := <-firstDone; !errors.Is(err, providers.ErrExecuteCancelled) {
		t.Fatalf("first Execute() error = %v, want ErrExecuteCancelled", err)
	}

	if _, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "reuse-cancel",
	}); err != nil {
		t.Fatalf("Execute() after cancellation error = %v, want nil (identity must be released)", err)
	}
}

func TestExecute_ReleasesLiveIdentityAfterPanicUnwind(t *testing.T) {
	t.Parallel()

	calls := 0
	root := mustCorrelationRootService(t, func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
		calls++
		if calls == 1 {
			panic("simulated unexpected terminal unwind")
		}
		return providers.ExecuteResult{}, nil
	})

	func() {
		defer func() { _ = recover() }()
		_, _ = root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "reuse-panic",
		})
	}()

	if _, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "reuse-panic",
	}); err != nil {
		t.Fatalf("Execute() after panic unwind error = %v, want nil (identity must be released)", err)
	}
	if calls != 2 {
		t.Fatalf("adapter calls = %d, want 2", calls)
	}
}
