package ondemandtarget

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
)

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

// TestCancelInterruptsOnlyTheActiveInvocation proves Cancel owns the
// request-scoped context of the captured target's active invocation, waits
// for it to terminalize as CANCELED, and replaces the stopped runtime under
// the same opaque target identity for a later prompt. It intentionally does
// not delegate to factorysessions.Service.Cancel: that contract addresses
// durable sessions, while this on-demand runtime is a live target whose
// provider work is governed by the runtime lifecycle.
func TestCancelInterruptsOnlyTheActiveInvocation(t *testing.T) {
	startedSignal := make(chan struct{})
	invocationCount := 0
	sessions := &fakeSessions{invoke: func(ctx context.Context, _ string, _ factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
		invocationCount++
		if invocationCount == 1 {
			close(startedSignal)
			<-ctx.Done()
			return factorysessions.InvocationResult{}, ctx.Err()
		}
		return factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}, nil
	}}
	lifecycle := &fakeLifecycle{}
	replacementSessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	replacementLifecycle := &fakeLifecycle{}
	opener := &sequencedOpener{results: []openResult{
		{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: lifecycle}},
		{opened: roles.OpenedInvocationRuntime{Sessions: replacementSessions, Lifecycle: replacementLifecycle}},
	}}
	resolve := func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}
	svc := newTestService(t, opener, resolve, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}

	requestID := "turn-1"
	invokeResult := make(chan struct {
		result factorysessions.InvocationResult
		err    error
	}, 1)
	go func() {
		result, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{RequestID: &requestID})
		invokeResult <- struct {
			result factorysessions.InvocationResult
			err    error
		}{result: result, err: err}
	}()
	select {
	case <-startedSignal:
	case <-time.After(5 * time.Second):
		t.Fatal("active invocation did not start")
	}

	result, err := svc.Cancel(context.Background(), started.SessionID, factorysessions.ControlRequest{RequestID: "control-1", Reason: "user requested"})
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if result.SessionID != started.SessionID || result.Operation != factorysessions.LifecycleControlCancel || result.Outcome != factorysessions.LifecycleControlOutcomeAccepted {
		t.Fatalf("Cancel() result = %+v, want accepted cancellation for %q", result, started.SessionID)
	}
	got := <-invokeResult
	if got.err != nil {
		t.Fatalf("canceled InvokeFactorySession() error = %v, want published canceled result", got.err)
	}
	if got.result.Status != factorysessions.InvocationTerminalStatusCanceled || got.result.ErrorCode != string(factorysessions.InvocationErrorCodeCanceled) || got.result.SessionID != started.SessionID || got.result.RequestID != requestID {
		t.Fatalf("canceled InvokeFactorySession() result = %+v, want the published canceled outcome for %q", got.result, started.SessionID)
	}
	if lifecycle.stopCalls != 1 {
		t.Fatalf("StopLifecycle calls = %d, want 1 because cancellation stops the captured provider runtime", lifecycle.stopCalls)
	}

	second, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{})
	if err != nil || second.Status != factorysessions.InvocationTerminalStatusCompleted {
		t.Fatalf("later InvokeFactorySession() = (%+v, %v), want completed invocation through the replacement runtime", second, err)
	}
	if len(opener.calls) != 2 {
		t.Fatalf("OpenInvocationRuntime calls = %d, want the original and replacement runtime", len(opener.calls))
	}
	if len(replacementSessions.invokeCalls) != 1 || replacementLifecycle.stopCalls != 0 {
		t.Fatalf("replacement runtime = (invoke calls %v, StopLifecycle calls %d), want one later invocation and no close", replacementSessions.invokeCalls, replacementLifecycle.stopCalls)
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
	canceled, err := svc.Cancel(context.Background(), second.SessionID, factorysessions.ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel(second) error = %v", err)
	}
	if canceled.SessionID != second.SessionID || canceled.Outcome != factorysessions.LifecycleControlOutcomeNoOp {
		t.Fatalf("Cancel(second) result = %+v, want a no-op scoped to the inactive second target", canceled)
	}

	if len(firstSessions.invokeCalls) != 1 {
		t.Fatalf("first target invoke calls = %v, want exactly 1", firstSessions.invokeCalls)
	}
	if len(firstSessions.cancelCalls) != 0 || len(secondSessions.cancelCalls) != 0 {
		t.Fatalf("durable session cancel calls = (%v, %v), want none -- an inactive live target has no invocation context to interrupt", firstSessions.cancelCalls, secondSessions.cancelCalls)
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

	cancelled, err := client.Cancel(context.Background(), started.SessionID, factorysessions.ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel() via TargetExecutionService error = %v", err)
	}
	if cancelled.SessionID != started.SessionID || cancelled.Outcome != factorysessions.LifecycleControlOutcomeNoOp {
		t.Fatalf("Cancel() via TargetExecutionService result = %+v, want a no-op scoped to a completed target", cancelled)
	}
	if len(sessions.cancelCalls) != 0 {
		t.Fatalf("durable session cancel calls = %v, want none -- this target cancels only a live invocation context", sessions.cancelCalls)
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
