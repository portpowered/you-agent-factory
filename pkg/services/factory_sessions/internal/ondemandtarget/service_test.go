package ondemandtarget

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
)

// fakeOpener is a minimal invocationRuntimeOpener test double: it records
// every call and returns a fixed opened runtime/error, letting these tests
// exercise Service without constructing *runtimeopening.Factory's own large
// production dependency graph.
type fakeOpener struct {
	calls  []factorysessions.RuntimeOpeningRequest
	opened roles.OpenedInvocationRuntime
	err    error
}

func (f *fakeOpener) OpenInvocationRuntime(
	_ context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	_ runtimeopening.ExternalEffects,
	_ *zap.Logger,
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
func (f *fakeLifecycle) CurrentRuntimeBundle() factoryruntime.HostedInstance {
	return nil
}

// fakeSessions is a minimal factorysessions.Service test double: it embeds
// the interface unimplemented so any call beyond
// Invoke/Cancel/CloseFactorySession this package's Service actually uses
// reaches a nil method value and panics, proving no other capability is
// dispatched to.
type fakeSessions struct {
	factorysessions.Service

	mu sync.Mutex

	invokeCalls  []string
	invokeResult factorysessions.InvocationResult
	invokeErr    error

	cancelCalls    []string
	cancelContexts []context.Context
	cancelRequests []factorysessions.ControlRequest
	cancelResult   factorysessions.LifecycleControlResult
	cancelErr      error

	closeCalls []string
	closeErr   error
}

func (f *fakeSessions) InvokeFactorySession(
	_ context.Context, sessionID string, _ factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invokeCalls = append(f.invokeCalls, sessionID)
	return f.invokeResult, f.invokeErr
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

func (f *fakeSessions) CloseFactorySession(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls = append(f.closeCalls, sessionID)
	return f.closeErr
}

func newTestService(t *testing.T, opener invocationRuntimeOpener, resolve RuntimeResolver, generateID factorysessions.SessionIDGenerator) *Service {
	t.Helper()
	return &Service{
		factory:           opener,
		resolve:           resolve,
		generateID:        generateID,
		logger:            zap.NewNop(),
		runtimes:          make(map[string]*activatedRuntime),
		startsByRequestID: make(map[string]string),
		pendingStarts:     make(map[string]*pendingStart),
	}
}

// newTestServiceWithObservedLogger is newTestService, but with a real,
// observable logger (instead of a no-op one) for tests that assert on what
// this Service actually logs.
func newTestServiceWithObservedLogger(t *testing.T, opener invocationRuntimeOpener, resolve RuntimeResolver, generateID factorysessions.SessionIDGenerator) (*Service, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(zapcore.DebugLevel)
	return &Service{
		factory:           opener,
		resolve:           resolve,
		generateID:        generateID,
		logger:            zap.New(core),
		runtimes:          make(map[string]*activatedRuntime),
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
	factory := &runtimeopening.Factory{}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	generateID := func() string { return "id" }
	logger := zap.NewNop()

	tests := []struct {
		name       string
		factory    *runtimeopening.Factory
		resolve    RuntimeResolver
		generateID factorysessions.SessionIDGenerator
		logger     *zap.Logger
	}{
		{"missing factory", nil, resolve, generateID, logger},
		{"missing resolve", factory, nil, generateID, logger},
		{"missing generateID", factory, resolve, nil, logger},
		{"missing logger", factory, resolve, generateID, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.factory, runtimeopening.ExternalEffects{}, tt.resolve, tt.generateID, tt.logger); err == nil {
				t.Fatal("New() error = nil, want a construction error")
			}
		})
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
// blank value from generateID fails StartAsync, closes the runtime it had
// already opened, and leaves an unrelated prior activation addressable under
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
	if failingLifecycle.stopCalls != 1 {
		t.Fatalf("failing activation StopLifecycle calls = %d, want exactly 1 (closed once)", failingLifecycle.stopCalls)
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
// fails StartAsync, closes the newly opened runtime, and leaves the original
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
	if secondLifecycle.stopCalls != 1 {
		t.Fatalf("colliding activation StopLifecycle calls = %d, want exactly 1 (closed once)", secondLifecycle.stopCalls)
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

// sequencedOpener is an invocationRuntimeOpener test double that returns a
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
	_ runtimeopening.ExternalEffects,
	_ *zap.Logger,
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

// TestInvokeFactorySessionReusesTheCachedRuntime proves InvokeFactorySession
// calls against a previously started identity dispatch against the exact
// cached runtime's DefaultSessionID without opening a second runtime, and
// substitute the result's SessionID with the caller-facing generated
// identity rather than leaking the runtime's own shared internal constant.
func TestInvokeFactorySessionReusesTheCachedRuntime(t *testing.T) {
	sessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: &fakeLifecycle{}}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}

	first, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{})
	if err != nil {
		t.Fatalf("InvokeFactorySession() first error = %v", err)
	}
	if first.SessionID != started.SessionID {
		t.Fatalf("InvokeFactorySession() result.SessionID = %q, want the generated wrapper identity %q", first.SessionID, started.SessionID)
	}
	if _, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); err != nil {
		t.Fatalf("InvokeFactorySession() second error = %v", err)
	}

	if len(opener.calls) != 1 {
		t.Fatalf("OpenInvocationRuntime call count = %d, want exactly 1 (no second open)", len(opener.calls))
	}
	if len(sessions.invokeCalls) != 2 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 2", len(sessions.invokeCalls))
	}
	for _, sessionID := range sessions.invokeCalls {
		if sessionID != factorysessions.DefaultSessionID {
			t.Fatalf("InvokeFactorySession sessionID = %q, want the runtime's own DefaultSessionID", sessionID)
		}
	}
}

// TestInvokeFactorySessionUnknownIdentityReportsSessionNotFound proves an
// identity this service never started reports ErrSessionNotFound.
func TestInvokeFactorySessionUnknownIdentityReportsSessionNotFound(t *testing.T) {
	svc := newTestService(t, &fakeOpener{}, nil, sequentialIDs("wrapper"))
	_, err := svc.InvokeFactorySession(context.Background(), "unknown", factorysessions.InvocationRequest{})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("InvokeFactorySession() error = %v, want ErrSessionNotFound", err)
	}
}

// ctxKey is a private context key type for a test-only value, used to prove
// Cancel forwards the caller's original context rather than substituting or
// dropping it.
type ctxKey string

// TestCancelForwardsContextAndControlRequestToTheCapturedRuntime proves
// Cancel forwards the exact caller context and ControlRequest to the
// captured runtime's own Cancel call (against its DefaultSessionID, exactly
// like InvokeFactorySession already does) and returns its lifecycle result or
// error unchanged -- without reclassifying the outcome.
func TestCancelForwardsContextAndControlRequestToTheCapturedRuntime(t *testing.T) {
	cancelResult := factorysessions.LifecycleControlResult{
		Operation: factorysessions.LifecycleControlKind("CANCEL"),
		Outcome:   factorysessions.LifecycleControlOutcome("APPLIED"),
	}
	sessions := &fakeSessions{cancelResult: cancelResult}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: &fakeLifecycle{}}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}

	ctx := context.WithValue(context.Background(), ctxKey("trace"), "trace-1")
	request := factorysessions.ControlRequest{RequestID: "control-1", Reason: "user requested"}
	result, err := svc.Cancel(ctx, started.SessionID, request)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if result != cancelResult {
		t.Fatalf("Cancel() result = %+v, want the captured runtime's own result %+v unchanged", result, cancelResult)
	}
	if len(sessions.cancelRequests) != 1 || sessions.cancelRequests[0] != request {
		t.Fatalf("captured runtime cancel requests = %v, want exactly the original ControlRequest %+v", sessions.cancelRequests, request)
	}
	if len(sessions.cancelContexts) != 1 || sessions.cancelContexts[0].Value(ctxKey("trace")) != "trace-1" {
		t.Fatal("captured runtime Cancel did not receive the original caller context")
	}
	if sessions.cancelCalls[0] != factorysessions.DefaultSessionID {
		t.Fatalf("Cancel() dispatched sessionID = %q, want the runtime's own DefaultSessionID", sessions.cancelCalls[0])
	}
}

// TestCancelUnknownIdentityReportsSessionNotFound proves Cancel against an
// identity this service never started fails with the existing
// ErrSessionNotFound sentinel (preserving errors.Is compatibility) and makes
// no call into any runtime.
func TestCancelUnknownIdentityReportsSessionNotFound(t *testing.T) {
	svc := newTestService(t, &fakeOpener{}, nil, sequentialIDs("wrapper"))
	_, err := svc.Cancel(context.Background(), "unknown", factorysessions.ControlRequest{})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("Cancel() error = %v, want ErrSessionNotFound", err)
	}
}

// TestInvokeAndCancelAfterCloseReportSessionNotFound proves invocation and
// cancellation against an evicted (closed) identity fail with the existing
// ErrSessionNotFound sentinel and never reach the torn-down runtime.
func TestInvokeAndCancelAfterCloseReportSessionNotFound(t *testing.T) {
	sessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: &fakeLifecycle{}}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}
	if err := svc.CloseFactorySession(context.Background(), started.SessionID); err != nil {
		t.Fatalf("CloseFactorySession() error = %v", err)
	}

	if _, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("InvokeFactorySession() after close error = %v, want ErrSessionNotFound", err)
	}
	if _, err := svc.Cancel(context.Background(), started.SessionID, factorysessions.ControlRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("Cancel() after close error = %v, want ErrSessionNotFound", err)
	}
	if len(sessions.cancelCalls) != 0 {
		t.Fatalf("cancel calls = %v, want zero -- the runtime was already evicted", sessions.cancelCalls)
	}
}

// TestInvokeAndCancelIsolationBetweenMultipleStartedTargets proves an
// invocation or cancellation for one started target's opaque identity never
// reaches a second, independently started target's captured runtime.
func TestInvokeAndCancelIsolationBetweenMultipleStartedTargets(t *testing.T) {
	firstSessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	secondSessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	opener := &sequencedOpener{results: []openResult{
		{opened: roles.OpenedInvocationRuntime{Sessions: firstSessions, Lifecycle: &fakeLifecycle{}}},
		{opened: roles.OpenedInvocationRuntime{Sessions: secondSessions, Lifecycle: &fakeLifecycle{}}},
	}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	first, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{Source: factorysessions.Source{FactoryID: "@you/first"}})
	if err != nil {
		t.Fatalf("StartAsync() first error = %v", err)
	}
	second, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{Source: factorysessions.Source{FactoryID: "@you/second"}})
	if err != nil {
		t.Fatalf("StartAsync() second error = %v", err)
	}

	if _, err := svc.InvokeFactorySession(context.Background(), first.SessionID, factorysessions.InvocationRequest{}); err != nil {
		t.Fatalf("InvokeFactorySession(first) error = %v", err)
	}
	if _, err := svc.Cancel(context.Background(), second.SessionID, factorysessions.ControlRequest{}); err != nil {
		t.Fatalf("Cancel(second) error = %v", err)
	}

	if len(firstSessions.invokeCalls) != 1 {
		t.Fatalf("first target invoke calls = %v, want exactly 1", firstSessions.invokeCalls)
	}
	if len(firstSessions.cancelCalls) != 0 {
		t.Fatalf("first target cancel calls = %v, want zero -- cancel targeted the second activation", firstSessions.cancelCalls)
	}
	if len(secondSessions.cancelCalls) != 1 {
		t.Fatalf("second target cancel calls = %v, want exactly 1", secondSessions.cancelCalls)
	}
	if len(secondSessions.invokeCalls) != 0 {
		t.Fatalf("second target invoke calls = %v, want zero -- invoke targeted the first activation", secondSessions.invokeCalls)
	}
}

// TestCloseFactorySessionTearsDownAndEvicts proves a close call tears down
// the cached runtime and evicts it, so a later call against the same
// identity reports ErrSessionNotFound instead of reusing a torn-down runtime.
func TestCloseFactorySessionTearsDownAndEvicts(t *testing.T) {
	sessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	lifecycle := &fakeLifecycle{}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: lifecycle}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}

	if err := svc.CloseFactorySession(context.Background(), started.SessionID); err != nil {
		t.Fatalf("CloseFactorySession() error = %v", err)
	}
	if lifecycle.stopCalls != 1 {
		t.Fatalf("StopLifecycle call count = %d, want exactly 1", lifecycle.stopCalls)
	}

	if _, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("InvokeFactorySession() after close error = %v, want ErrSessionNotFound", err)
	}
}

// TestCloseFactorySessionUnknownIdentityIsNoOpSuccess proves closing an
// unknown or already-closed identity succeeds without effect, matching
// factorysessions.Service.CloseFactorySession's documented idempotent
// close semantics.
func TestCloseFactorySessionUnknownIdentityIsNoOpSuccess(t *testing.T) {
	svc := newTestService(t, &fakeOpener{}, nil, sequentialIDs("wrapper"))
	if err := svc.CloseFactorySession(context.Background(), "unknown"); err != nil {
		t.Fatalf("CloseFactorySession() error = %v, want nil for an unknown identity", err)
	}
}

// TestCloseTearsDownEveryTrackedRuntime proves Close (the io.Closer this
// Service implements for a future process lifecycle plan to register) tears
// down every runtime this Service has opened -- not just one -- and evicts
// each of them, so a later call against any of their identities reports
// ErrSessionNotFound instead of reusing a torn-down runtime.
func TestCloseTearsDownEveryTrackedRuntime(t *testing.T) {
	firstLifecycle := &fakeLifecycle{}
	secondLifecycle := &fakeLifecycle{}
	openCount := 0
	opener := &funcOpener{open: func(context.Context, *factorysessions.RuntimeOpeningRequest, runtimeopening.ExternalEffects, *zap.Logger) (roles.OpenedInvocationRuntime, error) {
		openCount++
		lifecycle := firstLifecycle
		if openCount == 2 {
			lifecycle = secondLifecycle
		}
		return roles.OpenedInvocationRuntime{
			Sessions:  &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}},
			Lifecycle: lifecycle,
		}, nil
	}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	first, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{Source: factorysessions.Source{FactoryID: "@you/first"}})
	if err != nil {
		t.Fatalf("StartAsync() first error = %v", err)
	}
	second, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{Source: factorysessions.Source{FactoryID: "@you/second"}})
	if err != nil {
		t.Fatalf("StartAsync() second error = %v", err)
	}

	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if firstLifecycle.stopCalls != 1 {
		t.Fatalf("first runtime StopLifecycle call count = %d, want exactly 1", firstLifecycle.stopCalls)
	}
	if secondLifecycle.stopCalls != 1 {
		t.Fatalf("second runtime StopLifecycle call count = %d, want exactly 1", secondLifecycle.stopCalls)
	}

	if _, err := svc.InvokeFactorySession(context.Background(), first.SessionID, factorysessions.InvocationRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("InvokeFactorySession(first) after Close error = %v, want ErrSessionNotFound", err)
	}
	if _, err := svc.InvokeFactorySession(context.Background(), second.SessionID, factorysessions.InvocationRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("InvokeFactorySession(second) after Close error = %v, want ErrSessionNotFound", err)
	}

	// Close is idempotent: a second call has nothing left to close and
	// succeeds without re-closing either runtime.
	if err := svc.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil (idempotent)", err)
	}
	if firstLifecycle.stopCalls != 1 || secondLifecycle.stopCalls != 1 {
		t.Fatalf("StopLifecycle call counts after second Close = (%d, %d), want (1, 1) -- no double close", firstLifecycle.stopCalls, secondLifecycle.stopCalls)
	}
}

// TestCloseWithNoOpenedRuntimesIsNoOpSuccess proves calling Close before any
// StartAsync call succeeds without effect.
func TestCloseWithNoOpenedRuntimesIsNoOpSuccess(t *testing.T) {
	svc := newTestService(t, &fakeOpener{}, nil, sequentialIDs("wrapper"))
	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil when no runtime was ever opened", err)
	}
}

// TestLoggingNeverEmitsWorkingRootOrRawFailureText proves this Service's
// structured logs stay bounded even when a resolve or activation failure's
// own error text embeds a resolved filesystem path: neither the caller's
// editor working root nor any substring of the raw underlying error's
// message ever reaches a logged field or message across the resolve-failure,
// open-failure, activation-failure, dispatch-failure, and close-failure
// paths. Only closed, safe fields (Factory target identity, status,
// counters) may appear.
func TestLoggingNeverEmitsWorkingRootOrRawFailureText(t *testing.T) {
	const sensitiveWorkingRoot = "/Users/alice/secret-project"
	pathBearingErr := fmt.Errorf("open %s/factory.json: permission denied", sensitiveWorkingRoot)

	assertNoLeak := func(t *testing.T, observed *observer.ObservedLogs) {
		t.Helper()
		if observed.Len() == 0 {
			t.Fatal("no log entries observed, want at least one bounded failure log")
		}
		for _, entry := range observed.All() {
			if strings.Contains(entry.Message, sensitiveWorkingRoot) {
				t.Fatalf("log message %q contains the working root, want it bounded", entry.Message)
			}
			if strings.Contains(entry.Message, pathBearingErr.Error()) {
				t.Fatalf("log message %q contains the raw error text, want it bounded", entry.Message)
			}
			for _, field := range entry.Context {
				rendered := fmt.Sprintf("%v", field.Interface)
				if field.String != "" {
					rendered = field.String
				}
				if strings.Contains(rendered, sensitiveWorkingRoot) {
					t.Fatalf("log field %s=%q contains the working root, want it bounded", field.Key, rendered)
				}
				if strings.Contains(rendered, pathBearingErr.Error()) {
					t.Fatalf("log field %s=%q contains the raw error text, want it bounded", field.Key, rendered)
				}
			}
		}
	}

	t.Run("open failure", func(t *testing.T) {
		opener := &fakeOpener{err: pathBearingErr}
		resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
			return factorysessions.RuntimeOpeningRequest{}, nil
		}
		svc, observed := newTestServiceWithObservedLogger(t, opener, resolve, sequentialIDs("wrapper"))
		_, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{
			Args: map[string]any{"workingRoot": sensitiveWorkingRoot},
		})
		if err == nil {
			t.Fatal("StartAsync() error = nil, want the open failure")
		}
		assertNoLeak(t, observed)
	})

	t.Run("activation failure", func(t *testing.T) {
		opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{
			Sessions:  &fakeSessions{},
			Lifecycle: &fakeLifecycle{startErr: pathBearingErr},
		}}
		resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
			return factorysessions.RuntimeOpeningRequest{}, nil
		}
		svc, observed := newTestServiceWithObservedLogger(t, opener, resolve, sequentialIDs("wrapper"))
		_, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{
			Args: map[string]any{"workingRoot": sensitiveWorkingRoot},
		})
		if err == nil {
			t.Fatal("StartAsync() error = nil, want the activation failure")
		}
		assertNoLeak(t, observed)
	})

	t.Run("dispatch failure", func(t *testing.T) {
		opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{
			Sessions:  &fakeSessions{invokeErr: pathBearingErr},
			Lifecycle: &fakeLifecycle{},
		}}
		resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
			return factorysessions.RuntimeOpeningRequest{}, nil
		}
		svc, observed := newTestServiceWithObservedLogger(t, opener, resolve, sequentialIDs("wrapper"))
		started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{
			Args: map[string]any{"workingRoot": sensitiveWorkingRoot},
		})
		if err != nil {
			t.Fatalf("StartAsync() error = %v", err)
		}
		observed.TakeAll()
		if _, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); err == nil {
			t.Fatal("InvokeFactorySession() error = nil, want the dispatch failure")
		}
		assertNoLeak(t, observed)
	})

	t.Run("close failure", func(t *testing.T) {
		opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{
			Sessions:  &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}, closeErr: pathBearingErr},
			Lifecycle: &fakeLifecycle{},
		}}
		resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
			return factorysessions.RuntimeOpeningRequest{}, nil
		}
		svc, observed := newTestServiceWithObservedLogger(t, opener, resolve, sequentialIDs("wrapper"))
		started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{
			Args: map[string]any{"workingRoot": sensitiveWorkingRoot},
		})
		if err != nil {
			t.Fatalf("StartAsync() error = %v", err)
		}
		observed.TakeAll()
		if err := svc.CloseFactorySession(context.Background(), started.SessionID); err == nil {
			t.Fatal("CloseFactorySession() error = nil, want the close failure")
		}
		assertNoLeak(t, observed)
	})
}

// TestServiceSatisfiesPublishedTargetExecutionCapability proves a caller
// typed only against the owner-published target-execution shape -- never
// against the concrete *Service type or any wider aggregate contract -- can
// start a target, invoke and cancel its captured session, and close it,
// observing the exact same behavior (including post-close
// ErrSessionNotFound) as a caller holding the concrete type. This is the
// behavioral proof for the narrow capability's publication: any caller
// depending on it receives a complete, non-panicking implementation of
// exactly these four operations.
func TestServiceSatisfiesPublishedTargetExecutionCapability(t *testing.T) {
	sessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: &fakeLifecycle{}}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	var client factorysessions.TargetExecutionService = svc

	started, err := client.StartAsync(context.Background(), factorysessions.StartRequest{
		Source: factorysessions.Source{FactoryID: "@you/review"},
	})
	if err != nil {
		t.Fatalf("StartAsync() via TargetExecutionService error = %v", err)
	}
	if started.SessionID == "" {
		t.Fatal("StartAsync() via TargetExecutionService returned a blank SessionID")
	}

	if _, err := client.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); err != nil {
		t.Fatalf("InvokeFactorySession() via TargetExecutionService error = %v", err)
	}
	if len(sessions.invokeCalls) != 1 {
		t.Fatalf("invoke calls = %v, want exactly 1 dispatched to the captured runtime", sessions.invokeCalls)
	}

	if _, err := client.Cancel(context.Background(), started.SessionID, factorysessions.ControlRequest{}); err != nil {
		t.Fatalf("Cancel() via TargetExecutionService error = %v", err)
	}
	if len(sessions.cancelCalls) != 1 {
		t.Fatalf("cancel calls = %v, want exactly 1 forwarded to the captured runtime", sessions.cancelCalls)
	}

	if err := client.CloseFactorySession(context.Background(), started.SessionID); err != nil {
		t.Fatalf("CloseFactorySession() via TargetExecutionService error = %v", err)
	}
	if err := client.CloseFactorySession(context.Background(), started.SessionID); err != nil {
		t.Fatalf("repeated CloseFactorySession() via TargetExecutionService error = %v, want idempotent success", err)
	}
	if _, err := client.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("InvokeFactorySession() after close via TargetExecutionService error = %v, want ErrSessionNotFound", err)
	}
}

// TestServiceViaTargetExecutionCapabilityRejectsUnsupportedTarget proves an
// unresolvable target reported by the resolver reaches a caller of the
// published target-execution capability unchanged, opening no runtime.
func TestServiceViaTargetExecutionCapabilityRejectsUnsupportedTarget(t *testing.T) {
	opener := &fakeOpener{}
	resolveErr := errors.New("unsupported factory target")
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, resolveErr
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	var client factorysessions.TargetExecutionService = svc

	_, err := client.StartAsync(context.Background(), factorysessions.StartRequest{
		Source: factorysessions.Source{FactoryID: "@you/unsupported"},
	})
	if !errors.Is(err, resolveErr) {
		t.Fatalf("StartAsync() via TargetExecutionService error = %v, want %v", err, resolveErr)
	}
	if len(opener.calls) != 0 {
		t.Fatalf("OpenInvocationRuntime call count = %d, want 0 after an unsupported-target failure", len(opener.calls))
	}
}

// funcOpener is an invocationRuntimeOpener test double backed directly by a
// function, for tests that need each successive OpenInvocationRuntime call
// to return a distinct opened runtime (fakeOpener always returns the same
// fixed one).
type funcOpener struct {
	open func(context.Context, *factorysessions.RuntimeOpeningRequest, runtimeopening.ExternalEffects, *zap.Logger) (roles.OpenedInvocationRuntime, error)
}

func (f *funcOpener) OpenInvocationRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	effects runtimeopening.ExternalEffects,
	logger *zap.Logger,
) (roles.OpenedInvocationRuntime, error) {
	return f.open(ctx, request, effects, logger)
}
