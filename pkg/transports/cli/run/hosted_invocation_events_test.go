package run

import (
	"context"
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

func TestHostedInvocationOperationForwardsDetachedCapabilities(t *testing.T) {
	delegate := testInvocationOperation{}
	operation := &hostedInvocationOperation{delegate: delegate}

	if got, err := operation.ResolveModelInvocationFactoryDir("factory"); err != nil || got != "factory" {
		t.Fatalf("ResolveModelInvocationFactoryDir() = (%q, %v), want factory", got, err)
	}
	if err := operation.ExportModelInvocationArtifact("source", "destination"); err == nil {
		t.Fatal("ExportModelInvocationArtifact() error = nil")
	}
	if _, err := operation.InvokeModel(t.Context(), factorysessions.InvocationTarget{}, "model", models.Request{}); err == nil {
		t.Fatal("InvokeModel() error = nil")
	}
}

func TestHostedInvocationOperationUsesDelegateForJavaScript(t *testing.T) {
	called := false
	service := &hostedInvocationSessionsStub{
		projection: factorysessions.SessionProjection{Context: factorysessions.ProjectionContext{
			FactoryCfg: &factorydefinitions.FactoryConfig{
				Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
					Kind: factorydefinitions.OrchestratorKindJavaScript,
				},
			},
		}},
	}
	delegate := testInvocationOperation{invokeFactory: func(
		context.Context,
		factorysessions.InvocationTarget,
		factorysessions.InvocationRequest,
		factorysessions.FactoryEventConsumer,
	) (factorysessions.FactoryInvocationOutcome, error) {
		called = true
		return factorysessions.FactoryInvocationOutcome{
			Result: factorydefinitions.FactoryInvocationResult{
				Status: factorydefinitions.InvocationTerminalStatusCompleted,
			},
		}, nil
	}}
	operation := &hostedInvocationOperation{
		delegate: delegate,
		hosted: &factorysessions.HostedLiveInvocation{
			Sessions: service,
			Invoker:  service,
		},
	}

	outcome, err := operation.InvokeFactory(
		t.Context(), factorysessions.InvocationTarget{}, factorysessions.InvocationRequest{}, nil,
	)
	if err != nil {
		t.Fatalf("InvokeFactory() error = %v", err)
	}
	if !called || outcome.Result.Status != factorydefinitions.InvocationTerminalStatusCompleted {
		t.Fatalf("delegate called = %t, outcome = %#v", called, outcome)
	}
	if service.invokeCalls != 0 {
		t.Fatalf("hosted session invocation calls = %d, want 0", service.invokeCalls)
	}
}

func TestHostedInvocationOperationPublishesAndReconcilesEvents(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		sessionID   string
		wantDurable bool
	}{
		{name: "default session"},
		{name: "durable session", sessionID: "session-42", wantDurable: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			live := make(chan factorydefinitions.FactoryEvent, 1)
			live <- hostedInvocationFactoryEvent("live")
			close(live)
			service := &hostedInvocationSessionsStub{
				liveStreams: []*factorydefinitions.FactoryEventStream{
					{History: []factorydefinitions.FactoryEvent{hostedInvocationFactoryEvent("history")}, Events: live},
					{History: []factorydefinitions.FactoryEvent{
						hostedInvocationFactoryEvent("history"), hostedInvocationFactoryEvent("live"), hostedInvocationFactoryEvent("final"),
					}},
				},
				durableStream: &factorydefinitions.FactoryEventStream{History: []factorydefinitions.FactoryEvent{
					hostedInvocationFactoryEvent("history"), hostedInvocationFactoryEvent("live"), hostedInvocationFactoryEvent("final"),
				}},
				invokeResult: factorysessions.InvocationResult{
					RequestID: "request-1", SessionID: testCase.sessionID,
					Status: factorysessions.InvocationTerminalStatusCompleted,
				},
			}
			presented := make(chan []factorydefinitions.FactoryEvent, 4)
			operation := &hostedInvocationOperation{
				delegate: testInvocationOperation{},
				hosted: &factorysessions.HostedLiveInvocation{
					Sessions: service,
					Invoker:  service,
				},
			}

			outcome, err := operation.InvokeFactory(
				t.Context(), factorysessions.InvocationTarget{}, factorysessions.InvocationRequest{},
				func(events []factorydefinitions.FactoryEvent) { presented <- events },
			)
			if err != nil {
				t.Fatalf("InvokeFactory() error = %v", err)
			}
			if outcome.Result.RequestID != "request-1" || service.invokeCalls != 1 {
				t.Fatalf("outcome = %#v, invoke calls = %d", outcome, service.invokeCalls)
			}
			if testCase.wantDurable != (service.durableCalls == 1) {
				t.Fatalf("durable calls = %d, want durable=%t", service.durableCalls, testCase.wantDurable)
			}
			if !testCase.wantDurable && service.liveCalls != 2 {
				t.Fatalf("live calls = %d, want 2", service.liveCalls)
			}
			assertHostedInvocationEventIDs(t, presented, []string{"history", "live", "final"})
		})
	}
}

func TestHostedInvocationOperationRejectsIncompleteEventBridges(t *testing.T) {
	tests := []struct {
		name      string
		operation *hostedInvocationOperation
		wantError string
	}{
		{name: "nil operation", wantError: "hosted invocation operation is required"},
		{name: "nil delegate", operation: &hostedInvocationOperation{}, wantError: "hosted invocation operation is required"},
		{name: "nil hosted runtime", operation: &hostedInvocationOperation{delegate: testInvocationOperation{}}, wantError: "hosted live invocation runtime is incomplete"},
		{name: "nil sessions", operation: &hostedInvocationOperation{delegate: testInvocationOperation{}, hosted: &factorysessions.HostedLiveInvocation{}}, wantError: "hosted live invocation runtime is incomplete"},
		{name: "nil invoker", operation: &hostedInvocationOperation{delegate: testInvocationOperation{}, hosted: &factorysessions.HostedLiveInvocation{Sessions: &hostedInvocationSessionsStub{}}}, wantError: "hosted live invocation runtime is incomplete"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			operation := testCase.operation
			_, err := operation.InvokeFactory(t.Context(), factorysessions.InvocationTarget{}, factorysessions.InvocationRequest{}, nil)
			if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("InvokeFactory() error = %v, want %q", err, testCase.wantError)
			}
		})
	}
}

func TestHostedInvocationOperationReportsEventBridgeFailures(t *testing.T) {
	tests := []struct {
		name       string
		service    *hostedInvocationSessionsStub
		result     factorysessions.InvocationResult
		wantError  string
		wantResult bool
	}{
		{
			name:      "subscribe fails",
			service:   &hostedInvocationSessionsStub{subscribeErr: errors.New("subscribe failed")},
			wantError: "subscribe invocation Factory Events: subscribe failed",
		},
		{
			name:      "stream is nil",
			service:   &hostedInvocationSessionsStub{liveStreams: []*factorydefinitions.FactoryEventStream{nil}},
			wantError: "subscribe invocation Factory Events: stream is unavailable",
		},
		{
			name: "terminal result tolerates reconciliation failure",
			service: &hostedInvocationSessionsStub{
				liveStreams:     []*factorydefinitions.FactoryEventStream{{Events: closedFactoryInvocationEvents()}},
				subscribeErrors: []error{nil, errors.New("reconcile failed")},
			},
			result:     factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted},
			wantResult: true,
		},
		{
			name: "non-terminal result preserves reconciliation failure",
			service: &hostedInvocationSessionsStub{
				liveStreams:     []*factorydefinitions.FactoryEventStream{{Events: closedFactoryInvocationEvents()}},
				subscribeErrors: []error{nil, errors.New("reconcile failed")},
			},
			wantError: "read invocation Factory Events: reconcile failed",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.service.invokeResult = testCase.result
			operation := &hostedInvocationOperation{
				delegate: testInvocationOperation{},
				hosted: &factorysessions.HostedLiveInvocation{
					Sessions: testCase.service,
					Invoker:  testCase.service,
				},
			}
			outcome, err := operation.InvokeFactory(
				t.Context(), factorysessions.InvocationTarget{}, factorysessions.InvocationRequest{},
				func([]factorydefinitions.FactoryEvent) {},
			)
			if testCase.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
					t.Fatalf("InvokeFactory() error = %v, want %q", err, testCase.wantError)
				}
				return
			}
			if err != nil || !testCase.wantResult || outcome.Result.Status != factorydefinitions.InvocationTerminalStatusCompleted {
				t.Fatalf("InvokeFactory() = (%#v, %v), want completed result", outcome, err)
			}
		})
	}
}

type hostedInvocationSessionsStub struct {
	factorysessions.Service
	projection      factorysessions.SessionProjection
	projectionErr   error
	subscribeErr    error
	subscribeErrors []error
	liveStreams     []*factorydefinitions.FactoryEventStream
	durableStream   *factorydefinitions.FactoryEventStream
	durableErr      error
	invokeResult    factorysessions.InvocationResult
	invokeErr       error
	liveCalls       int
	durableCalls    int
	invokeCalls     int
}

var _ factorysessions.Service = (*hostedInvocationSessionsStub)(nil)

func (stub *hostedInvocationSessionsStub) GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error) {
	return stub.projection, stub.projectionErr
}

func (stub *hostedInvocationSessionsStub) InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	stub.invokeCalls++
	return stub.invokeResult, stub.invokeErr
}

func (stub *hostedInvocationSessionsStub) SubscribeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
	stub.liveCalls++
	if len(stub.subscribeErrors) >= stub.liveCalls {
		if err := stub.subscribeErrors[stub.liveCalls-1]; err != nil {
			return nil, err
		}
	}
	if stub.subscribeErr != nil {
		return nil, stub.subscribeErr
	}
	if len(stub.liveStreams) == 0 {
		return nil, errors.New("live stream not configured")
	}
	index := stub.liveCalls - 1
	if index >= len(stub.liveStreams) {
		index = len(stub.liveStreams) - 1
	}
	return stub.liveStreams[index], nil
}

func (stub *hostedInvocationSessionsStub) ReadDurableFactorySessionEventStream(context.Context, string, factorysessions.EventReconnectRequest) (*factorydefinitions.FactoryEventStream, error) {
	stub.durableCalls++
	return stub.durableStream, stub.durableErr
}

func hostedInvocationFactoryEvent(id string) factorydefinitions.FactoryEvent {
	return factorydefinitions.FactoryEvent{Id: id}
}

func closedFactoryInvocationEvents() <-chan factorydefinitions.FactoryEvent {
	events := make(chan factorydefinitions.FactoryEvent)
	close(events)
	return events
}

func assertHostedInvocationEventIDs(t *testing.T, presented <-chan []factorydefinitions.FactoryEvent, want []string) {
	t.Helper()
	got := make([]string, 0, len(want))
	for len(got) < len(want) {
		batch := <-presented
		for _, event := range batch {
			got = append(got, event.Id)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("presented event IDs = %v, want %v", got, want)
	}
}
