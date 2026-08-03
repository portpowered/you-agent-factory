package ondemandtarget

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.uber.org/zap"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

	cancelCalls []string

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
	_ context.Context, sessionID string, _ factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelCalls = append(f.cancelCalls, sessionID)
	return factorysessions.LifecycleControlResult{}, nil
}

func (f *fakeSessions) CloseFactorySession(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls = append(f.closeCalls, sessionID)
	return f.closeErr
}

func newTestService(t *testing.T, opener *fakeOpener, resolve RuntimeResolver, generateID factorysessions.SessionIDGenerator) *Service {
	t.Helper()
	return &Service{
		factory:    opener,
		resolve:    resolve,
		generateID: generateID,
		logger:     zap.NewNop(),
		runtimes:   make(map[string]*activatedRuntime),
	}
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

// TestStartFactoryTargetSubstitutesGeneratedIdentity proves a successful
// start resolves the target, opens exactly one runtime, dispatches the
// request's content synchronously, and returns the real published
// InvocationResult with SessionID substituted for this service's own
// generated identity (never the runtime's shared internal constant).
func TestStartFactoryTargetSubstitutesGeneratedIdentity(t *testing.T) {
	sessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "hello"},
		},
	}}
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

	result, err := svc.StartFactoryTarget(context.Background(), factorysessions.StartRequest{
		RequestID: "req-1",
		Source:    factorysessions.Source{FactoryID: "@you/review"},
		Args:      map[string]any{"workingRoot": "/work/project", "content": []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hi"}}},
	})
	if err != nil {
		t.Fatalf("StartFactoryTarget() error = %v", err)
	}
	if len(resolveCalls) != 1 || resolveCalls[0] != "@you/review|/work/project" {
		t.Fatalf("resolve calls = %v, want exactly one call for @you/review|/work/project", resolveCalls)
	}
	if len(opener.calls) != 1 {
		t.Fatalf("OpenInvocationRuntime call count = %d, want exactly 1", len(opener.calls))
	}
	if len(sessions.invokeCalls) != 1 || sessions.invokeCalls[0] != factorysessions.DefaultSessionID {
		t.Fatalf("InvokeFactorySession calls = %v, want exactly one call against DefaultSessionID", sessions.invokeCalls)
	}
	if result.SessionID != "wrapper-1" {
		t.Fatalf("result.SessionID = %q, want the generated wrapper identity", result.SessionID)
	}
	if result.Status != factorysessions.InvocationTerminalStatusCompleted {
		t.Fatalf("result.Status = %q, want the real published status", result.Status)
	}
	if len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "hello" {
		t.Fatalf("result.PrimaryResult = %+v, want the real published primary result preserved", result.PrimaryResult)
	}
}

// TestStartFactoryTargetPropagatesResolveFailure proves a resolver failure
// opens no runtime.
func TestStartFactoryTargetPropagatesResolveFailure(t *testing.T) {
	opener := &fakeOpener{}
	resolveErr := errors.New("resolve boom")
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, resolveErr
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	_, err := svc.StartFactoryTarget(context.Background(), factorysessions.StartRequest{})
	if !errors.Is(err, resolveErr) {
		t.Fatalf("StartFactoryTarget() error = %v, want %v", err, resolveErr)
	}
	if len(opener.calls) != 0 {
		t.Fatalf("OpenInvocationRuntime call count = %d, want 0 after a resolve failure", len(opener.calls))
	}
}

// TestStartFactoryTargetDispatchFailureClosesTheJustOpenedRuntime proves a
// synchronous dispatch failure after a successful open still tears down the
// just-opened runtime (StopLifecycle/WaitForRuntime observed) rather than
// leaking it, and never caches an identity for it.
func TestStartFactoryTargetDispatchFailureClosesTheJustOpenedRuntime(t *testing.T) {
	sessions := &fakeSessions{invokeErr: errors.New("dispatch boom")}
	lifecycle := &fakeLifecycle{}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: lifecycle}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	_, err := svc.StartFactoryTarget(context.Background(), factorysessions.StartRequest{})
	if err == nil {
		t.Fatal("StartFactoryTarget() error = nil, want the dispatch failure")
	}
	if lifecycle.stopCalls != 1 {
		t.Fatalf("StopLifecycle call count = %d, want exactly 1 (compensating close)", lifecycle.stopCalls)
	}
	if len(sessions.closeCalls) != 1 {
		t.Fatalf("CloseFactorySession call count = %d, want exactly 1 (compensating close)", len(sessions.closeCalls))
	}
	svc.mu.Lock()
	runtimeCount := len(svc.runtimes)
	svc.mu.Unlock()
	if runtimeCount != 0 {
		t.Fatalf("cached runtime count = %d, want 0 after a dispatch failure", runtimeCount)
	}
}

// TestInvokeFactoryTargetReusesTheCachedRuntime proves a later
// InvokeFactoryTarget call against a previously started identity dispatches
// against the exact cached runtime's DefaultSessionID without opening a
// second runtime.
func TestInvokeFactoryTargetReusesTheCachedRuntime(t *testing.T) {
	sessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: &fakeLifecycle{}}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	started, err := svc.StartFactoryTarget(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartFactoryTarget() error = %v", err)
	}

	if _, err := svc.InvokeFactoryTarget(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); err != nil {
		t.Fatalf("InvokeFactoryTarget() error = %v", err)
	}

	if len(opener.calls) != 1 {
		t.Fatalf("OpenInvocationRuntime call count = %d, want exactly 1 (no second open)", len(opener.calls))
	}
	if len(sessions.invokeCalls) != 2 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 2 (start's dispatch + the later invoke)", len(sessions.invokeCalls))
	}
	for _, sessionID := range sessions.invokeCalls {
		if sessionID != factorysessions.DefaultSessionID {
			t.Fatalf("InvokeFactorySession sessionID = %q, want the runtime's own DefaultSessionID", sessionID)
		}
	}
}

// TestInvokeFactoryTargetUnknownIdentityReportsSessionNotFound proves an
// identity this service never started reports ErrSessionNotFound.
func TestInvokeFactoryTargetUnknownIdentityReportsSessionNotFound(t *testing.T) {
	svc := newTestService(t, &fakeOpener{}, nil, sequentialIDs("wrapper"))
	_, err := svc.InvokeFactoryTarget(context.Background(), "unknown", factorysessions.InvocationRequest{})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("InvokeFactoryTarget() error = %v, want ErrSessionNotFound", err)
	}
}

// TestCloseFactoryTargetTearsDownAndEvicts proves a close call tears down
// the cached runtime and evicts it, so a later call against the same
// identity reports ErrSessionNotFound instead of reusing a torn-down runtime.
func TestCloseFactoryTargetTearsDownAndEvicts(t *testing.T) {
	sessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	lifecycle := &fakeLifecycle{}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: lifecycle}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	started, err := svc.StartFactoryTarget(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartFactoryTarget() error = %v", err)
	}

	if err := svc.CloseFactoryTarget(context.Background(), started.SessionID); err != nil {
		t.Fatalf("CloseFactoryTarget() error = %v", err)
	}
	if lifecycle.stopCalls != 1 {
		t.Fatalf("StopLifecycle call count = %d, want exactly 1", lifecycle.stopCalls)
	}

	if _, err := svc.InvokeFactoryTarget(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("InvokeFactoryTarget() after close error = %v, want ErrSessionNotFound", err)
	}
}

// TestCloseFactoryTargetUnknownIdentityIsNoOpSuccess proves closing an
// unknown or already-closed identity succeeds without effect, matching
// factorysessions.Service.CloseFactorySession's documented idempotent
// close semantics.
func TestCloseFactoryTargetUnknownIdentityIsNoOpSuccess(t *testing.T) {
	svc := newTestService(t, &fakeOpener{}, nil, sequentialIDs("wrapper"))
	if err := svc.CloseFactoryTarget(context.Background(), "unknown"); err != nil {
		t.Fatalf("CloseFactoryTarget() error = %v, want nil for an unknown identity", err)
	}
}
