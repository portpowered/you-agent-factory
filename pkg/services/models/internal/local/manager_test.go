package local

import (
	"context"
	"errors"
	"testing"
	"time"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestResourceLimiterScopeCapacityAndIdempotentRelease(t *testing.T) {
	t.Parallel()
	limiter, err := NewResourceLimiter(Hooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	scopeA := resourceTestScope(t, "factory-session:capacity:a")
	scopeB := resourceTestScope(t, "factory-session:capacity:b")
	configA, workerA := resourceTestConfiguration(1)
	configB, workerB := resourceTestConfiguration(2)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	releaseA := acquireTestResources(t, limiter, ctx, scopeA, configA, workerA)
	defer releaseA()
	// Identical model/backend/load-policy names do not share capacity or the
	// lower capacity chosen by A.
	releaseB := acquireTestResources(t, limiter, ctx, scopeB, configB, workerB)
	defer releaseB()
	assertResourceCapacityFull(t, limiter, scopeA, configA, workerA)
	assertResourceCapacityFull(t, limiter, scopeB, configB, workerB)

	releaseA()
	releaseNextA := acquireTestResources(t, limiter, ctx, scopeA, configA, workerA)
	defer releaseNextA()
	releaseA() // Must not release the newer reservation.
	assertResourceCapacityFull(t, limiter, scopeA, configA, workerA)
	assertResourceCapacityFull(t, limiter, scopeB, configB, workerB)

	releaseNextA()
	releaseB()
	for range 3 {
		release := acquireTestResources(t, limiter, ctx, scopeB, configB, workerB)
		release()
	}
}

func TestResourceLimiterScopeCancellationRollsBackPartialReservations(t *testing.T) {
	t.Parallel()
	var outcomes []bool
	limiter, err := NewResourceLimiter(Hooks{
		MarkResourceWaitFinished: func(_ context.Context, _ time.Time, success bool) {
			outcomes = append(outcomes, success)
		},
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	scope := resourceTestScope(t, "factory-session:capacity:rollback")
	config, worker := resourceTestConfiguration(1)
	second := config.Resources[0]
	second.Name, second.Model = "second", "SECOND_MODEL"
	config.Resources = append(config.Resources, second)
	worker.Resources = append(worker.Resources, apisurface.RuntimeResource{Name: second.Name, Capacity: 1})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	secondWorker := *worker
	secondWorker.Resources = secondWorker.Resources[1:]
	releaseSecond := acquireTestResources(t, limiter, ctx, scope, config, &secondWorker)
	defer releaseSecond()
	canceled, cancelWait := context.WithCancel(ctx)
	cancelWait()
	// The first resource is free; the second is held. The cancelled wait must
	// roll back the first reservation before returning.
	release, err := limiter.Acquire(canceled, scope, config, worker)
	if !errors.Is(err, context.Canceled) || release != nil {
		t.Fatalf("partial acquisition = %v, has release=%t; want cancellation without release", err, release != nil)
	}
	firstWorker := *worker
	firstWorker.Resources = firstWorker.Resources[:1]
	releaseFirst := acquireTestResources(t, limiter, ctx, scope, config, &firstWorker)
	defer releaseFirst()
	assertResourceCapacityFull(t, limiter, scope, config, &secondWorker)
	releaseFirst()
	releaseSecond()
	release = acquireTestResources(t, limiter, ctx, scope, config, worker)
	release()
	if len(outcomes) != 5 || outcomes[0] != true || outcomes[1] != false ||
		outcomes[2] != true || outcomes[3] != false || outcomes[4] != true {
		t.Fatalf("resource wait outcomes = %v, want [true false true false true]", outcomes)
	}
}

func TestResourceLimiterScopeWaitingAcquisitionResumesAfterRelease(t *testing.T) {
	t.Parallel()
	started := make(chan struct{}, 1)
	limiter, err := NewResourceLimiter(Hooks{
		MarkResourceWaitStarted: func(_ context.Context, _ time.Time) { started <- struct{}{} },
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	scope := resourceTestScope(t, "factory-session:capacity:waiting")
	config, worker := resourceTestConfiguration(1)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	release := acquireTestResources(t, limiter, ctx, scope, config, worker)
	defer release()
	<-started
	type acquisition struct {
		release func()
		err     error
	}
	completed := make(chan acquisition, 1)
	go func() {
		release, err := limiter.Acquire(ctx, scope, config, worker)
		completed <- acquisition{release: release, err: err}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("second acquisition did not start")
	}
	release()
	select {
	case result := <-completed:
		if result.err != nil || result.release == nil {
			t.Fatalf("waiting acquisition = %v, has release=%t", result.err, result.release != nil)
		}
		result.release()
	case <-ctx.Done():
		t.Fatal("release did not resume waiting acquisition")
	}
}

func TestResourceLimiterCloseScopeRejectsWaiterAndLateAcquirePreservesPeer(t *testing.T) {
	t.Parallel()
	started := make(chan struct{}, 2)
	limiter, err := NewResourceLimiter(Hooks{
		MarkResourceWaitStarted: func(_ context.Context, _ time.Time) { started <- struct{}{} },
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	scopeA := resourceTestScope(t, "factory-session:capacity:close-a")
	scopeB := resourceTestScope(t, "factory-session:capacity:close-b")
	config, worker := resourceTestConfiguration(1)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	releaseA := acquireTestResources(t, limiter, ctx, scopeA, config, worker)
	defer releaseA()
	<-started
	finished := make(chan error, 1)
	go func() {
		release, err := limiter.Acquire(ctx, scopeA, config, worker)
		if release != nil {
			release()
		}
		finished <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("waiting acquisition did not start")
	}
	limiter.CloseScope(scopeA)
	select {
	case err := <-finished:
		if !errors.Is(err, apisurface.ErrRuntimeScopeClosed) {
			t.Fatalf("closed waiter = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("close did not wake waiting acquisition")
	}
	// A late, repeated release must not recreate scope capacity.
	releaseA()
	releaseA()
	if release, err := limiter.Acquire(ctx, scopeA, config, worker); release != nil || !errors.Is(err, apisurface.ErrRuntimeScopeClosed) {
		t.Fatalf("closed acquisition = %v, has release=%t", err, release != nil)
	}
	releaseB := acquireTestResources(t, limiter, ctx, scopeB, config, worker)
	<-started
	releaseB()
	limiter.CloseScope(scopeA)
	limiter.Close()
	limiter.Close()
	if release, err := limiter.Acquire(ctx, scopeB, config, worker); release != nil || !errors.Is(err, apisurface.ErrRuntimeScopeClosed) {
		t.Fatalf("shutdown acquisition = %v, has release=%t", err, release != nil)
	}
}

func TestResourceLimiterCloseBeforeResolutionRejectsLateAcquisition(t *testing.T) {
	t.Parallel()
	limiter, err := NewResourceLimiter(Hooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	scope := resourceTestScope(t, "factory-session:capacity:close-race")
	config, worker := resourceTestConfiguration(1)
	// The injected observation closes the scope before entry resolution. No
	// sleeps or scheduler timing determine which side wins this race.
	limiter.hooks.MarkResourceWaitStarted = func(_ context.Context, _ time.Time) { limiter.CloseScope(scope) }
	if release, err := limiter.Acquire(t.Context(), scope, config, worker); release != nil || !errors.Is(err, apisurface.ErrRuntimeScopeClosed) {
		t.Fatalf("close-winning acquisition = %v, has release=%t", err, release != nil)
	}
}

func resourceTestScope(t *testing.T, value string) apisurface.RuntimeScopeRef {
	t.Helper()
	scope, err := (apisurface.RuntimeScopeRef{}).Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func resourceTestConfiguration(capacity int) (*apisurface.RuntimeConfig, *apisurface.RuntimeWorker) {
	return &apisurface.RuntimeConfig{Resources: []apisurface.RuntimeResource{{
			Name: "model", Type: apisurface.RuntimeResourceTypeModel, Model: "OMNIVOICE_Q4_K_M",
			Backend: "LLAMACPP", LoadPolicy: "ON_DEMAND", Capacity: capacity,
		}}}, &apisurface.RuntimeWorker{
			ModelLocality: apisurface.RuntimeModelLocalityLocal,
			Resources:     []apisurface.RuntimeResource{{Name: "model", Capacity: capacity}},
		}
}

func acquireTestResources(t *testing.T, limiter *ResourceLimiter, ctx context.Context,
	scope apisurface.RuntimeScopeRef, config *apisurface.RuntimeConfig, worker *apisurface.RuntimeWorker,
) func() {
	t.Helper()
	release, err := limiter.Acquire(ctx, scope, config, worker)
	if err != nil || release == nil {
		t.Fatalf("Acquire(%s) = %v, has release=%t; want reservation", scope.String(), err, release != nil)
	}
	return release
}

func assertResourceCapacityFull(t *testing.T, limiter *ResourceLimiter, scope apisurface.RuntimeScopeRef,
	config *apisurface.RuntimeConfig, worker *apisurface.RuntimeWorker,
) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if release, err := limiter.Acquire(ctx, scope, config, worker); !errors.Is(err, context.Canceled) || release != nil {
		if release != nil {
			release()
		}
		t.Fatalf("full capacity acquisition = %v, has release=%t; want cancellation", err, release != nil)
	}
}
