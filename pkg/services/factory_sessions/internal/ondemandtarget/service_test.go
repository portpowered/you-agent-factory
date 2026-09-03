package ondemandtarget

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
)

// fakeOpener is a minimal InvocationRuntimeOpening test double: it records
// every call and returns a fixed opened runtime/error, letting these tests
// exercise Service without constructing the grouped production dependency
// graph.
type fakeOpener struct {
	calls  []factorysessions.RuntimeOpeningRequest
	opened roles.OpenedInvocationRuntime
	err    error
}

func (f *fakeOpener) OpenInvocationRuntime(
	_ context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedInvocationRuntime, error) {
	f.calls = append(f.calls, *request)
	if f.err != nil {
		return roles.OpenedInvocationRuntime{}, f.err
	}
	return f.opened, nil
}

// fakeLifecycle is a minimal roles.LifecycleRuntime test double: every
// method succeeds as a no-op unless a field is set to force a failure.
type fakeLifecycle struct {
	mu sync.Mutex

	startErr         error
	startWorkerErr   error
	completeErr      error
	stopCalls        int
	waitCalls        int
	stopWorkerCalled bool
	hosted           factoryruntime.RuntimeRecord
}

func (f *fakeLifecycle) StartLifecycle(context.Context, context.Context) error { return f.startErr }
func (f *fakeLifecycle) StartWorkerLifecycle(context.Context) (factorysessions.RuntimeStop, error) {
	if f.startWorkerErr != nil {
		return nil, f.startWorkerErr
	}
	return func(context.Context) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.stopWorkerCalled = true
		return nil
	}, nil
}
func (f *fakeLifecycle) CompleteStartup(context.Context) error { return f.completeErr }
func (f *fakeLifecycle) WaitForRuntime(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.waitCalls++
	return nil
}
func (f *fakeLifecycle) StopLifecycle(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls++
	return nil
}
func (f *fakeLifecycle) FailStartup(err error) error { return err }
func (f *fakeLifecycle) CurrentRuntimeBundle() factoryruntime.RuntimeRecord {
	return f.hosted
}

type fakeHostedInstance struct {
	factoryruntime.RuntimeRecord
	runtime factoryruntime.Service
}

func (f fakeHostedInstance) RuntimeService() factoryruntime.Service { return f.runtime }

type recordingTurnControlRuntime struct {
	factoryruntime.Service

	mu                sync.Mutex
	terminateRequests []factoryruntime.TerminateRequest
	onTerminate       func()
}

func (f *recordingTurnControlRuntime) ControlTerminate(
	_ context.Context,
	request factoryruntime.TerminateRequest,
) (factoryruntime.TerminateResult, error) {
	f.mu.Lock()
	f.terminateRequests = append(f.terminateRequests, request)
	onTerminate := f.onTerminate
	f.mu.Unlock()
	if onTerminate != nil {
		onTerminate()
	}
	return factoryruntime.TerminateResult{
		Outcome: factoryruntime.ControlOutcomeAccepted,
		WorkerSessionControl: factoryruntime.WorkerSessionControlResult{
			TurnID:  request.TurnID,
			Action:  request.WorkerSessionAction,
			Outcome: factoryruntime.WorkerSessionControlAggregateOutcomeApplied,
			Children: []factoryruntime.WorkerSessionControlChildResult{{
				WorkerSessionID: "worker-session-controlled",
				DispatchID:      "dispatch-controlled",
				Outcome:         factoryruntime.WorkerSessionControlChildOutcomeApplied,
			}},
		},
	}, nil
}

func (f *recordingTurnControlRuntime) terminateCalls() []factoryruntime.TerminateRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]factoryruntime.TerminateRequest(nil), f.terminateRequests...)
}

// fakeSessions is a minimal factorysessions.Service test double: it embeds
// the interface unimplemented so any call beyond the target-execution
// operations this package's Service actually uses
// reaches a nil method value and panics, proving no other capability is
// dispatched to.
type fakeSessions struct {
	factorysessions.Service

	mu sync.Mutex

	invokeCalls  []string
	invokeResult factorysessions.InvocationResult
	invokeErr    error
	invoke       func(context.Context, string, factorysessions.InvocationRequest) (factorysessions.InvocationResult, error)

	cancelCalls    []string
	cancelContexts []context.Context
	cancelRequests []factorysessions.ControlRequest
	cancelResult   factorysessions.LifecycleControlResult
	cancelErr      error

	closeCalls []string
	closeErr   error
	closeErrs  []error
	close      func(context.Context, string) error

	factoryEventCalls  []string
	factoryEventStream *factorydefinitions.FactoryEventStream
	factoryEventErr    error

	// getSessionResult is the projection Service reads to decide which
	// orchestrator path an invocation takes. The zero value carries no
	// FactoryConfig, which reads as "not a JavaScript Factory" and so selects
	// the Work-submission path every existing cell already expects.
	getSessionResult factorysessions.SessionProjection
	getSessionErr    error
	getSessionCalls  []string
}

// GetFactorySession is used by Service to choose an invocation's orchestrator
// path: a JavaScript Factory declares no work types and must run as a durable
// workflow rather than through Work submission.
func (f *fakeSessions) GetFactorySession(
	_ context.Context, sessionID string,
) (factorysessions.SessionProjection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getSessionCalls = append(f.getSessionCalls, sessionID)
	return f.getSessionResult, f.getSessionErr
}

func (f *fakeSessions) InvokeFactorySession(
	ctx context.Context, sessionID string, request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	f.mu.Lock()
	f.invokeCalls = append(f.invokeCalls, sessionID)
	invoke := f.invoke
	result := f.invokeResult
	err := f.invokeErr
	f.mu.Unlock()
	if invoke != nil {
		return invoke(ctx, sessionID, request)
	}
	return result, err
}

func (f *fakeSessions) Cancel(
	ctx context.Context, sessionID string, request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelCalls = append(f.cancelCalls, sessionID)
	f.cancelContexts = append(f.cancelContexts, ctx)
	f.cancelRequests = append(f.cancelRequests, request)
	return f.cancelResult, f.cancelErr
}

func (f *fakeSessions) CloseFactorySession(ctx context.Context, sessionID string) error {
	f.mu.Lock()
	f.closeCalls = append(f.closeCalls, sessionID)
	close := f.close
	if len(f.closeErrs) > 0 {
		err := f.closeErrs[0]
		f.closeErrs = f.closeErrs[1:]
		f.mu.Unlock()
		return err
	}
	err := f.closeErr
	f.mu.Unlock()
	if close != nil {
		return close(ctx, sessionID)
	}
	return err
}

func (f *fakeSessions) SubscribeFactoryEventsForSession(
	_ context.Context,
	sessionID string,
	_ *factorydefinitions.FactoryEventReconnectCursor,
) (*factorydefinitions.FactoryEventStream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.factoryEventCalls = append(f.factoryEventCalls, sessionID)
	return f.factoryEventStream, f.factoryEventErr
}

func newTestService(t *testing.T, opener runtimeopening.InvocationRuntimeOpening, resolve RuntimeResolver, generateID factorysessions.SessionIDGenerator) *Service {
	t.Helper()
	return &Service{
		opening:           opener,
		resolve:           resolve,
		generateID:        generateID,
		logger:            zap.NewNop(),
		runtimes:          make(map[string]*activatedRuntime),
		controls:          make(map[string]*activationControl),
		startsByRequestID: make(map[string]string),
		pendingStarts:     make(map[string]*pendingStart),
	}
}

// newTestServiceWithObservedLogger is newTestService, but with a real,
// observable logger (instead of a no-op one) for tests that assert on what
// this Service actually logs.
func newTestServiceWithObservedLogger(t *testing.T, opener runtimeopening.InvocationRuntimeOpening, resolve RuntimeResolver, generateID factorysessions.SessionIDGenerator) (*Service, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(zapcore.DebugLevel)
	return &Service{
		opening:           opener,
		resolve:           resolve,
		generateID:        generateID,
		logger:            zap.New(core),
		runtimes:          make(map[string]*activatedRuntime),
		controls:          make(map[string]*activationControl),
		startsByRequestID: make(map[string]string),
		pendingStarts:     make(map[string]*pendingStart),
	}, observed
}

func sequentialIDs(prefix string) func() string {
	n := 0
	return func() string {
		n++
		return prefix + "-" + string(rune('0'+n))
	}
}

func TestNewRejectsMissingRequiredDependencies(t *testing.T) {
	var factory runtimeopening.InvocationRuntimeOpening = &runtimeopening.Factory{}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	generateID := func() string { return "id" }
	logger := zap.NewNop()

	tests := []struct {
		name       string
		opening    runtimeopening.InvocationRuntimeOpening
		resolve    RuntimeResolver
		generateID factorysessions.SessionIDGenerator
		logger     *zap.Logger
	}{
		{"missing opening", nil, resolve, generateID, logger},
		{"missing resolve", factory, nil, generateID, logger},
		{"missing generateID", factory, resolve, nil, logger},
		{"missing logger", factory, resolve, generateID, nil},
		{"typed nil opening", typedNilInvocationRuntimeOpening(), resolve, generateID, logger},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.opening, tt.resolve, tt.generateID, tt.logger); err == nil {
				t.Fatal("New() error = nil, want a construction error")
			}
		})
	}
}

func typedNilInvocationRuntimeOpening() runtimeopening.InvocationRuntimeOpening {
	var opening *runtimeopening.Factory
	return opening
}

// TestNewUsesInvocationOpeningCapabilityLazily proves construction accepts
// the owner-published invocation opening capability without opening a runtime,
// then opens the selected target exactly once when StartAsync actually runs.
func TestNewUsesInvocationOpeningCapabilityLazily(t *testing.T) {
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{
		Sessions:  &fakeSessions{},
		Lifecycle: &fakeLifecycle{},
	}}
	var opening runtimeopening.InvocationRuntimeOpening = opener
	svc, err := New(
		opening,
		func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
			return factorysessions.RuntimeOpeningRequest{}, nil
		},
		func() string { return "target-runtime" },
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(opener.calls) != 0 {
		t.Fatalf("OpenInvocationRuntime calls after New() = %d, want 0", len(opener.calls))
	}

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{
		Source: factorysessions.Source{FactoryID: "@you/review"},
	})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}
	if started.SessionID != "target-runtime" {
		t.Fatalf("StartAsync() SessionID = %q, want target-runtime", started.SessionID)
	}
	if len(opener.calls) != 1 {
		t.Fatalf("OpenInvocationRuntime calls after StartAsync() = %d, want 1", len(opener.calls))
	}
}

// TestStartAsyncOpensRuntimeAndReturnsGeneratedIdentity proves a successful
// StartAsync resolves the target, opens exactly one runtime, dispatches
// nothing (a caller dispatches this turn's content separately, via
// InvokeFactorySession against the returned identity -- see this method's
// own doc comment), and returns AsyncStartResult carrying this service's own
// generated identity (never the runtime's shared internal constant) and a
// non-terminal RUNNING status.
func TestStartAsyncOpensRuntimeAndReturnsGeneratedIdentity(t *testing.T) {
	sessions := &fakeSessions{}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{
		Sessions:  sessions,
		Lifecycle: &fakeLifecycle{},
	}}
	var resolveCalls []string
	resolve := func(_ context.Context, factoryTargetID, workingRoot string) (factorysessions.RuntimeOpeningRequest, error) {
		resolveCalls = append(resolveCalls, factoryTargetID+"|"+workingRoot)
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	result, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{
		RequestID: "req-1",
		Source:    factorysessions.Source{FactoryID: "@you/review"},
		Args:      map[string]any{"workingRoot": "/work/project"},
	})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}
	if len(resolveCalls) != 1 || resolveCalls[0] != "@you/review|/work/project" {
		t.Fatalf("resolve calls = %v, want exactly one call for @you/review|/work/project", resolveCalls)
	}
	if len(opener.calls) != 1 {
		t.Fatalf("OpenInvocationRuntime call count = %d, want exactly 1", len(opener.calls))
	}
	if len(sessions.invokeCalls) != 0 {
		t.Fatalf("InvokeFactorySession calls = %v, want zero -- StartAsync dispatches nothing", sessions.invokeCalls)
	}
	if result.SessionID != "wrapper-1" {
		t.Fatalf("result.SessionID = %q, want the generated wrapper identity", result.SessionID)
	}
	if result.Status != string(factorysessions.LifecycleStatusRunning) {
		t.Fatalf("result.Status = %q, want RUNNING (a fresh open, no terminal outcome yet)", result.Status)
	}
}

// TestStartAsyncSameRequestIDConvergesOnASingleActivation proves that calling
// StartAsync twice with the exact same non-blank RequestID opens exactly one
// runtime and returns the identical generated SessionID both times, instead
// of opening a second runtime for a repeated request. This is the mechanism
// that makes a caller's retried start -- for example the ACP prompt
// delegation transport's stable per-episode RequestID, retried after its own
// post-start bookkeeping (RecordPendingFactorySession) fails -- safe: the
// retry converges on the exact activation the original call already opened
// rather than leaking a second one.
func TestStartAsyncSameRequestIDConvergesOnASingleActivation(t *testing.T) {
	sessions := &fakeSessions{}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{
		Sessions:  sessions,
		Lifecycle: &fakeLifecycle{},
	}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	req := factorysessions.StartRequest{
		RequestID: "session-1/episode/1",
		Source:    factorysessions.Source{FactoryID: "@you/review"},
		Args:      map[string]any{"workingRoot": "/work/project"},
	}
	first, err := svc.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("first StartAsync() error = %v", err)
	}
	second, err := svc.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("second StartAsync() error = %v", err)
	}

	if len(opener.calls) != 1 {
		t.Fatalf("OpenInvocationRuntime call count = %d, want exactly 1 across both StartAsync calls", len(opener.calls))
	}
	if first.SessionID != second.SessionID {
		t.Fatalf("SessionID[0] = %q, SessionID[1] = %q, want the identical converged identity", first.SessionID, second.SessionID)
	}
	if first.SessionID != "wrapper-1" {
		t.Fatalf("SessionID = %q, want the generated wrapper identity from the one real activation", first.SessionID)
	}
}

// TestStartAsyncBlankRequestIDOpensASeparateRuntimeEveryCall proves that a
// caller who passes no RequestID at all (blank) gets no deduplication --
// every such call opens its own runtime, matching this method's original,
// pre-deduplication behavior for callers that never supply a stable key.
func TestStartAsyncBlankRequestIDOpensASeparateRuntimeEveryCall(t *testing.T) {
	sessions := &fakeSessions{}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{
		Sessions:  sessions,
		Lifecycle: &fakeLifecycle{},
	}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	req := factorysessions.StartRequest{Source: factorysessions.Source{FactoryID: "@you/review"}}
	first, err := svc.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("first StartAsync() error = %v", err)
	}
	second, err := svc.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("second StartAsync() error = %v", err)
	}

	if len(opener.calls) != 2 {
		t.Fatalf("OpenInvocationRuntime call count = %d, want exactly 2 (no dedup key supplied)", len(opener.calls))
	}
	if first.SessionID == second.SessionID {
		t.Fatalf("SessionID[0] = %q, SessionID[1] = %q, want two distinct generated identities", first.SessionID, second.SessionID)
	}
}

// TestStartAsyncDistinctRequestsOpenConcurrently proves the service protects
// only identity allocation and publication. Runtime construction for two
// independent customer sessions must overlap instead of waiting behind a
// service-wide activation lock.
func TestStartAsyncDistinctRequestsOpenConcurrently(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	var callsMu sync.Mutex
	var openedSessionIDs []string
	opener := &funcOpener{open: func(
		_ context.Context,
		request *factorysessions.RuntimeOpeningRequest,
	) (roles.OpenedInvocationRuntime, error) {
		sessionID := request.FactorySession.FactorySessionID
		callsMu.Lock()
		openedSessionIDs = append(openedSessionIDs, sessionID)
		callsMu.Unlock()
		started <- sessionID
		<-release
		return roles.OpenedInvocationRuntime{Sessions: &fakeSessions{}, Lifecycle: &fakeLifecycle{}}, nil
	}}
	svc := newTestService(t, opener, func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}, sequentialIDs("wrapper"))

	results := make(chan factorysessions.AsyncStartResult, 2)
	errs := make(chan error, 2)
	for _, requestID := range []string{"request-a", "request-b"} {
		go func(requestID string) {
			result, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{RequestID: requestID})
			results <- result
			errs <- err
		}(requestID)
	}

	opened := map[string]struct{}{}
	for range 2 {
		select {
		case sessionID := <-started:
			if sessionID == "" {
				t.Fatal("OpenInvocationRuntime received a blank preallocated Factory Session ID")
			}
			opened[sessionID] = struct{}{}
		case <-time.After(5 * time.Second):
			t.Fatal("independent runtime openings did not overlap")
		}
	}
	close(release)

	returned := map[string]struct{}{}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("StartAsync() error = %v", err)
		}
		result := <-results
		returned[result.SessionID] = struct{}{}
	}
	if len(opened) != 2 || len(returned) != 2 {
		t.Fatalf("opened sessions = %v, returned sessions = %v; want two distinct identities", opened, returned)
	}
	for sessionID := range returned {
		if _, ok := opened[sessionID]; !ok {
			t.Fatalf("returned session %q was not the identity used to open its runtime", sessionID)
		}
	}
	callsMu.Lock()
	defer callsMu.Unlock()
	if len(openedSessionIDs) != 2 {
		t.Fatalf("OpenInvocationRuntime calls = %v, want two overlapping calls", openedSessionIDs)
	}
}

// TestStartAsyncPropagatesResolveFailure proves a resolver failure opens no
// runtime.
func TestStartAsyncPropagatesResolveFailure(t *testing.T) {
	opener := &fakeOpener{}
	resolveErr := errors.New("resolve boom")
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, resolveErr
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	_, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if !errors.Is(err, resolveErr) {
		t.Fatalf("StartAsync() error = %v, want %v", err, resolveErr)
	}
	if len(opener.calls) != 0 {
		t.Fatalf("OpenInvocationRuntime call count = %d, want 0 after a resolve failure", len(opener.calls))
	}
}

// TestStartAsyncConcurrentSameRequestIDOpensExactlyOneRuntime proves that
// genuinely concurrent StartAsync calls for the exact same non-blank
// RequestID -- not the sequential-call convergence
// TestStartAsyncSameRequestIDConvergesOnASingleActivation already covers --
// still open exactly one runtime. resolve blocks on a channel the test
// controls, so every goroutine that reaches StartAsync while the first is
// still resolving is forced to either join the in-flight reservation or, if
// it arrives even earlier, race for the reservation itself under s.mu; only
// the one goroutine that wins the reservation ever calls resolve/open.
func TestStartAsyncConcurrentSameRequestIDOpensExactlyOneRuntime(t *testing.T) {
	sessions := &fakeSessions{}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{
		Sessions:  sessions,
		Lifecycle: &fakeLifecycle{},
	}}
	release := make(chan struct{})
	var resolveCalls int32
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		atomic.AddInt32(&resolveCalls, 1)
		<-release
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	req := factorysessions.StartRequest{
		RequestID: "session-1/episode/1",
		Source:    factorysessions.Source{FactoryID: "@you/review"},
	}

	const callers = 8
	results := make([]factorysessions.AsyncStartResult, callers)
	errs := make([]error, callers)
	var start sync.WaitGroup
	start.Add(1)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := range callers {
		go func(i int) {
			defer wg.Done()
			start.Wait()
			results[i], errs[i] = svc.StartAsync(context.Background(), req)
		}(i)
	}
	start.Done()
	// Give the goroutines that don't win the reservation a chance to reach
	// the join-and-wait path before releasing the one real activation.
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("StartAsync()[%d] error = %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&resolveCalls); got != 1 {
		t.Fatalf("resolve call count = %d, want exactly 1 across %d concurrent callers sharing one RequestID", got, callers)
	}
	if len(opener.calls) != 1 {
		t.Fatalf("OpenInvocationRuntime call count = %d, want exactly 1", len(opener.calls))
	}
	first := results[0].SessionID
	if first == "" {
		t.Fatalf("SessionID[0] is blank, want the generated wrapper identity")
	}
	for i, r := range results {
		if r.SessionID != first {
			t.Fatalf("SessionID[%d] = %q, want the identical converged identity %q", i, r.SessionID, first)
		}
	}
}

// TestStartAsyncBlankGeneratedIdentityFailsWithoutPublishing proves that a
// blank value from generateID fails StartAsync before opening a runtime and
// leaves an unrelated prior activation addressable under
// its own identity -- rather than publishing an empty map key that a later
// caller could never look up.
func TestStartAsyncBlankGeneratedIdentityFailsWithoutPublishing(t *testing.T) {
	sessions := &fakeSessions{}
	priorLifecycle := &fakeLifecycle{}
	priorSessions := &fakeSessions{}
	failingLifecycle := &fakeLifecycle{}
	opener := &sequencedOpener{
		results: []openResult{
			{opened: roles.OpenedInvocationRuntime{Sessions: priorSessions, Lifecycle: priorLifecycle}},
			{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: failingLifecycle}},
		},
	}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	ids := []string{"wrapper-prior", ""}
	n := 0
	generateID := func() string {
		id := ids[n]
		n++
		return id
	}
	svc := newTestService(t, opener, resolve, generateID)

	prior, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{
		RequestID: "req-prior",
		Source:    factorysessions.Source{FactoryID: "@you/review"},
	})
	if err != nil {
		t.Fatalf("prior StartAsync() error = %v", err)
	}

	_, err = svc.StartAsync(context.Background(), factorysessions.StartRequest{
		RequestID: "req-blank",
		Source:    factorysessions.Source{FactoryID: "@you/review"},
	})
	if err == nil {
		t.Fatalf("StartAsync() with a blank generated identity error = nil, want an error")
	}
	if failingLifecycle.stopCalls != 0 {
		t.Fatalf("failing activation StopLifecycle calls = %d, want 0 because identity admission precedes opening", failingLifecycle.stopCalls)
	}
	if len(opener.calls) != 1 {
		t.Fatalf("OpenInvocationRuntime calls = %d, want only the prior activation", len(opener.calls))
	}
	if priorLifecycle.stopCalls != 0 {
		t.Fatalf("prior activation StopLifecycle calls = %d, want 0 -- it must remain open", priorLifecycle.stopCalls)
	}

	again, err := svc.InvokeFactorySession(context.Background(), prior.SessionID, factorysessions.InvocationRequest{})
	if err != nil {
		t.Fatalf("InvokeFactorySession() on the prior activation error = %v, want it to remain addressable", err)
	}
	if len(priorSessions.invokeCalls) != 1 {
		t.Fatalf("prior activation invoke calls = %v, want exactly one dispatch to the untouched prior runtime", priorSessions.invokeCalls)
	}
	_ = again
}

// TestStartAsyncCollidingGeneratedIdentityFailsWithoutOverwriting proves that
// a generated wrapper identity colliding with an already-tracked one --
// across two entirely distinct requests, not a retry of the same one --
// fails StartAsync before opening another runtime and leaves the original
// activation's map entry (and therefore ownership) untouched rather than
// letting the second request silently redirect callers of the first
// request's SessionID to the second request's runtime.
func TestStartAsyncCollidingGeneratedIdentityFailsWithoutOverwriting(t *testing.T) {
	firstSessions := &fakeSessions{}
	firstLifecycle := &fakeLifecycle{}
	secondSessions := &fakeSessions{}
	secondLifecycle := &fakeLifecycle{}
	opener := &sequencedOpener{
		results: []openResult{
			{opened: roles.OpenedInvocationRuntime{Sessions: firstSessions, Lifecycle: firstLifecycle}},
			{opened: roles.OpenedInvocationRuntime{Sessions: secondSessions, Lifecycle: secondLifecycle}},
		},
	}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	generateID := func() string { return "wrapper-collide" }
	svc := newTestService(t, opener, resolve, generateID)

	first, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{
		RequestID: "req-first",
		Source:    factorysessions.Source{FactoryID: "@you/review"},
	})
	if err != nil {
		t.Fatalf("first StartAsync() error = %v", err)
	}
	if first.SessionID != "wrapper-collide" {
		t.Fatalf("first.SessionID = %q, want the generated identity", first.SessionID)
	}

	_, err = svc.StartAsync(context.Background(), factorysessions.StartRequest{
		RequestID: "req-second",
		Source:    factorysessions.Source{FactoryID: "@you/review"},
	})
	if err == nil {
		t.Fatalf("second StartAsync() with a colliding generated identity error = nil, want an error")
	}
	if secondLifecycle.stopCalls != 0 {
		t.Fatalf("colliding activation StopLifecycle calls = %d, want 0 because collision admission precedes opening", secondLifecycle.stopCalls)
	}
	if len(opener.calls) != 1 {
		t.Fatalf("OpenInvocationRuntime calls = %d, want only the original activation", len(opener.calls))
	}
	if firstLifecycle.stopCalls != 0 {
		t.Fatalf("original activation StopLifecycle calls = %d, want 0 -- it must remain open", firstLifecycle.stopCalls)
	}

	if _, err := svc.InvokeFactorySession(context.Background(), first.SessionID, factorysessions.InvocationRequest{}); err != nil {
		t.Fatalf("InvokeFactorySession() on the original activation error = %v, want it to still resolve to the original runtime", err)
	}
	if len(firstSessions.invokeCalls) != 1 {
		t.Fatalf("original activation invoke calls = %v, want exactly one dispatch to the untouched original runtime", firstSessions.invokeCalls)
	}
	if len(secondSessions.invokeCalls) != 0 {
		t.Fatalf("colliding activation invoke calls = %v, want zero -- it was never published", secondSessions.invokeCalls)
	}
}

// openResult is one fakeOpener call's canned outcome.
type openResult struct {
	opened roles.OpenedInvocationRuntime
	err    error
}

// sequencedOpener is an invocation runtime-opening test double that returns a
// distinct result for each successive OpenInvocationRuntime call, in order --
// letting a test drive two calls that must return two independently
// closeable runtimes instead of fakeOpener's single fixed result.
type sequencedOpener struct {
	mu      sync.Mutex
	results []openResult
	calls   []factorysessions.RuntimeOpeningRequest
}

func (f *sequencedOpener) OpenInvocationRuntime(
	_ context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedInvocationRuntime, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, *request)
	idx := len(f.calls) - 1
	if idx >= len(f.results) {
		return roles.OpenedInvocationRuntime{}, fmt.Errorf("sequencedOpener: no result configured for call %d", idx)
	}
	return f.results[idx].opened, f.results[idx].err
}

// TestInvokeFactorySessionRoutesByOrchestratorKind proves this activation
// chooses an invocation's path from the opened runtime's own orchestrator
// kind rather than sending every Factory through Work submission.
//
// A JavaScript Factory's whole workflow is its program, so it declares no work
// types. Work submission begins by resolving the single work type carrying
// handlingBehavior DEFAULT, so routing one there fails with "expected exactly
// one work type with handlingBehavior DEFAULT for simplified prompt runs"
// before any Worker runs -- which is what an ACP client saw as a bare
// dependency_unavailable, while `you run --named` on the identical Factory
// worked because the CLI's own invocation operation has always branched here.
//
// Asserting InvokeFactorySession is *not* called is the whole point: a
// JavaScript Factory reaching it at all is the defect.
func TestInvokeFactorySessionRoutesByOrchestratorKind(t *testing.T) {
	javaScriptConfig := &factorydefinitions.FactoryConfig{
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind: factorydefinitions.OrchestratorKindJavaScript,
		},
	}

	tests := []struct {
		name           string
		factoryConfig  *factorydefinitions.FactoryConfig
		getSessionErr  error
		wantWorkSubmit bool
	}{
		{
			name:           "petri factory submits work",
			factoryConfig:  &factorydefinitions.FactoryConfig{},
			wantWorkSubmit: true,
		},
		{
			name:           "javascript factory does not submit work",
			factoryConfig:  javaScriptConfig,
			wantWorkSubmit: false,
		},
		{
			// An unreadable projection must not fail the invocation on its
			// own. It only selects a path, and the Work-submission path
			// reports its own failure precisely.
			name:           "unreadable projection falls back to work submission",
			factoryConfig:  javaScriptConfig,
			getSessionErr:  errors.New("projection unavailable"),
			wantWorkSubmit: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			sessions := &fakeSessions{
				getSessionResult: factorysessions.SessionProjection{
					Context: factorysessions.ProjectionContext{FactoryCfg: testCase.factoryConfig},
				},
				getSessionErr: testCase.getSessionErr,
			}
			opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{
				Sessions:  sessions,
				Lifecycle: &fakeLifecycle{},
			}}
			resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
				return factorysessions.RuntimeOpeningRequest{}, nil
			}
			svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

			started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{
				Source: factorysessions.Source{FactoryID: "@you/spawn"},
			})
			if err != nil {
				t.Fatalf("StartAsync() error = %v", err)
			}
			// The JavaScript path needs an execution runtime this fake does
			// not provide, so it fails; the assertion is on which path ran,
			// not on the outcome.
			_, _ = svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{})

			submitted := len(sessions.invokeCalls) > 0
			if submitted != testCase.wantWorkSubmit {
				t.Fatalf("InvokeFactorySession (Work submission) calls = %v, want submitted = %v",
					sessions.invokeCalls, testCase.wantWorkSubmit)
			}
		})
	}
}
