package invocation

import (
	"context"
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

type hostedLiveSessionsFake struct {
	executionMethodsStub
	sessions     map[string]factorysessions.SessionProjection
	invokeResult factorysessions.InvocationResult
	invokeErr    error
}

func newHostedLiveSessionsFake(projection factorysessions.SessionProjection) *hostedLiveSessionsFake {
	return &hostedLiveSessionsFake{
		sessions: map[string]factorysessions.SessionProjection{
			factorysessions.DefaultSessionID: projection,
		},
	}
}

var _ factorysessions.Service = (*hostedLiveSessionsFake)(nil)

func (fake *hostedLiveSessionsFake) ForRuntime(factorysessions.OpeningBindingRequest) (factorysessions.Service, error) {
	return fake, nil
}

func (fake *hostedLiveSessionsFake) ActivateNamedFactory(context.Context, string) error {
	return factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	return fake.invokeResult, fake.invokeErr
}

func (fake *hostedLiveSessionsFake) OpenFactorySession(context.Context, factorysessions.OpenRequest) (*factorysessions.OpenResult, error) {
	return &factorysessions.OpenResult{SessionID: factorysessions.DefaultSessionID}, nil
}

func (fake *hostedLiveSessionsFake) OpenFactorySessionFromFolder(context.Context, string, *factorysessions.TargetRef, bool, bool) (*factorysessions.OpenResult, error) {
	return &factorysessions.OpenResult{SessionID: factorysessions.DefaultSessionID}, nil
}

func (fake *hostedLiveSessionsFake) ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error) {
	return nil, nil
}

func (fake *hostedLiveSessionsFake) GetFactorySession(_ context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	if projection, ok := fake.sessions[sessionID]; ok {
		return projection, nil
	}
	return factorysessions.SessionProjection{}, factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) GetFactorySessionSyncPreflight(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor, *factorydefinitions.FactorySessionLogicalResolveHint) (factorysessions.SyncPreflightResult, error) {
	return factorysessions.SyncPreflightResult{}, factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) GetFactorySessionResult(context.Context, string) (factoryruntime.LiveSessionResult, error) {
	return factoryruntime.LiveSessionResult{}, factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) GetFactorySessionPartialResult(context.Context, string) (factoryruntime.PartialSessionResult, error) {
	return factoryruntime.PartialSessionResult{}, factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) SubscribeFactoryResponseEvents(context.Context, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	return nil, factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) SubscribeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
	return nil, factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) ProbeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) error {
	return factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) ReadDurableFactorySessionEventStream(context.Context, string, factorysessions.EventReconnectRequest) (*factorydefinitions.FactoryEventStream, error) {
	return nil, factorysessions.ErrDurableSessionNotFound
}

func (fake *hostedLiveSessionsFake) ProbeDurableFactorySessionEvents(context.Context, string, factorysessions.EventReconnectRequest) error {
	return factorysessions.ErrDurableSessionNotFound
}

func (fake *hostedLiveSessionsFake) ObserveForSession(context.Context, string, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) PauseLiveFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) ResumeLiveFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) CloseFactorySession(context.Context, string) error {
	return factorysessions.ErrSessionNotFound
}

func (fake *hostedLiveSessionsFake) PauseDurableFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *hostedLiveSessionsFake) ResumeDurableFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *hostedLiveSessionsFake) CancelDurableFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *hostedLiveSessionsFake) TerminateDurableFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *hostedLiveSessionsFake) ApproveDurableFactorySession(context.Context, string, factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *hostedLiveSessionsFake) RetryDurableFactorySessionDispatch(context.Context, string, factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *hostedLiveSessionsFake) InterruptDurableFactorySessionDispatch(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

type executionMethodsStub struct{}

func (executionMethodsStub) StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	return factorysessions.AsyncStartResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) StartSync(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	return factorysessions.SyncStartResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) ResumeInterruptedSession(context.Context, string, factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error) {
	return factorysessions.AsyncStartResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) GetSession(context.Context, string) (factorysessions.SessionReadResult, error) {
	return factorysessions.SessionReadResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) Pause(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) Resume(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) Cancel(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) Terminate(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) Approve(context.Context, string, factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) RetryDispatch(context.Context, string, factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) InterruptDispatch(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) GetResult(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	return factorysessions.ResultReadResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) ListDispatches(context.Context, string) (factorysessions.ListDispatchesResult, error) {
	return factorysessions.ListDispatchesResult{}, nil
}

func (executionMethodsStub) QueryDispatches(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
	return factorysessions.ListDispatchesResult{}, nil
}

func (executionMethodsStub) GetDispatch(context.Context, string, string) (factorysessions.DispatchDetail, error) {
	return factorysessions.DispatchDetail{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) ListArtifacts(context.Context, string) (factorysessions.ListArtifactsResult, error) {
	return factorysessions.ListArtifactsResult{}, nil
}

func (executionMethodsStub) GetArtifact(context.Context, string, string) (factorysessions.ArtifactDetail, error) {
	return factorysessions.ArtifactDetail{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) ReadEvents(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
	return factorysessions.EventReadResult{}, factorysessions.ErrDurableSessionNotFound
}

func (executionMethodsStub) ListSessions(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	return factorysessions.ListSessionsResult{}, nil
}

type hostedInvokerFake struct {
	result factorydefinitions.FactoryInvocationResult
	err    error
}

func (fake *hostedInvokerFake) InvokeFactorySession(
	_ context.Context,
	sessionID string,
	_ factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	if sessionID != factorysessions.DefaultSessionID {
		return factorydefinitions.FactoryInvocationResult{}, factorysessions.ErrSessionNotFound
	}
	return fake.result, fake.err
}

func TestInvokeFactoryRejectsIncompleteHostedLiveInvocation(t *testing.T) {
	t.Parallel()

	op := &operation{}
	_, err := op.InvokeFactory(
		context.Background(),
		roles.InvocationTarget{
			HostedLiveInvocation: &factorysessions.HostedLiveInvocation{
				Sessions: newHostedLiveSessionsFake(factorysessions.SessionProjection{}),
			},
		},
		factorysessions.InvocationRequest{},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "hosted live invocation runtime is incomplete") {
		t.Fatalf("InvokeFactory() error = %v, want incomplete hosted runtime", err)
	}
}

func TestInvokeFactoryUsesHostedLiveRuntimeForPetriFactory(t *testing.T) {
	t.Parallel()

	petriProjection := factorysessions.SessionProjection{
		Context: factorysessions.ProjectionContext{
			FactoryCfg: &factorydefinitions.FactoryConfig{
				Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
					Kind: factorydefinitions.OrchestratorKindPetri,
				},
			},
		},
	}
	wantResult := factorydefinitions.FactoryInvocationResult{
		Status: factorydefinitions.InvocationTerminalStatusCompleted,
	}
	sessions := newHostedLiveSessionsFake(petriProjection)
	invoker := &hostedInvokerFake{result: wantResult}

	op := &operation{}
	outcome, err := op.InvokeFactory(
		context.Background(),
		roles.InvocationTarget{
			HostedLiveInvocation: &factorysessions.HostedLiveInvocation{
				Sessions: sessions,
				Invoker:  invoker,
			},
		},
		factorysessions.InvocationRequest{},
		nil,
	)
	if err != nil {
		t.Fatalf("InvokeFactory() error = %v", err)
	}
	if outcome.Result.Status != wantResult.Status {
		t.Fatalf("result status = %q, want %q", outcome.Result.Status, wantResult.Status)
	}
}

// raceyEventSubscribeSessionsFake subscribes successfully once (used by
// startLiveInvocationFactoryEvents) and fails every subsequent subscribe
// (used by the post-invocation trailing-events read inside finish()),
// simulating a Factory Event stream that becomes unreadable purely because of
// teardown timing after the invocation already produced a terminal result.
type raceyEventSubscribeSessionsFake struct {
	*hostedLiveSessionsFake
	subscribeCalls    int
	subscribeErrAfter error
}

func (fake *raceyEventSubscribeSessionsFake) SubscribeFactoryEventsForSession(
	ctx context.Context,
	sessionID string,
	cursor *factorydefinitions.FactoryEventReconnectCursor,
) (*factorydefinitions.FactoryEventStream, error) {
	fake.subscribeCalls++
	if fake.subscribeCalls > 1 {
		return nil, fake.subscribeErrAfter
	}
	events := make(chan factorydefinitions.FactoryEvent)
	close(events)
	return &factorydefinitions.FactoryEventStream{Events: events}, nil
}

// TestInvokeFactoryHostedLiveRuntimePreservesResultWhenTrailingEventReadFails
// proves a post-result trailing-Factory-Event read failure (a teardown-timing
// race, not an invocation failure) does not erase an already-determined
// terminal invocation result. Regression for the "invocation_result record
// count = 0" flake: InvokeFactory previously joined this kind of post-result
// error unconditionally, which discarded a valid outcome.Result and caused
// callers to skip writing the public terminal record entirely.
func TestInvokeFactoryHostedLiveRuntimePreservesResultWhenTrailingEventReadFails(t *testing.T) {
	t.Parallel()

	petriProjection := factorysessions.SessionProjection{
		Context: factorysessions.ProjectionContext{
			FactoryCfg: &factorydefinitions.FactoryConfig{
				Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
					Kind: factorydefinitions.OrchestratorKindPetri,
				},
			},
		},
	}
	wantResult := factorydefinitions.FactoryInvocationResult{
		Status:    factorydefinitions.InvocationTerminalStatusFailed,
		ErrorCode: "WORK_REJECTED",
		Message:   "deterministic worker rejection",
	}
	sessions := &raceyEventSubscribeSessionsFake{
		hostedLiveSessionsFake: newHostedLiveSessionsFake(petriProjection),
		subscribeErrAfter:      errors.New("event stream unavailable during teardown"),
	}
	invoker := &hostedInvokerFake{result: wantResult}

	op := &operation{}
	outcome, err := op.InvokeFactory(
		context.Background(),
		roles.InvocationTarget{
			HostedLiveInvocation: &factorysessions.HostedLiveInvocation{
				Sessions: sessions,
				Invoker:  invoker,
			},
		},
		factorysessions.InvocationRequest{},
		func([]factorydefinitions.FactoryEvent) {},
	)
	if err != nil {
		t.Fatalf("InvokeFactory() error = %v, want nil (trailing event read failure must not suppress the determined result)", err)
	}
	if outcome.Result.Status != wantResult.Status {
		t.Fatalf("result status = %q, want %q", outcome.Result.Status, wantResult.Status)
	}
	if outcome.Result.ErrorCode != wantResult.ErrorCode {
		t.Fatalf("result error code = %q, want %q", outcome.Result.ErrorCode, wantResult.ErrorCode)
	}
	if sessions.subscribeCalls < 2 {
		t.Fatalf("subscribeCalls = %d, want at least 2 (start + trailing read)", sessions.subscribeCalls)
	}
}

func TestInvokeFactoryHostedLiveRuntimePropagatesInvokerError(t *testing.T) {
	t.Parallel()

	petriProjection := factorysessions.SessionProjection{
		Context: factorysessions.ProjectionContext{
			FactoryCfg: &factorydefinitions.FactoryConfig{
				Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
					Kind: factorydefinitions.OrchestratorKindPetri,
				},
			},
		},
	}
	invokerErr := errors.New("invoke failed")
	invoker := &hostedInvokerFake{err: invokerErr}

	op := &operation{}
	_, err := op.InvokeFactory(
		context.Background(),
		roles.InvocationTarget{
			HostedLiveInvocation: &factorysessions.HostedLiveInvocation{
				Sessions: newHostedLiveSessionsFake(petriProjection),
				Invoker:  invoker,
			},
		},
		factorysessions.InvocationRequest{},
		nil,
	)
	if !errors.Is(err, invokerErr) {
		t.Fatalf("InvokeFactory() error = %v, want %v", err, invokerErr)
	}
}
