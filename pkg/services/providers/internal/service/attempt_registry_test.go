package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"

	"go.uber.org/goleak"
)

func TestLiveAttemptRegistry_BindThenContainsThenRelease(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	if registry.contains(key) {
		t.Fatal("contains() = true before bind, want false")
	}
	release, err := registry.bind(key, nil)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	if !registry.contains(key) {
		t.Fatal("contains() = false after bind, want true")
	}
	release()
	if registry.contains(key) {
		t.Fatal("contains() = true after release, want false")
	}
}

func TestLiveAttemptRegistry_BindCollisionRejectsSecondLiveIdentity(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	release, err := registry.bind(key, nil)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	defer release()

	if _, err := registry.bind(key, nil); !errors.Is(err, errAttemptAlreadyLive) {
		t.Fatalf("second bind() error = %v, want errAttemptAlreadyLive", err)
	}
	if !registry.contains(key) {
		t.Fatal("contains() = false after rejected second bind, want the first binding to remain live")
	}
}

func TestLiveAttemptRegistry_DistinctIdentitiesAreIndependentlyLive(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	sameProviderOtherAttempt := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-2"}
	otherProviderSameAttempt := liveAttemptKey{provider: providers.IDClaude, attemptID: "attempt-1"}
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	release, err := registry.bind(key, nil)
	if err != nil {
		t.Fatalf("bind(key) error = %v, want nil", err)
	}
	defer release()

	otherAttemptRelease, err := registry.bind(sameProviderOtherAttempt, nil)
	if err != nil {
		t.Fatalf("bind(same provider, other attempt) error = %v, want nil", err)
	}
	defer otherAttemptRelease()

	otherProviderRelease, err := registry.bind(otherProviderSameAttempt, nil)
	if err != nil {
		t.Fatalf("bind(other provider, same attempt) error = %v, want nil", err)
	}
	defer otherProviderRelease()
}

func TestLiveAttemptRegistry_ReleaseIsIdempotent(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	release, err := registry.bind(key, nil)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	release()
	release()

	if registry.contains(key) {
		t.Fatal("contains() = true after repeated release, want false")
	}

	if _, err := registry.bind(key, nil); err != nil {
		t.Fatalf("rebind after release error = %v, want nil", err)
	}
}

func TestNativeAttemptControl_SupportsCancelAndTerminateOnly(t *testing.T) {
	t.Parallel()

	control := &nativeAttemptControl{cancel: func() {}, done: make(chan struct{})}
	for _, action := range []providers.ControlAction{providers.ControlActionCancel, providers.ControlActionTerminate} {
		if !control.supports(action) {
			t.Fatalf("supports(%q) = false, want true", action)
		}
	}
	if control.supports(providers.ControlActionPause) {
		t.Fatal("supports(pause) = true, want false")
	}
}

func TestLiveAttemptRegistry_ClaimNoOpForUnboundIdentity(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	if _, ok := registry.claim(key, providers.ControlActionCancel); ok {
		t.Fatal("claim() on unbound identity = true, want false")
	}
}

func TestLiveAttemptRegistry_ClaimNoOpWithoutControlHandle(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	release, err := registry.bind(key, nil)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	defer release()

	if _, ok := registry.claim(key, providers.ControlActionCancel); ok {
		t.Fatal("claim() on identity with no control handle = true, want false")
	}
	if !registry.contains(key) {
		t.Fatal("contains() = false after no-op claim, want the identity to remain live")
	}
}

func TestLiveAttemptRegistry_ClaimNoOpForUnsupportedAction(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}
	cancelCalls := 0
	control := &nativeAttemptControl{
		cancel: func() { cancelCalls++ },
		done:   make(chan struct{}),
	}

	release, err := registry.bind(key, control)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	defer release()

	if _, ok := registry.claim(key, providers.ControlActionPause); ok {
		t.Fatal("claim(pause) = true, want false (no per-attempt pause seam)")
	}
	if cancelCalls != 0 {
		t.Fatalf("cancel calls = %d, want 0 for an unsupported action", cancelCalls)
	}
	if !registry.contains(key) {
		t.Fatal("contains() = false after unsupported-action claim, want the identity to remain live")
	}
}

func TestLiveAttemptRegistry_ClaimRemovesEntryAndSignalWaitsForDone(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}
	cancelCalls := 0
	done := make(chan struct{})
	control := &nativeAttemptControl{
		cancel: func() { cancelCalls++ },
		done:   done,
	}

	if _, err := registry.bind(key, control); err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}

	claimed, ok := registry.claim(key, providers.ControlActionCancel)
	if !ok {
		t.Fatal("claim() ok = false, want true for a bound, supported action")
	}
	if registry.contains(key) {
		t.Fatal("contains() = true after claim, want the entry removed")
	}

	// close(done) races the goroutine's signal() call; either order proves the
	// contract: signal() never returns before done is readable, and once it
	// is, signal() returns after invoking cancel exactly once. A dedicated
	// root-level test (TestControlAttempt_BlocksUntilSignaledNativeAttemptReturns)
	// proves the actual blocking-until-terminal-behavior property against a
	// caller-controlled gate, since that cannot be shown deterministically at
	// this unit level without sleep-based timing.
	signalDone := make(chan struct{})
	go func() {
		_, _ = claimed.signal(context.Background())
		close(signalDone)
	}()
	close(done)
	<-signalDone
	if cancelCalls != 1 {
		t.Fatalf("cancel calls = %d, want 1", cancelCalls)
	}
}

func TestLiveAttemptRegistry_ClaimIsExclusiveAmongConcurrentCallers(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}
	closedDone := make(chan struct{})
	close(closedDone)
	control := &nativeAttemptControl{cancel: func() {}, done: closedDone}

	if _, err := registry.bind(key, control); err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}

	const attempts = 8
	var wins int32
	var wg sync.WaitGroup
	wg.Add(attempts)
	for range attempts {
		go func() {
			defer wg.Done()
			if _, ok := registry.claim(key, providers.ControlActionCancel); ok {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()

	if wins != 1 {
		t.Fatalf("concurrent claim() winners = %d, want exactly 1", wins)
	}
}

// TestNativeAttemptControl_ClaimAfterNaturalSuccessButBeforeReleaseReportsUnsupported
// proves the exact adapter-return-before-deregistration interleaving a
// reviewer flagged: Execute can return with a natural success and call
// control.finish(false) before its deferred release() has removed the
// registry entry, leaving a narrow window where a concurrent claim() still
// finds the identity live. Even though claim() wins that race (the identity
// is still registered), signal() must ground its answer in the real recorded
// outcome (finish's cancelled=false) rather than the bare fact that done is
// closed, so the caller sees accepted=false/unsupported instead of a false
// ControlOutcomeCompleted for an attempt that already succeeded on its own.
func TestNativeAttemptControl_ClaimAfterNaturalSuccessButBeforeReleaseReportsUnsupported(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}
	cancelCalls := 0
	control := &nativeAttemptControl{cancel: func() { cancelCalls++ }, done: make(chan struct{})}

	release, err := registry.bind(key, control)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}

	// Model Execute returning naturally: finish(false) already ran (as it
	// would synchronously right after s.execution.Execute returns), but the
	// deferred release() has not yet run, so the registry still reports the
	// identity live.
	control.finish(false)
	if !registry.contains(key) {
		t.Fatal("contains() = false before release(), want the identity still live in the race window")
	}

	claimed, ok := registry.claim(key, providers.ControlActionCancel)
	if !ok {
		t.Fatal("claim() ok = false, want true: the identity is still registered in this window")
	}

	accepted, err := claimed.signal(context.Background())
	if err != nil {
		t.Fatalf("signal() error = %v, want nil", err)
	}
	if accepted {
		t.Fatal("signal() accepted = true, want false: the attempt already succeeded naturally before this claim landed")
	}
	if cancelCalls != 1 {
		t.Fatalf("cancel calls = %d, want 1 (signal must still invoke cancel even when it loses the race)", cancelCalls)
	}

	// The deferred release() the real Execute call would still run afterward
	// must remain a safe no-op.
	release()
	if registry.contains(key) {
		t.Fatal("contains() = true after release(), want the identity removed")
	}
}

// TestLiveAttemptRegistry_ClaimRacingReleaseHasOneDeterministicWinnerAndAlwaysRemoves
// proves the natural-completion-versus-control race story 004 requires:
// registry.claim (a control call) and the release closure returned by bind
// (a natural Execute completion) share one mutex, so a concurrent pair of
// them can never both observe the identity as live, and the identity is
// always gone afterward either way. Run repeatedly (no sleep) so both
// possible lock-acquisition orderings actually occur across iterations.
func TestLiveAttemptRegistry_ClaimRacingReleaseHasOneDeterministicWinnerAndAlwaysRemoves(t *testing.T) {
	t.Parallel()

	for i := range 200 {
		registry := newLiveAttemptRegistry()
		key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}
		closedDone := make(chan struct{})
		close(closedDone)
		control := &nativeAttemptControl{cancel: func() {}, done: closedDone}

		release, err := registry.bind(key, control)
		if err != nil {
			t.Fatalf("iteration %d: bind() error = %v, want nil", i, err)
		}

		var claimed int32
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, ok := registry.claim(key, providers.ControlActionCancel); ok {
				atomic.AddInt32(&claimed, 1)
			}
		}()
		go func() {
			defer wg.Done()
			release()
		}()
		wg.Wait()

		if claimed > 1 {
			t.Fatalf("iteration %d: claim() succeeded more than once racing a concurrent release()", i)
		}
		if registry.contains(key) {
			t.Fatalf("iteration %d: contains() = true after a claim/release race, want the identity removed either way", i)
		}
	}
}

type ownedProcessControlFunc func(context.Context) (bool, error)

func (kill ownedProcessControlFunc) ForceKill(ctx context.Context) (bool, error) {
	return kill(ctx)
}

type killCatalogStub struct{ catalog.Service }

func (killCatalogStub) ResolveProviderID(id providers.ID) (providers.ID, error) { return id, nil }

type killACPStub struct{ acp.ContinuationService }

func (killACPStub) Resolve(providers.ID) (providers.ID, bool) { return "", false }

type killExecutionStub struct {
	execution.ContinuationService
	attempt execution.Attempt
}

func (stub killExecutionStub) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	return stub.attempt(ctx, request)
}

func (stub killExecutionStub) Continue(ctx context.Context, request execution.ContinuationRequest) (providers.ExecuteResult, error) {
	return stub.attempt(ctx, request.ExecuteRequest)
}

func TestProvidersNativeKillCapabilityRequiresSupportedAdapterAndExactOwnedHandle(t *testing.T) {
	t.Parallel()
	for _, id := range []providers.ID{providers.IDCodex, providers.IDClaude, providers.IDAntigravity} {
		for _, continued := range []bool{false, true} {
			t.Run(string(id)+fmt.Sprint("/continued=", continued), func(t *testing.T) {
				t.Parallel()
				started, stopped := make(chan struct{}), make(chan struct{})
				var stop sync.Once
				defer stop.Do(func() { close(stopped) })
				var signals atomic.Int32
				effect := ownedProcessControlFunc(func(context.Context) (bool, error) {
					signals.Add(1)
					stop.Do(func() { close(stopped) })
					return true, nil
				})
				adapter := killExecutionStub{attempt: func(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
					request.OwnedProcessObserver(effect)
					close(started)
					select {
					case <-stopped:
						return providers.ExecuteResult{}, nil
					case <-ctx.Done():
						return providers.ExecuteResult{}, ctx.Err()
					}
				}}
				service := &Service{catalog: killCatalogStub{}, acp: killACPStub{}, execution: adapter,
					attempts: newLiveAttemptRegistry(), logger: logging.NoopLogger{}}
				request := providers.ExecuteRequest{Provider: id, AttemptID: "physical"}
				executed := make(chan error, 1)
				go func() {
					var err error
					if continued {
						_, err = service.dispatchContinuation(t.Context(), request, providers.SessionRef{})
					} else {
						_, err = service.Execute(t.Context(), request)
					}
					executed <- err
				}()
				<-started
				result, err := service.ControlAttempt(t.Context(), providers.ControlAttemptRequest{
					Provider: id, AttemptID: request.AttemptID, Action: providers.ControlActionKill,
				})
				want := providers.ControlOutcomeCompleted
				if id == providers.IDAntigravity {
					want = providers.ControlOutcomeUnsupported
					if signals.Load() != 0 {
						t.Error("unsupported adapter signaled a process")
					}
					stop.Do(func() { close(stopped) })
				}
				if err != nil || result.Outcome != want || result.Action != providers.ControlActionKill {
					t.Errorf("control = %#v, %v; want %s", result, err, want)
				}
				if err := <-executed; err != nil {
					t.Fatalf("execution error = %v", err)
				}
			})
		}
	}
}

func TestNativeAttemptControl_KillRequiresAttachedLiveCapability(t *testing.T) {
	t.Parallel()
	for _, allowed := range []bool{false, true} {
		control := &nativeAttemptControl{allowKill: allowed, cancel: func() {
			t.Error("capability admission canceled the attempt")
		}, done: make(chan struct{})}
		if control.supports(providers.ControlActionKill) {
			t.Fatal("unattached attempt supports kill")
		}
		control.attachProcess(ownedProcessControlFunc(func(context.Context) (bool, error) {
			t.Error("capability admission signaled the process")
			return true, nil
		}))
		if control.supports(providers.ControlActionKill) != allowed {
			t.Fatalf("attached kill support does not match provider capability %v", allowed)
		}
		control.finish(false)
		control.attachProcess(ownedProcessControlFunc(func(context.Context) (bool, error) {
			t.Error("late capability signaled a completed attempt")
			return true, nil
		}))
		if control.supports(providers.ControlActionKill) {
			t.Fatal("completed attempt regained kill authority")
		}
	}
}

func TestProviderKillRequestRetainsExactAttemptValidation(t *testing.T) {
	t.Parallel()
	request := providers.ControlAttemptRequest{Provider: providers.IDCodex, AttemptID: "physical-attempt", Action: providers.ControlActionKill}
	if err := request.Validate(); err != nil {
		t.Fatalf("kill request invalid: %v", err)
	}
	request.AttemptID = " "
	if err := request.Validate(); !errors.Is(err, providers.ErrInvalidControlRequest) {
		t.Fatalf("missing exact attempt accepted: %v", err)
	}
}

func TestNativeAttemptControl_KillJoinsTreeAndAuthoritativeAttempt(t *testing.T) {
	t.Parallel()
	control := &nativeAttemptControl{allowKill: true, cancel: func() {
		t.Error("kill used ordinary cancellation")
	}, done: make(chan struct{})}
	signalStarted := make(chan struct{})
	treeJoined := make(chan struct{})
	control.attachProcess(ownedProcessControlFunc(func(ctx context.Context) (bool, error) {
		close(signalStarted)
		select {
		case <-treeJoined:
			return true, nil
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}))
	if !control.supports(providers.ControlActionKill) {
		t.Fatal("attached attempt does not support kill")
	}
	result := make(chan bool, 1)
	go func() {
		accepted, err := control.signal(t.Context())
		if err != nil {
			t.Errorf("kill error = %v", err)
		}
		result <- accepted
	}()
	<-signalStarted
	control.finish(false)
	select {
	case <-result:
		t.Fatal("provider completion alone reported kill completion")
	default:
	}
	close(treeJoined)
	if !<-result {
		t.Fatal("confirmed tree and provider join did not complete kill")
	}
}

func TestNativeAttemptControl_KillNeverInventsJoinOrFallsBackToCancel(t *testing.T) {
	t.Parallel()
	killError := errors.New("partial tree cleanup")
	for _, test := range []struct {
		name     string
		accepted bool
		killErr  error
		joined   bool
		want     bool
		wantErr  error
	}{
		{name: "expired", joined: true},
		{name: "partial", killErr: killError, joined: true, wantErr: killError},
		{name: "unconfirmed provider join", accepted: true, wantErr: context.Canceled},
		{name: "both joined", accepted: true, joined: true, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			control := &nativeAttemptControl{allowKill: true, done: make(chan struct{}), cancel: func() {
				t.Error("force fell back to cancellation")
			}}
			control.attachProcess(ownedProcessControlFunc(func(context.Context) (bool, error) {
				if test.joined {
					control.finish(false)
				} else {
					cancel()
				}
				return test.accepted, test.killErr
			}))
			if !control.supports(providers.ControlActionKill) {
				t.Fatal("missing attached capability")
			}
			accepted, err := control.signal(ctx)
			if accepted != test.want || !errors.Is(err, test.wantErr) {
				t.Fatalf("signal = %v, %v; want %v, %v", accepted, err, test.want, test.wantErr)
			}
			if test.killErr != nil && !errors.Is(err, providers.ErrControlSignalFailed) {
				t.Fatalf("kill error lacks signal failure identity: %v", err)
			}
		})
	}
}

func TestLiveAttemptRegistry_KillPinsIdentityAndNeverRetargetsCapability(t *testing.T) {
	t.Parallel()
	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "owned"}
	control := &nativeAttemptControl{allowKill: true, cancel: func() {}, done: make(chan struct{})}
	var calls int
	control.attachProcess(ownedProcessControlFunc(func(context.Context) (bool, error) {
		calls++
		control.finish(false)
		return true, nil
	}))
	release, err := registry.bind(key, control)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, ok := registry.claim(liveAttemptKey{provider: providers.IDClaude, attemptID: key.attemptID}, providers.ControlActionKill); ok {
		t.Fatal("wrong provider acquired kill authority")
	}
	claimed, ok := registry.claim(key, providers.ControlActionKill)
	if !ok {
		t.Fatal("exact attempt did not acquire kill authority")
	}
	if _, ok := registry.claim(key, providers.ControlActionKill); ok {
		t.Fatal("duplicate control acquired kill authority")
	}
	if _, err := registry.bind(key, nil); !errors.Is(err, errAttemptAlreadyLive) {
		t.Fatalf("replacement admitted during join: %v", err)
	}
	control.attachProcess(ownedProcessControlFunc(func(context.Context) (bool, error) {
		t.Error("claimed control retargeted a second capability")
		return true, nil
	}))
	accepted, err := claimed.signal(t.Context())
	if err != nil || !accepted || calls != 1 {
		t.Fatalf("kill = %v, %v; effects = %d", accepted, err, calls)
	}
	release()
	releaseReplacement, err := registry.bind(key, nil)
	if err != nil {
		t.Fatalf("replacement refused after authoritative release: %v", err)
	}
	defer releaseReplacement()
	release()
	if !registry.contains(key) {
		t.Fatal("old release erased replacement registration")
	}
}

func TestProviderDeclinedKillPreservesOrdinaryCancel(t *testing.T) {
	t.Parallel()
	registry := newLiveAttemptRegistry()
	service := &Service{attempts: registry, logger: logging.NoopLogger{}}
	var kills, cancellations atomic.Int32
	control := &nativeAttemptControl{allowKill: true, done: make(chan struct{})}
	control.cancel = func() {
		cancellations.Add(1)
		control.finish(true)
	}
	control.attachProcess(ownedProcessControlFunc(func(context.Context) (bool, error) {
		kills.Add(1)
		return false, nil // The runner expired ownership before the claim arrived.
	}))
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "expired-process"}
	release, err := registry.bind(key, control)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	request := providers.ControlAttemptRequest{Provider: key.provider, AttemptID: key.attemptID, Action: providers.ControlActionKill}
	for range 2 {
		result, err := service.ControlAttempt(t.Context(), request)
		if err != nil || result.Outcome != providers.ControlOutcomeUnsupported {
			t.Fatalf("expired kill = %#v, %v", result, err)
		}
	}
	control.attachProcess(ownedProcessControlFunc(func(context.Context) (bool, error) {
		t.Error("expired capability was replaced")
		return true, nil
	}))
	if control.supports(providers.ControlActionKill) || kills.Load() != 1 || cancellations.Load() != 0 {
		t.Fatalf("unsupported kill changed execution or retained authority: kills=%d cancels=%d", kills.Load(), cancellations.Load())
	}
	request.Action = providers.ControlActionCancel
	result, err := service.ControlAttempt(t.Context(), request)
	if err != nil || result.Outcome != providers.ControlOutcomeCompleted || cancellations.Load() != 1 {
		t.Fatalf("ordinary cancel after declined kill = %#v, %v; cancels=%d", result, err, cancellations.Load())
	}
}

func TestProviderCanceledKillObservationDoesNotConsumeControl(t *testing.T) {
	t.Parallel()
	registry := newLiveAttemptRegistry()
	service := &Service{attempts: registry, logger: logging.NoopLogger{}}
	control := &nativeAttemptControl{allowKill: true, cancel: func() {}, done: make(chan struct{})}
	control.attachProcess(ownedProcessControlFunc(func(context.Context) (bool, error) {
		t.Error("already canceled observer signaled the process")
		return true, nil
	}))
	key := liveAttemptKey{provider: providers.IDClaude, attemptID: "still-live"}
	release, err := registry.bind(key, control)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = service.ControlAttempt(ctx, providers.ControlAttemptRequest{Provider: key.provider, AttemptID: key.attemptID, Action: providers.ControlActionKill})
	if !errors.Is(err, context.Canceled) || !registry.contains(key) {
		t.Fatalf("canceled observation consumed authority: %v", err)
	}
	if _, ok := registry.claim(key, providers.ControlActionCancel); !ok {
		t.Fatal("canceled force observation blocked ordinary cancel")
	}
}

func TestDeclinedKillCannotRestoreReplacementControl(t *testing.T) {
	t.Parallel()
	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "reused"}
	old := &nativeAttemptControl{allowKill: true, cancel: func() {}, done: make(chan struct{})}
	old.attachProcess(ownedProcessControlFunc(func(context.Context) (bool, error) { return false, nil }))
	release, err := registry.bind(key, old)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.claim(key, providers.ControlActionKill); !ok {
		t.Fatal("old control was not claimed")
	}
	release()
	replacement := &nativeAttemptControl{cancel: func() {}, done: make(chan struct{})}
	releaseReplacement, err := registry.bind(key, replacement)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseReplacement()
	if _, ok := registry.claim(key, providers.ControlActionCancel); !ok {
		t.Fatal("replacement was not claimed")
	}
	registry.restoreDeclinedKill(key, old)
	if _, ok := registry.claim(key, providers.ControlActionCancel); ok {
		t.Fatal("old declined kill reopened replacement authority")
	}
}

func TestProviderKillStopsOnlyTheSelectedLiveAttempt(t *testing.T) {
	t.Parallel()
	// Gate cleanup joins the live sibling before ending its execution context.
	// testing cancels t.Context before Cleanup, which would obscure isolation.
	attemptCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	type attemptGate struct {
		started, stopped chan struct{}
		done             chan error
		signals          atomic.Int32
		stop             sync.Once
	}
	gates := map[string]*attemptGate{}
	for _, id := range []string{"target", "sibling"} {
		gates[id] = &attemptGate{started: make(chan struct{}), stopped: make(chan struct{}), done: make(chan error, 1)}
	}
	adapter := killExecutionStub{attempt: func(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
		gate := gates[request.AttemptID]
		request.OwnedProcessObserver(ownedProcessControlFunc(func(context.Context) (bool, error) {
			gate.signals.Add(1)
			gate.stop.Do(func() { close(gate.stopped) })
			return true, nil
		}))
		close(gate.started)
		select {
		case <-gate.stopped:
			return providers.ExecuteResult{}, nil
		case <-ctx.Done():
			return providers.ExecuteResult{}, ctx.Err()
		}
	}}
	service := &Service{catalog: killCatalogStub{}, acp: killACPStub{}, execution: adapter,
		attempts: newLiveAttemptRegistry(), logger: logging.NoopLogger{}}
	for id, gate := range gates {
		t.Cleanup(func() {
			gate.stop.Do(func() { close(gate.stopped) })
			if err := <-gate.done; err != nil {
				t.Errorf("attempt %s execution = %v", id, err)
			}
		})
		go func() {
			_, err := service.Execute(attemptCtx, providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: id})
			gate.done <- err
		}()
		<-gate.started
	}
	request := providers.ControlAttemptRequest{Provider: providers.IDCodex, AttemptID: "stale", Action: providers.ControlActionKill}
	result, err := service.ControlAttempt(t.Context(), request)
	if err != nil || result.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("stale kill = %#v, %v", result, err)
	}
	request.AttemptID = "target"
	result, err = service.ControlAttempt(t.Context(), request)
	if err != nil || result.Outcome != providers.ControlOutcomeCompleted || result.AttemptID != "target" {
		t.Fatalf("target kill = %#v, %v", result, err)
	}
	if gates["target"].signals.Load() != 1 || gates["sibling"].signals.Load() != 0 {
		t.Fatal("kill did not signal exactly the selected attempt")
	}
	select {
	case <-gates["sibling"].done:
		t.Fatal("target kill stopped the sibling execution")
	default:
	}
}

// TestMain fails the package when a test leaves goroutines running, which
// otherwise surfaces as teardown hangs and cross-test interference.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
