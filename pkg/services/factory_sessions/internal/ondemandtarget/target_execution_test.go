// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package ondemandtarget

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap/zaptest/observer"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

// TestInvokeFactorySessionReusesTheCachedRuntime proves InvokeFactorySession
// calls against a previously started identity dispatch against the exact
// cached runtime's explicit Factory Session without opening a second runtime,
// and returns that same caller-facing generated identity.
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
	if got := opener.calls[0].FactorySession.FactorySessionID; got != started.SessionID {
		t.Fatalf("OpenInvocationRuntime FactorySessionID = %q, want preallocated session %q", got, started.SessionID)
	}
	if len(sessions.invokeCalls) != 2 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 2", len(sessions.invokeCalls))
	}
	for _, sessionID := range sessions.invokeCalls {
		if sessionID != started.SessionID {
			t.Fatalf("InvokeFactorySession sessionID = %q, want explicit session %q", sessionID, started.SessionID)
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
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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

// TestCancelFansCapturedTurnBeforeInvocationCancellation proves the ACP-facing
// target control reaches the exact pre-replacement Factory Runtime with its
// immutable turn/control identity before the invocation context is canceled.
// A retry after replacement uses the retained evidence and cannot control the
// later runtime or turn.
func TestCancelFansCapturedTurnBeforeInvocationCancellation(t *testing.T) {
	invocationStarted := make(chan struct{})
	var invocationContext context.Context
	sessions := &fakeSessions{invoke: func(ctx context.Context, _ string, _ factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
		invocationContext = ctx
		close(invocationStarted)
		<-ctx.Done()
		return factorysessions.InvocationResult{}, ctx.Err()
	}}
	originalRuntime := &recordingTurnControlRuntime{}
	originalRuntime.onTerminate = func() {
		select {
		case <-invocationContext.Done():
			t.Error("captured Factory turn control ran after invocation context cancellation")
		default:
		}
	}
	originalLifecycle := &fakeLifecycle{hosted: fakeHostedInstance{runtime: originalRuntime}}
	replacementRuntime := &recordingTurnControlRuntime{}
	replacementLifecycle := &fakeLifecycle{hosted: fakeHostedInstance{runtime: replacementRuntime}}
	opener := &sequencedOpener{results: []openResult{
		{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: originalLifecycle}},
		{opened: roles.OpenedInvocationRuntime{Sessions: &fakeSessions{}, Lifecycle: replacementLifecycle}},
	}}
	svc := newTestService(t, opener, func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	invoked := make(chan error, 1)
	go func() {
		_, invokeErr := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{})
		invoked <- invokeErr
	}()
	<-invocationStarted

	control := factorysessions.ControlRequest{
		RequestID: "control-captured-1",
		Reason:    "acp session/cancel",
		TurnID:    "turn-captured-1",
	}
	if _, err := svc.Cancel(context.Background(), started.SessionID, control); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := <-invoked; err != nil {
		t.Fatalf("InvokeFactorySession after Cancel: %v", err)
	}
	calls := originalRuntime.terminateCalls()
	if len(calls) != 1 {
		t.Fatalf("original Factory Runtime ControlTerminate calls = %#v, want one", calls)
	}
	if got := calls[0]; got.TurnID != control.TurnID || got.ControlID != control.RequestID || got.WorkerSessionAction != factoryruntime.WorkerSessionControlActionCancel {
		t.Fatalf("original Factory Runtime control = %#v, want captured CANCEL identity", got)
	}

	if _, err := svc.Cancel(context.Background(), started.SessionID, control); err != nil {
		t.Fatalf("retry Cancel: %v", err)
	}
	if got := replacementRuntime.terminateCalls(); len(got) != 0 {
		t.Fatalf("replacement Factory Runtime ControlTerminate calls = %#v, want none", got)
	}
}

// TestTerminateFactorySessionFansCapturedTurnBeforeCleanup proves a committed
// ACP close controls the exact active Factory Runtime before invocation or
// runtime cleanup can stop the target. The original captured control never
// reaches a later replacement because termination evicts its runtime.
func TestTerminateFactorySessionFansCapturedTurnBeforeCleanup(t *testing.T) {
	invocationStarted := make(chan struct{})
	var invocationContext context.Context
	sessions := &fakeSessions{invoke: func(ctx context.Context, _ string, _ factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
		invocationContext = ctx
		close(invocationStarted)
		<-ctx.Done()
		return factorysessions.InvocationResult{}, ctx.Err()
	}}
	originalRuntime := &recordingTurnControlRuntime{}
	originalRuntime.onTerminate = func() {
		select {
		case <-invocationContext.Done():
			t.Error("captured Factory turn termination ran after invocation context cancellation")
		default:
		}
	}
	lifecycle := &fakeLifecycle{hosted: fakeHostedInstance{runtime: originalRuntime}}
	svc := newTestService(t, &fakeOpener{opened: roles.OpenedInvocationRuntime{
		Sessions: sessions, Lifecycle: lifecycle,
	}}, func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	invoked := make(chan error, 1)
	go func() {
		_, invokeErr := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{})
		invoked <- invokeErr
	}()
	<-invocationStarted

	control := factorysessions.ControlRequest{
		RequestID: "control-close-1",
		Reason:    "acp session/close",
		TurnID:    "turn-close-1",
	}
	if err := svc.TerminateFactorySession(context.Background(), started.SessionID, control); err != nil {
		t.Fatalf("TerminateFactorySession: %v", err)
	}
	if err := <-invoked; err != nil {
		t.Fatalf("InvokeFactorySession after TerminateFactorySession: %v", err)
	}
	calls := originalRuntime.terminateCalls()
	if len(calls) != 1 {
		t.Fatalf("original Factory Runtime ControlTerminate calls = %#v, want one", calls)
	}
	if got := calls[0]; got.TurnID != control.TurnID || got.ControlID != control.RequestID || got.WorkerSessionAction != factoryruntime.WorkerSessionControlActionTerminate {
		t.Fatalf("original Factory Runtime control = %#v, want captured TERMINATE identity", got)
	}
	if lifecycle.stopCalls != 1 {
		t.Fatalf("StopLifecycle calls = %d, want one cleanup after the captured control", lifecycle.stopCalls)
	}
	if err := svc.TerminateFactorySession(context.Background(), started.SessionID, control); err != nil {
		t.Fatalf("repeated TerminateFactorySession: %v, want idempotent success", err)
	}
	if got := originalRuntime.terminateCalls(); len(got) != 1 {
		t.Fatalf("original Factory Runtime ControlTerminate calls after retry = %#v, want one", got)
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

// TestBTRCP0ConcurrentTargetCancellationIsolationCharacterization freezes the
// concurrent target boundary: canceling one active target publishes exactly
// one canceled result and replaces only that target's runtime while a second
// target continues its blocked invocation to completion. Repeated cancel and
// close controls must not replay terminal work or cleanup effects.
func TestBTRCP0ConcurrentTargetCancellationIsolationCharacterization(t *testing.T) {
	fixture := newBTRCConcurrentTargetFixture(t)
	first, second := fixture.startTargets(t)
	firstDone, secondDone := fixture.invokeTargets(t, first.SessionID, second.SessionID)
	waitBTRCTargetStarted(t, "first", fixture.firstStarted)
	waitBTRCTargetStarted(t, "second", fixture.secondStarted)

	canceled, err := fixture.svc.Cancel(t.Context(), first.SessionID, factorysessions.ControlRequest{RequestID: "cancel-first"})
	if err != nil {
		t.Fatalf("Cancel(first) error = %v", err)
	}
	if canceled.SessionID != first.SessionID || canceled.Outcome != factorysessions.LifecycleControlOutcomeAccepted || canceled.Status != factorysessions.LifecycleStatusRunning {
		t.Fatalf("Cancel(first) result = %+v, want ACCEPTED/RUNNING for first target", canceled)
	}
	fixture.assertSecondStillActive(t, secondDone)

	repeatedCancel, err := fixture.svc.Cancel(t.Context(), first.SessionID, factorysessions.ControlRequest{RequestID: "cancel-first"})
	if err != nil {
		t.Fatalf("repeated Cancel(first) error = %v", err)
	}
	if repeatedCancel.SessionID != first.SessionID || repeatedCancel.Outcome != factorysessions.LifecycleControlOutcomeNoOp {
		t.Fatalf("repeated Cancel(first) result = %+v, want a no-op on the replacement runtime", repeatedCancel)
	}
	fixture.assertReplacementStillActive(t)

	close(fixture.secondRelease)
	firstOutcome, secondOutcome := awaitBTRCTargetInvocations(t, firstDone, secondDone)
	fixture.assertInvocationOutcomes(t, first, second, firstOutcome, secondOutcome)
	fixture.assertCanceledCleanup(t)
	fixture.closeTargets(t, first.SessionID, second.SessionID)
	fixture.assertFinalCleanup(t)
	if len(fixture.firstSessions.invokeCalls) != 1 || len(fixture.secondSessions.invokeCalls) != 1 || len(fixture.replacementSessions.invokeCalls) != 0 {
		t.Fatalf("target invocation calls = first:%v second:%v replacement:%v, want one/original one/none", fixture.firstSessions.invokeCalls, fixture.secondSessions.invokeCalls, fixture.replacementSessions.invokeCalls)
	}
	if len(fixture.opener.calls) != 3 {
		t.Fatalf("OpenInvocationRuntime calls = %d, want first, second, and first replacement only", len(fixture.opener.calls))
	}
}

type btrcTargetInvocationOutcome struct {
	result factorysessions.InvocationResult
	err    error
}

type btrcConcurrentTargetFixture struct {
	svc                        *Service
	opener                     *sequencedOpener
	firstSessions              *fakeSessions
	secondSessions             *fakeSessions
	replacementSessions        *fakeSessions
	firstLifecycle             *fakeLifecycle
	secondLifecycle            *fakeLifecycle
	replacementLifecycle       *fakeLifecycle
	firstStarted               chan struct{}
	secondStarted              chan struct{}
	secondRelease              chan struct{}
	firstRequestID             string
	secondRequestID            string
	firstArtifactsClosed       int
	secondArtifactsClosed      int
	replacementArtifactsClosed int
}

func newBTRCConcurrentTargetFixture(t *testing.T) *btrcConcurrentTargetFixture {
	t.Helper()
	fixture := &btrcConcurrentTargetFixture{
		firstStarted:         make(chan struct{}),
		secondStarted:        make(chan struct{}),
		secondRelease:        make(chan struct{}),
		firstRequestID:       "invoke-first",
		secondRequestID:      "invoke-second",
		firstLifecycle:       &fakeLifecycle{},
		secondLifecycle:      &fakeLifecycle{},
		replacementLifecycle: &fakeLifecycle{},
	}
	fixture.firstSessions = &fakeSessions{invoke: func(ctx context.Context, _ string, _ factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
		close(fixture.firstStarted)
		<-ctx.Done()
		return factorysessions.InvocationResult{}, ctx.Err()
	}}
	fixture.secondSessions = &fakeSessions{invoke: func(ctx context.Context, _ string, request factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
		close(fixture.secondStarted)
		select {
		case <-fixture.secondRelease:
			result := factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}
			if request.RequestID != nil {
				result.RequestID = *request.RequestID
			}
			return result, nil
		case <-ctx.Done():
			return factorysessions.InvocationResult{}, ctx.Err()
		}
	}}
	fixture.replacementSessions = &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	fixture.opener = &sequencedOpener{results: []openResult{
		{opened: roles.OpenedInvocationRuntime{
			Sessions: fixture.firstSessions, Lifecycle: fixture.firstLifecycle,
			CloseArtifacts: func() error { fixture.firstArtifactsClosed++; return nil },
		}},
		{opened: roles.OpenedInvocationRuntime{
			Sessions: fixture.secondSessions, Lifecycle: fixture.secondLifecycle,
			CloseArtifacts: func() error { fixture.secondArtifactsClosed++; return nil },
		}},
		{opened: roles.OpenedInvocationRuntime{
			Sessions: fixture.replacementSessions, Lifecycle: fixture.replacementLifecycle,
			CloseArtifacts: func() error { fixture.replacementArtifactsClosed++; return nil },
		}},
	}}
	fixture.svc = newTestService(t, fixture.opener, func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}, sequentialIDs("wrapper"))
	return fixture
}

func (fixture *btrcConcurrentTargetFixture) startTargets(t *testing.T) (factorysessions.AsyncStartResult, factorysessions.AsyncStartResult) {
	t.Helper()
	first, err := fixture.svc.StartAsync(t.Context(), factorysessions.StartRequest{Source: factorysessions.Source{FactoryID: "@you/first"}})
	if err != nil {
		t.Fatalf("StartAsync(first) error = %v", err)
	}
	second, err := fixture.svc.StartAsync(t.Context(), factorysessions.StartRequest{Source: factorysessions.Source{FactoryID: "@you/second"}})
	if err != nil {
		t.Fatalf("StartAsync(second) error = %v", err)
	}
	return first, second
}

func (fixture *btrcConcurrentTargetFixture) invokeTargets(t *testing.T, firstID, secondID string) (<-chan btrcTargetInvocationOutcome, <-chan btrcTargetInvocationOutcome) {
	t.Helper()
	firstDone := make(chan btrcTargetInvocationOutcome, 1)
	secondDone := make(chan btrcTargetInvocationOutcome, 1)
	go func() {
		result, err := fixture.svc.InvokeFactorySession(t.Context(), firstID, factorysessions.InvocationRequest{RequestID: &fixture.firstRequestID})
		firstDone <- btrcTargetInvocationOutcome{result: result, err: err}
	}()
	go func() {
		result, err := fixture.svc.InvokeFactorySession(t.Context(), secondID, factorysessions.InvocationRequest{RequestID: &fixture.secondRequestID})
		secondDone <- btrcTargetInvocationOutcome{result: result, err: err}
	}()
	return firstDone, secondDone
}

func waitBTRCTargetStarted(t *testing.T, name string, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s target invocation", name)
	}
}

func (fixture *btrcConcurrentTargetFixture) assertSecondStillActive(t *testing.T, secondDone <-chan btrcTargetInvocationOutcome) {
	t.Helper()
	select {
	case outcome := <-secondDone:
		t.Fatalf("second target terminalized during first cancellation = %+v, want it still blocked", outcome)
	default:
	}
	if fixture.secondLifecycle.stopCalls != 0 || len(fixture.secondSessions.closeCalls) != 0 || fixture.secondArtifactsClosed != 0 {
		t.Fatalf("second target cleanup during first cancellation = lifecycle:%d session:%d artifacts:%d, want all zero", fixture.secondLifecycle.stopCalls, len(fixture.secondSessions.closeCalls), fixture.secondArtifactsClosed)
	}
}

func (fixture *btrcConcurrentTargetFixture) assertReplacementStillActive(t *testing.T) {
	t.Helper()
	if fixture.replacementLifecycle.stopCalls != 0 || fixture.replacementArtifactsClosed != 0 {
		t.Fatalf("replacement cleanup after repeated cancel = lifecycle:%d artifacts:%d, want all zero", fixture.replacementLifecycle.stopCalls, fixture.replacementArtifactsClosed)
	}
}

func awaitBTRCTargetInvocations(t *testing.T, firstDone, secondDone <-chan btrcTargetInvocationOutcome) (btrcTargetInvocationOutcome, btrcTargetInvocationOutcome) {
	t.Helper()
	var first, second btrcTargetInvocationOutcome
	select {
	case first = <-firstDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for canceled first target result")
	}
	select {
	case second = <-secondDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for completed second target result")
	}
	return first, second
}

func (fixture *btrcConcurrentTargetFixture) assertInvocationOutcomes(t *testing.T, first, second factorysessions.AsyncStartResult, firstOutcome, secondOutcome btrcTargetInvocationOutcome) {
	t.Helper()
	if firstOutcome.err != nil || firstOutcome.result.Status != factorysessions.InvocationTerminalStatusCanceled || firstOutcome.result.ErrorCode != string(factorysessions.InvocationErrorCodeCanceled) || firstOutcome.result.RequestID != fixture.firstRequestID || firstOutcome.result.SessionID != first.SessionID {
		t.Fatalf("first invocation outcome = (%+v, %v), want one canceled result scoped to first", firstOutcome.result, firstOutcome.err)
	}
	if secondOutcome.err != nil || secondOutcome.result.Status != factorysessions.InvocationTerminalStatusCompleted || secondOutcome.result.RequestID != fixture.secondRequestID || secondOutcome.result.SessionID != second.SessionID {
		t.Fatalf("second invocation outcome = (%+v, %v), want one completed result scoped to second", secondOutcome.result, secondOutcome.err)
	}
}

func (fixture *btrcConcurrentTargetFixture) assertCanceledCleanup(t *testing.T) {
	t.Helper()
	if fixture.firstLifecycle.stopCalls != 1 || fixture.firstArtifactsClosed != 1 || len(fixture.firstSessions.closeCalls) != 1 {
		t.Fatalf("first canceled runtime cleanup = lifecycle:%d session:%d artifacts:%d, want exactly one each", fixture.firstLifecycle.stopCalls, len(fixture.firstSessions.closeCalls), fixture.firstArtifactsClosed)
	}
}

func (fixture *btrcConcurrentTargetFixture) closeTargets(t *testing.T, firstID, secondID string) {
	t.Helper()
	if err := fixture.svc.CloseFactorySession(t.Context(), firstID); err != nil {
		t.Fatalf("CloseFactorySession(first) error = %v", err)
	}
	if err := fixture.svc.CloseFactorySession(t.Context(), firstID); err != nil {
		t.Fatalf("repeated CloseFactorySession(first) error = %v", err)
	}
	if err := fixture.svc.CloseFactorySession(t.Context(), secondID); err != nil {
		t.Fatalf("CloseFactorySession(second) error = %v", err)
	}
	if err := fixture.svc.CloseFactorySession(t.Context(), secondID); err != nil {
		t.Fatalf("repeated CloseFactorySession(second) error = %v", err)
	}
}

func (fixture *btrcConcurrentTargetFixture) assertFinalCleanup(t *testing.T) {
	t.Helper()
	if fixture.replacementLifecycle.stopCalls != 1 || fixture.replacementArtifactsClosed != 1 || len(fixture.replacementSessions.closeCalls) != 1 {
		t.Fatalf("replacement runtime cleanup = lifecycle:%d session:%d artifacts:%d, want exactly one each", fixture.replacementLifecycle.stopCalls, len(fixture.replacementSessions.closeCalls), fixture.replacementArtifactsClosed)
	}
	if fixture.secondLifecycle.stopCalls != 1 || fixture.secondArtifactsClosed != 1 || len(fixture.secondSessions.closeCalls) != 1 {
		t.Fatalf("second runtime cleanup = lifecycle:%d session:%d artifacts:%d, want exactly one each", fixture.secondLifecycle.stopCalls, len(fixture.secondSessions.closeCalls), fixture.secondArtifactsClosed)
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

// TestCloseFactorySessionFailureRetainsCapturedTargetForRetry proves a failed
// close keeps the exact opaque target tracked and retries only the unfinished
// downstream close. The second request is therefore a real retry, never an
// idempotent success for a target that was silently discarded after failure.
func TestCloseFactorySessionFailureRetainsCapturedTargetForRetry(t *testing.T) {
	closeFailure := errors.New("close provider session failed")
	sessions := &fakeSessions{closeErrs: []error{closeFailure, nil}}
	lifecycle := &fakeLifecycle{}
	opener := &fakeOpener{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: lifecycle}}
	svc := newTestService(t, opener, func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}
	if err := svc.CloseFactorySession(context.Background(), started.SessionID); !errors.Is(err, closeFailure) {
		t.Fatalf("first CloseFactorySession() error = %v, want close failure", err)
	}
	if _, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); err == nil {
		t.Fatal("InvokeFactorySession() after failed close error = nil, want the retained target to reject new work while closing")
	}
	if len(sessions.closeCalls) != 1 || lifecycle.stopCalls != 1 {
		t.Fatalf("first close effects = (sessions %d, lifecycle %d), want (1, 1)", len(sessions.closeCalls), lifecycle.stopCalls)
	}

	if err := svc.CloseFactorySession(context.Background(), started.SessionID); err != nil {
		t.Fatalf("retry CloseFactorySession() error = %v", err)
	}
	if len(sessions.closeCalls) != 2 || lifecycle.stopCalls != 1 {
		t.Fatalf("retry close effects = (sessions %d, lifecycle %d), want (2, 1): retry only the failed session close", len(sessions.closeCalls), lifecycle.stopCalls)
	}
	if _, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("InvokeFactorySession() after successful retry error = %v, want ErrSessionNotFound", err)
	}
}

// TestCancelFailureRetainsCapturedTargetForSameIdentityRetry proves a failed
// cancel neither reports the interrupted prompt as canceled nor loses its
// target behind a no-op. Redelivery of the same immutable control can finish
// cleanup, replace the runtime, and only then publish cancellation.
func TestCancelFailureRetainsCapturedTargetForSameIdentityRetry(t *testing.T) {
	startedSignal := make(chan struct{})
	closeFailure := errors.New("close provider session failed")
	sessions := &fakeSessions{
		closeErrs: []error{closeFailure, nil},
		invoke: func(ctx context.Context, _ string, _ factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
			close(startedSignal)
			<-ctx.Done()
			return factorysessions.InvocationResult{}, ctx.Err()
		},
	}
	replacementSessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	opener := &sequencedOpener{results: []openResult{
		{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: &fakeLifecycle{}}},
		{opened: roles.OpenedInvocationRuntime{Sessions: replacementSessions, Lifecycle: &fakeLifecycle{}}},
	}}
	svc := newTestService(t, opener, func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}
	invokeResult := make(chan error, 1)
	go func() {
		_, invokeErr := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{})
		invokeResult <- invokeErr
	}()
	<-startedSignal

	control := factorysessions.ControlRequest{RequestID: "cancel-retry"}
	if _, err := svc.Cancel(context.Background(), started.SessionID, control); !errors.Is(err, closeFailure) {
		t.Fatalf("first Cancel() error = %v, want close failure", err)
	}
	if err := <-invokeResult; !errors.Is(err, errCancellationIncomplete) {
		t.Fatalf("interrupted InvokeFactorySession() error = %v, want incomplete-cancel failure rather than a false canceled result", err)
	}
	if len(sessions.closeCalls) != 1 || len(opener.calls) != 1 {
		t.Fatalf("first cancel effects = (close %d, opens %d), want (1, 1)", len(sessions.closeCalls), len(opener.calls))
	}

	retried, err := svc.Cancel(context.Background(), started.SessionID, control)
	if err != nil {
		t.Fatalf("retry Cancel() error = %v", err)
	}
	if retried.Outcome != factorysessions.LifecycleControlOutcomeAccepted {
		t.Fatalf("retry Cancel() outcome = %q, want ACCEPTED", retried.Outcome)
	}
	if len(sessions.closeCalls) != 2 || len(opener.calls) != 2 {
		t.Fatalf("retry cancel effects = (close %d, opens %d), want (2, 2)", len(sessions.closeCalls), len(opener.calls))
	}
	if result, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); err != nil || result.Status != factorysessions.InvocationTerminalStatusCompleted {
		t.Fatalf("InvokeFactorySession() after successful retry = (%+v, %v), want replacement completion", result, err)
	}
}

// TestConcurrentCancelAndCloseLeavesNoReplacementRuntime proves lifecycle
// control is serialized by opaque target identity, rather than only by the
// activation each caller observed. Close waits for Cancel's replacement and
// then closes that replacement, so success never leaves a live successor
// behind the captured Factory Session ID.
func TestConcurrentCancelAndCloseLeavesNoReplacementRuntime(t *testing.T) {
	invocationStarted := make(chan struct{})
	replacementOpening := make(chan struct{})
	releaseReplacement := make(chan struct{})
	originalSessions := &fakeSessions{invoke: func(ctx context.Context, _ string, _ factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
		close(invocationStarted)
		<-ctx.Done()
		return factorysessions.InvocationResult{}, ctx.Err()
	}}
	originalLifecycle := &fakeLifecycle{}
	replacementLifecycle := &fakeLifecycle{}
	var openerMu sync.Mutex
	openCount := 0
	opener := &funcOpener{open: func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedInvocationRuntime, error) {
		openerMu.Lock()
		openCount++
		call := openCount
		openerMu.Unlock()
		if call == 1 {
			return roles.OpenedInvocationRuntime{Sessions: originalSessions, Lifecycle: originalLifecycle}, nil
		}
		if call == 2 {
			close(replacementOpening)
			<-releaseReplacement
			return roles.OpenedInvocationRuntime{Sessions: &fakeSessions{}, Lifecycle: replacementLifecycle}, nil
		}
		return roles.OpenedInvocationRuntime{}, fmt.Errorf("unexpected runtime open %d", call)
	}}
	svc := newTestService(t, opener, func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}
	invokeDone := make(chan error, 1)
	go func() {
		_, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{})
		invokeDone <- err
	}()
	select {
	case <-invocationStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("invocation did not start")
	}

	cancelDone := make(chan error, 1)
	go func() {
		_, err := svc.Cancel(context.Background(), started.SessionID, factorysessions.ControlRequest{RequestID: "cancel"})
		cancelDone <- err
	}()
	select {
	case <-replacementOpening:
	case <-time.After(5 * time.Second):
		t.Fatal("Cancel did not reach replacement opening")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- svc.CloseFactorySession(context.Background(), started.SessionID) }()
	close(releaseReplacement)

	if err := <-cancelDone; err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("CloseFactorySession() error = %v", err)
	}
	if err := <-invokeDone; err != nil {
		t.Fatalf("interrupted InvokeFactorySession() error = %v, want canceled result", err)
	}
	if originalLifecycle.stopCalls != 1 || replacementLifecycle.stopCalls != 1 {
		t.Fatalf("StopLifecycle calls = (%d, %d), want both original and replacement closed once", originalLifecycle.stopCalls, replacementLifecycle.stopCalls)
	}
	openerMu.Lock()
	gotOpenCount := openCount
	openerMu.Unlock()
	if gotOpenCount != 2 {
		t.Fatalf("OpenInvocationRuntime calls = %d, want original plus one replacement", gotOpenCount)
	}
	if _, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("InvokeFactorySession() after successful close error = %v, want ErrSessionNotFound", err)
	}
}

// TestCancelRespectsCallerCancellationWhileInvocationIgnoresCancellation
// proves a non-cooperative provider cannot make control handling wait
// forever: the caller's context remains live in cancelInvocation.
func TestCancelRespectsCallerCancellationWhileInvocationIgnoresCancellation(t *testing.T) {
	invocationStarted := make(chan struct{})
	providerCanceled := make(chan struct{})
	releaseInvocation := make(chan struct{})
	sessions := &fakeSessions{invoke: func(ctx context.Context, _ string, _ factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
		close(invocationStarted)
		<-ctx.Done()
		close(providerCanceled)
		<-releaseInvocation
		return factorysessions.InvocationResult{}, ctx.Err()
	}}
	svc := newTestService(t, &fakeOpener{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: &fakeLifecycle{}}}, func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}, sequentialIDs("wrapper"))
	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}
	invokeDone := make(chan error, 1)
	go func() {
		_, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{})
		invokeDone <- err
	}()
	select {
	case <-invocationStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("invocation did not start")
	}

	controlContext, cancelControl := context.WithCancel(context.Background())
	cancelDone := make(chan error, 1)
	go func() {
		_, err := svc.Cancel(controlContext, started.SessionID, factorysessions.ControlRequest{RequestID: "cancel"})
		cancelDone <- err
	}()
	select {
	case <-providerCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("Cancel did not signal the provider invocation")
	}
	cancelControl()
	select {
	case err := <-cancelDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Cancel() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Cancel did not honor the caller cancellation")
	}

	close(releaseInvocation)
	if err := <-invokeDone; !errors.Is(err, errCancellationIncomplete) {
		t.Fatalf("InvokeFactorySession() error = %v, want incomplete cancellation after failed owner control", err)
	}
}

// TestCancelRespectsCallerCancellationDuringCleanupAndRetries proves a
// synchronous cleanup collaborator cannot outlive the caller's bound. The
// failed target remains mapped so the same captured control can complete only
// its unfinished cleanup and then publish a replacement runtime.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestCancelRespectsCallerCancellationDuringCleanupAndRetries(t *testing.T) {
	invocationStarted := make(chan struct{})
	cleanupStarted := make(chan struct{})
	cleanupAttempts := 0
	originalSessions := &fakeSessions{
		invoke: func(ctx context.Context, _ string, _ factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
			close(invocationStarted)
			<-ctx.Done()
			return factorysessions.InvocationResult{}, ctx.Err()
		},
		close: func(ctx context.Context, _ string) error {
			cleanupAttempts++
			if cleanupAttempts == 1 {
				close(cleanupStarted)
				<-ctx.Done()
				return ctx.Err()
			}
			return nil
		},
	}
	replacementSessions := &fakeSessions{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	originalLifecycle := &fakeLifecycle{}
	opener := &sequencedOpener{results: []openResult{
		{opened: roles.OpenedInvocationRuntime{Sessions: originalSessions, Lifecycle: originalLifecycle}},
		{opened: roles.OpenedInvocationRuntime{Sessions: replacementSessions, Lifecycle: &fakeLifecycle{}}},
	}}
	svc := newTestService(t, opener, func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}
	invokeDone := make(chan error, 1)
	go func() {
		_, invokeErr := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{})
		invokeDone <- invokeErr
	}()
	select {
	case <-invocationStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("invocation did not start")
	}

	controlContext, cancelControl := context.WithCancel(context.Background())
	cancelDone := make(chan error, 1)
	control := factorysessions.ControlRequest{RequestID: "cancel-cleanup-retry"}
	go func() {
		_, cancelErr := svc.Cancel(controlContext, started.SessionID, control)
		cancelDone <- cancelErr
	}()
	select {
	case <-cleanupStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Cancel did not reach cleanup")
	}
	cancelControl()
	select {
	case err := <-cancelDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Cancel() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Cancel did not honor caller cancellation during cleanup")
	}
	if err := <-invokeDone; !errors.Is(err, errCancellationIncomplete) {
		t.Fatalf("interrupted InvokeFactorySession() error = %v, want incomplete cancellation", err)
	}
	if len(originalSessions.closeCalls) != 1 || originalLifecycle.stopCalls != 1 {
		t.Fatalf("failed cancel cleanup effects = (close %d, stop %d), want (1, 1)", len(originalSessions.closeCalls), originalLifecycle.stopCalls)
	}
	if _, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); err == nil {
		t.Fatal("InvokeFactorySession() after canceled cleanup error = nil, want retained closing target to reject work")
	}

	result, err := svc.Cancel(context.Background(), started.SessionID, control)
	if err != nil {
		t.Fatalf("retry Cancel() error = %v", err)
	}
	if result.Outcome != factorysessions.LifecycleControlOutcomeAccepted {
		t.Fatalf("retry Cancel() outcome = %q, want ACCEPTED", result.Outcome)
	}
	if len(originalSessions.closeCalls) != 2 || originalLifecycle.stopCalls != 1 || len(opener.calls) != 2 {
		t.Fatalf("retry cancel effects = (close %d, stop %d, opens %d), want (2, 1, 2)", len(originalSessions.closeCalls), originalLifecycle.stopCalls, len(opener.calls))
	}
}

// TestCloseRespectsCallerCancellationDuringCleanupAndRetries proves close
// returns when its downstream cleanup observes caller cancellation, retains
// the exact target, and lets a retry finish only the outstanding cleanup.
func TestCloseRespectsCallerCancellationDuringCleanupAndRetries(t *testing.T) {
	cleanupStarted := make(chan struct{})
	cleanupAttempts := 0
	sessions := &fakeSessions{close: func(ctx context.Context, _ string) error {
		cleanupAttempts++
		if cleanupAttempts == 1 {
			close(cleanupStarted)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}}
	lifecycle := &fakeLifecycle{}
	svc := newTestService(t, &fakeOpener{opened: roles.OpenedInvocationRuntime{Sessions: sessions, Lifecycle: lifecycle}}, func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
		return factorysessions.RuntimeOpeningRequest{}, nil
	}, sequentialIDs("wrapper"))

	started, err := svc.StartAsync(context.Background(), factorysessions.StartRequest{})
	if err != nil {
		t.Fatalf("StartAsync() error = %v", err)
	}
	closeContext, cancelClose := context.WithCancel(context.Background())
	closeDone := make(chan error, 1)
	go func() { closeDone <- svc.CloseFactorySession(closeContext, started.SessionID) }()
	select {
	case <-cleanupStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("CloseFactorySession did not reach cleanup")
	}
	cancelClose()
	select {
	case err := <-closeDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("CloseFactorySession() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CloseFactorySession did not honor caller cancellation during cleanup")
	}
	if len(sessions.closeCalls) != 1 || lifecycle.stopCalls != 1 {
		t.Fatalf("failed close cleanup effects = (close %d, stop %d), want (1, 1)", len(sessions.closeCalls), lifecycle.stopCalls)
	}
	if _, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); err == nil {
		t.Fatal("InvokeFactorySession() after canceled cleanup error = nil, want retained closing target to reject work")
	}

	if err := svc.CloseFactorySession(context.Background(), started.SessionID); err != nil {
		t.Fatalf("retry CloseFactorySession() error = %v", err)
	}
	if len(sessions.closeCalls) != 2 || lifecycle.stopCalls != 1 {
		t.Fatalf("retry close effects = (close %d, stop %d), want (2, 1)", len(sessions.closeCalls), lifecycle.stopCalls)
	}
	if _, err := svc.InvokeFactorySession(context.Background(), started.SessionID, factorysessions.InvocationRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("InvokeFactorySession() after successful retry error = %v, want ErrSessionNotFound", err)
	}
}

// TestCloseFactorySessionUnknownIdentityIsNoOpSuccess proves closing an
// unknown or already-closed identity succeeds without effect, matching
// factorysessions.LiveControlService.CloseFactorySession's documented idempotent
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
	opener := &funcOpener{open: func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedInvocationRuntime, error) {
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
// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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
// start a target, invoke and cancel its captured session, observe its
// response and canonical Factory-event streams, and close it,
// observing the exact same behavior (including post-close
// ErrSessionNotFound) as a caller holding the concrete type. This is the
// behavioral proof for the newly extended capability's publication: an ACP
// caller can obtain the canonical Factory Event stream without downcasting
// to the concrete service.
func TestServiceSatisfiesPublishedTargetExecutionCapability(t *testing.T) {
	wantFactoryEvents := &factorydefinitions.FactoryEventStream{FactorySessionID: factorysessions.DefaultSessionID}
	sessions := &fakeSessions{
		invokeResult:       factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted},
		factoryEventStream: wantFactoryEvents,
	}
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

	stream, err := client.SubscribeFactoryEventsForSession(context.Background(), started.SessionID, nil)
	if err != nil || stream != wantFactoryEvents {
		t.Fatalf("SubscribeFactoryEventsForSession() = (%#v, %v), want captured stream and nil", stream, err)
	}

	if err := client.TerminateFactorySession(context.Background(), started.SessionID, factorysessions.ControlRequest{}); err != nil {
		t.Fatalf("TerminateFactorySession() via TargetExecutionService error = %v", err)
	}
	if err := client.CloseFactorySession(context.Background(), started.SessionID); err != nil {
		t.Fatalf("CloseFactorySession() after target terminate via TargetExecutionService error = %v, want idempotent success", err)
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

// funcOpener is an invocation runtime-opening test double backed directly by a
// function, for tests that need each successive OpenInvocationRuntime call
// to return a distinct opened runtime (fakeOpener always returns the same
// fixed one).
type funcOpener struct {
	open func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedInvocationRuntime, error)
}

func (f *funcOpener) OpenInvocationRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedInvocationRuntime, error) {
	return f.open(ctx, request)
}
