package factorysession_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	runningSessionID      = "dur-sess-js-run-n-001"
	successSessionID      = "dur-sess-petri-success-001"
	missingSessionID      = "dur-sess-missing-999"
	internalLeakProbePath = "pkg/services/factory_sessions/internal/sessionstore"
)

var canonicalMCPRequestPreparation mcpfactorysession.RequestPreparation = mcpRequestPreparation{
	start: func(request factorysessions.StartRequest) (factorysessions.StartRequest, error) { return request, nil },
	control: func(request factorysessions.ControlRequest) (factorysessions.ControlRequest, error) {
		return request, nil
	},
	approve: func(request factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error) {
		return request, nil
	},
	retryDispatch: func(request factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error) {
		return request, nil
	},
	interruptDispatch: func(request factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error) {
		return request, nil
	},
	listSessions: func(request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
		if request.Scope == "" {
			request.Scope = factorysessions.SessionListScopeLive
		}
		return request, nil
	},
	result: func(request factorysessions.ResultRequest) (factorysessions.ResultRequest, error) {
		return request, nil
	},
	eventReconnect: func(request factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error) {
		return request, nil
	},
}

type mcpRequestPreparation struct {
	start             func(factorysessions.StartRequest) (factorysessions.StartRequest, error)
	control           func(factorysessions.ControlRequest) (factorysessions.ControlRequest, error)
	approve           func(factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error)
	retryDispatch     func(factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error)
	interruptDispatch func(factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error)
	listSessions      func(factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error)
	result            func(factorysessions.ResultRequest) (factorysessions.ResultRequest, error)
	eventReconnect    func(factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error)
}

func (preparation mcpRequestPreparation) PrepareStart(request factorysessions.StartRequest) (factorysessions.StartRequest, error) {
	if preparation.start == nil {
		panic("unexpected PrepareStart")
	}
	return preparation.start(request)
}
func (preparation mcpRequestPreparation) PrepareControl(request factorysessions.ControlRequest) (factorysessions.ControlRequest, error) {
	if preparation.control == nil {
		panic("unexpected PrepareControl")
	}
	return preparation.control(request)
}
func (preparation mcpRequestPreparation) PrepareApprove(request factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error) {
	if preparation.approve == nil {
		panic("unexpected PrepareApprove")
	}
	return preparation.approve(request)
}
func (preparation mcpRequestPreparation) PrepareRetryDispatch(request factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error) {
	if preparation.retryDispatch == nil {
		panic("unexpected PrepareRetryDispatch")
	}
	return preparation.retryDispatch(request)
}
func (preparation mcpRequestPreparation) PrepareInterruptDispatch(request factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error) {
	if preparation.interruptDispatch == nil {
		panic("unexpected PrepareInterruptDispatch")
	}
	return preparation.interruptDispatch(request)
}
func (preparation mcpRequestPreparation) PrepareListSessions(request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
	if preparation.listSessions == nil {
		panic("unexpected PrepareListSessions")
	}
	return preparation.listSessions(request)
}
func (preparation mcpRequestPreparation) PrepareResult(request factorysessions.ResultRequest) (factorysessions.ResultRequest, error) {
	if preparation.result == nil {
		panic("unexpected PrepareResult")
	}
	return preparation.result(request)
}
func (preparation mcpRequestPreparation) PrepareEventReconnect(request factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error) {
	if preparation.eventReconnect == nil {
		panic("unexpected PrepareEventReconnect")
	}
	return preparation.eventReconnect(request)
}

func TestMockClient_StartAsync_RunningFixtureReturnsInProgressSession(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startAsync: func(_ context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			if request.RequestID != "req-js-run-n-001" {
				t.Fatalf("requestId = %q, want req-js-run-n-001", request.RequestID)
			}
			return runningAsyncStart(), nil
		},
	})

	response, err := client.StartAsync(context.Background(), asyncRunningExecutionRequest())
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v, want success", response)
	}
	if response.Result.SessionId != runningSessionID {
		t.Fatalf("sessionId = %q, want %q", response.Result.SessionId, runningSessionID)
	}
	if response.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Result.Status)
	}
	if response.Result.Links == nil || response.Result.Links.Session == nil {
		t.Fatal("links.session missing from async start response")
	}
}

func TestMockClient_GetSession_RunningFixtureReturnsDeterministicStatus(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return runningAsyncStart(), nil
		},
		getSession: func(_ context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
			if sessionID != runningSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, runningSessionID)
			}
			return runningSessionRead(), nil
		},
	})

	started, err := client.StartAsync(context.Background(), asyncRunningExecutionRequest())
	if err != nil || started.Error != nil || started.Result == nil {
		t.Fatalf("StartAsync = %#v, %v; want success", started, err)
	}
	response, err := client.GetSession(context.Background(), mcpfactorysession.GetSessionInput{SessionID: started.Result.SessionId})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("read = %#v, want running session read model", response)
	}
	if response.Result.SessionId != runningSessionID {
		t.Fatalf("sessionId = %q, want %q", response.Result.SessionId, runningSessionID)
	}
	if response.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Result.Status)
	}
	if response.Result.Progress == nil || response.Result.Progress.InFlightDispatches == nil {
		t.Fatal("progress.inFlightDispatches missing from running session read model")
	}
	if response.Result.ResultSummary == nil ||
		response.Result.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusPartial {
		t.Fatalf("resultSummary = %#v, want PARTIAL", response.Result.ResultSummary)
	}
}

func TestMockClient_GetResult_RunningFixtureReturnsTypedNotReadyEnvelope(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return runningAsyncStart(), nil
		},
		getResult: func(_ context.Context, sessionID string, request factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
			if sessionID != runningSessionID || request.Mode != factorysessions.ResultModeFinal {
				t.Fatalf("GetResult(%q, %#v), want final result read for running session", sessionID, request)
			}
			return notReadyResult(), nil
		},
	})

	started, err := client.StartAsync(context.Background(), asyncRunningExecutionRequest())
	if err != nil || started.Error != nil || started.Result == nil {
		t.Fatalf("StartAsync = %#v, %v; want success", started, err)
	}
	mode := factoryapi.FactorySessionResultModeFinal
	response, err := client.GetResult(context.Background(), mcpfactorysession.GetResultInput{SessionID: started.Result.SessionId, Mode: &mode})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if response.Result != nil || response.Error == nil {
		t.Fatalf("response = %#v, want not-ready error envelope", response)
	}
	if response.Error.Code != "factory_session.result.not_ready" || !response.Error.Retryable {
		t.Fatalf("error = %#v, want retryable factory_session.result.not_ready", response.Error)
	}
	if response.Error.SessionID != runningSessionID {
		t.Fatalf("sessionId = %q, want %q", response.Error.SessionID, runningSessionID)
	}
	if response.Error.Details == nil || response.Error.Details["reason"] != "RESULT_NOT_READY" {
		t.Fatalf("details = %#v, want RESULT_NOT_READY reason", response.Error.Details)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestMockClient_AsyncPolling_ObservesCompletedFixtureThroughStatusAndResult(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return runningAsyncStart(), nil
		},
		startSync: func(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
			return successfulSyncStart(), nil
		},
		getSession: func(_ context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
			if sessionID == runningSessionID {
				return runningSessionRead(), nil
			}
			if sessionID == successSessionID {
				return successfulSessionRead(), nil
			}
			t.Fatalf("unexpected session read %q", sessionID)
			return factorysessions.SessionReadResult{}, nil
		},
		getResult: func(_ context.Context, sessionID string, _ factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
			if sessionID == runningSessionID {
				return notReadyResult(), nil
			}
			if sessionID == successSessionID {
				return finalResult(), nil
			}
			t.Fatalf("unexpected result read %q", sessionID)
			return factorysessions.ResultReadResult{}, nil
		},
	})

	runningStart, err := client.StartAsync(context.Background(), asyncRunningExecutionRequest())
	if err != nil || runningStart.Error != nil || runningStart.Result == nil {
		t.Fatalf("running start = %#v, %v; want success", runningStart, err)
	}
	runningStatus, err := client.GetSession(context.Background(), mcpfactorysession.GetSessionInput{SessionID: runningStart.Result.SessionId})
	if err != nil || runningStatus.Error != nil || runningStatus.Result == nil {
		t.Fatalf("running status = %#v, %v; want success", runningStatus, err)
	}
	if runningStatus.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("running status = %q, want RUNNING", runningStatus.Result.Status)
	}
	mode := factoryapi.FactorySessionResultModeFinal
	notReady, err := client.GetResult(context.Background(), mcpfactorysession.GetResultInput{SessionID: runningSessionID, Mode: &mode})
	if err != nil || notReady.Error == nil || notReady.Error.Code != "factory_session.result.not_ready" {
		t.Fatalf("not-ready = %#v, %v", notReady, err)
	}

	completedStart, err := client.StartSync(context.Background(), syncSuccessExecutionRequest())
	if err != nil || completedStart.Error != nil || completedStart.Result == nil {
		t.Fatalf("completed start = %#v, %v; want success", completedStart, err)
	}
	completedStatus, err := client.GetSession(context.Background(), mcpfactorysession.GetSessionInput{SessionID: completedStart.Result.SessionId})
	if err != nil || completedStatus.Error != nil || completedStatus.Result == nil {
		t.Fatalf("completed status = %#v, %v; want success", completedStatus, err)
	}
	if completedStatus.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded ||
		completedStatus.Result.ResultSummary == nil ||
		completedStatus.Result.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("completed status = %#v, want succeeded FINAL", completedStatus.Result)
	}
	completedResult, err := client.GetResult(context.Background(), mcpfactorysession.GetResultInput{SessionID: successSessionID, Mode: &mode})
	if err != nil || completedResult.Error != nil || completedResult.Result == nil {
		t.Fatalf("completed result = %#v, %v; want success", completedResult, err)
	}
	if completedResult.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", completedResult.Result.ResultStatus)
	}
}

func TestMockClient_StartAsync_MalformedRequestReturnsStableEnvelope(t *testing.T) {
	preparation := mcpRequestPreparation{start: func(request factorysessions.StartRequest) (factorysessions.StartRequest, error) {
		return factorysessions.StartRequest{}, errors.New("factory session source factoryId is required")
	}}
	response, err := newTestClientWithService(scriptedExecutionService{}, preparation).StartAsync(context.Background(),
		factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-malformed-001",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind: factoryapi.FactorySessionExecutionSourceKindFactoryId,
			},
		},
	)
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if response.Result != nil || response.Error == nil || response.Error.Code != "BAD_REQUEST" || response.Error.Retryable {
		t.Fatalf("response = %#v, want non-retryable BAD_REQUEST", response)
	}
}

func TestMockClient_StartAsync_WithoutServiceReturnsUnavailableEnvelope(t *testing.T) {
	response, err := newTestClient().StartAsync(context.Background(), asyncRunningExecutionRequest())
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if response.Result != nil || response.Error == nil ||
		response.Error.Code != "factory_session.service.unavailable" || response.Error.Retryable {
		t.Fatalf("response = %#v, want unavailable service envelope", response)
	}
}

func TestMockClient_StartSync_SuccessFixtureReturnsTerminalSession(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startSync: func(_ context.Context, request factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
			if request.RequestID != "req-petri-success-001" {
				t.Fatalf("requestId = %q, want req-petri-success-001", request.RequestID)
			}
			return successfulSyncStart(), nil
		},
	})

	response, err := client.StartSync(context.Background(), syncSuccessExecutionRequest())
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v, want success", response)
	}
	if response.Result.SessionId != successSessionID ||
		response.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("result = %#v, want succeeded session %q", response.Result, successSessionID)
	}
	if response.Result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcome("COMPLETED") || response.Result.Result == nil {
		t.Fatalf("result = %#v, want completed sync outcome and result", response.Result)
	}
}

func TestMockClient_GetResult_TerminalSessionReturnsDeterministicResult(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startSync: func(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
			return successfulSyncStart(), nil
		},
		getResult: func(_ context.Context, sessionID string, request factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
			if sessionID != successSessionID || request.Mode != factorysessions.ResultModeFinal {
				t.Fatalf("GetResult(%q, %#v), want final terminal result", sessionID, request)
			}
			return finalResult(), nil
		},
	})

	started, err := client.StartSync(context.Background(), syncSuccessExecutionRequest())
	if err != nil || started.Error != nil || started.Result == nil {
		t.Fatalf("StartSync = %#v, %v; want success", started, err)
	}
	mode := factoryapi.FactorySessionResultModeFinal
	response, err := client.GetResult(context.Background(), mcpfactorysession.GetResultInput{SessionID: started.Result.SessionId, Mode: &mode})
	if err != nil || response.Error != nil || response.Result == nil {
		t.Fatalf("GetResult = %#v, %v; want success", response, err)
	}
	if response.Result.SessionId != successSessionID ||
		response.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal ||
		*response.Result.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("result = %#v, want succeeded FINAL", response.Result)
	}
	if response.Result.PrimaryResult == nil {
		t.Fatal("primaryResult missing from terminal result")
	}
}

func TestMockClient_StartSync_RepeatedInvocationReturnsStableSessionIdentity(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startSync: func(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
			return successfulSyncStart(), nil
		},
	})
	first, err := client.StartSync(context.Background(), syncSuccessExecutionRequest())
	if err != nil || first.Error != nil || first.Result == nil {
		t.Fatalf("first StartSync = %#v, %v; want success", first, err)
	}
	second, err := client.StartSync(context.Background(), syncSuccessExecutionRequest())
	if err != nil || second.Error != nil || second.Result == nil {
		t.Fatalf("second StartSync = %#v, %v; want success", second, err)
	}
	if second.Result.SessionId != first.Result.SessionId ||
		second.Result.Status != first.Result.Status ||
		second.Result.SyncOutcome != first.Result.SyncOutcome {
		t.Fatalf("repeated result drift: first %#v, second %#v", first.Result, second.Result)
	}
}

func TestMockClient_StartSync_MalformedRequestReturnsStableEnvelope(t *testing.T) {
	preparation := mcpRequestPreparation{start: func(request factorysessions.StartRequest) (factorysessions.StartRequest, error) {
		return factorysessions.StartRequest{}, errors.New("requestId is required")
	}}
	response, err := newTestClientWithService(scriptedExecutionService{}, preparation).StartSync(context.Background(),
		factoryapi.FactorySessionExecutionRequest{
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
				FactoryId: strPtr("customer-support-triage"),
			},
		},
	)
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if response.Result != nil || response.Error == nil || response.Error.Code != "BAD_REQUEST" || response.Error.Retryable {
		t.Fatalf("response = %#v, want non-retryable BAD_REQUEST", response)
	}
}

func TestMockClient_StartSync_WithoutServiceReturnsUnavailableEnvelope(t *testing.T) {
	response, err := newTestClient().StartSync(context.Background(), syncSuccessExecutionRequest())
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if response.Result != nil || response.Error == nil ||
		response.Error.Code != "factory_session.service.unavailable" {
		t.Fatalf("response = %#v, want unavailable service envelope", response)
	}
}

func TestMockClient_ListSessions_DefaultsToLiveScope(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		listSessions: func(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			if request.Scope != factorysessions.SessionListScopeLive {
				t.Fatalf("scope = %q, want live default", request.Scope)
			}
			return factorysessions.ListSessionsResult{Scope: factorysessions.SessionListScopeLive}, nil
		},
	})
	response, err := client.ListSessions(context.Background(), mcpfactorysession.ListSessionsInput{})
	if err != nil || response.Error != nil || response.Result == nil {
		t.Fatalf("ListSessions = %#v, %v; want success", response, err)
	}
	if response.Result.Scope == nil || *response.Result.Scope != factoryapi.FactorySessionListScopeLive {
		t.Fatalf("scope = %#v, want live", response.Result.Scope)
	}
}

func TestMockClient_ListSessions_ScopedPersistedAndAll(t *testing.T) {
	call := 0
	client := clientWithScript(scriptedExecutionService{
		listSessions: func(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			call++
			switch call {
			case 1:
				if request.Scope != factorysessions.SessionListScopePersisted {
					t.Fatalf("first scope = %q, want persisted", request.Scope)
				}
				return persistedSessionList(), nil
			case 2:
				if request.Scope != factorysessions.SessionListScopeAll {
					t.Fatalf("second scope = %q, want all", request.Scope)
				}
				return allSessionList(), nil
			default:
				t.Fatalf("unexpected list call %d", call)
				return factorysessions.ListSessionsResult{}, nil
			}
		},
	})

	persistedScope := factoryapi.FactorySessionListScopePersisted
	persisted, err := client.ListSessions(context.Background(), mcpfactorysession.ListSessionsInput{Scope: &persistedScope})
	if err != nil || persisted.Error != nil || persisted.Result == nil {
		t.Fatalf("persisted list = %#v, %v; want success", persisted, err)
	}
	assertListScope(t, persisted.Result, factoryapi.FactorySessionListScopePersisted)
	if len(persisted.Result.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want no live rows", persisted.Result.Sessions)
	}
	assertContainsDurableSession(t, persisted.Result, successSessionID)
	assertOmitsDurableSession(t, persisted.Result, "dur-sess-petri-run-001")

	allScope := factoryapi.FactorySessionListScopeAll
	all, err := client.ListSessions(context.Background(), mcpfactorysession.ListSessionsInput{Scope: &allScope})
	if err != nil || all.Error != nil || all.Result == nil {
		t.Fatalf("all list = %#v, %v; want success", all, err)
	}
	assertListScope(t, all.Result, factoryapi.FactorySessionListScopeAll)
	if len(all.Result.Sessions) != 1 || all.Result.Sessions[0].Id != "dur-sess-petri-run-001" {
		t.Fatalf("sessions = %#v, want one live row", all.Result.Sessions)
	}
	assertContainsDurableSession(t, all.Result, successSessionID)
}

func TestMockClient_ListSessions_UnsupportedScopeReturnsStableEnvelope(t *testing.T) {
	scope := factoryapi.FactorySessionListScope("workspace")
	preparation := mcpRequestPreparation{listSessions: func(request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
		return factorysessions.ListSessionsRequest{}, errors.New(`unsupported Factory Session scope "workspace"`)
	}}
	response, err := newTestClientWithService(scriptedExecutionService{}, preparation).
		ListSessions(context.Background(), mcpfactorysession.ListSessionsInput{Scope: &scope})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if response.Result != nil || response.Error == nil || response.Error.Code != "BAD_REQUEST" {
		t.Fatalf("response = %#v, want BAD_REQUEST", response)
	}
}

func TestMockClient_ListSessions_UnavailableServiceReturnsStableEnvelope(t *testing.T) {
	response, err := newTestClient().ListSessions(context.Background(), mcpfactorysession.ListSessionsInput{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if response.Result != nil || response.Error == nil ||
		response.Error.Code != "factory_session.service.unavailable" {
		t.Fatalf("response = %#v, want unavailable service envelope", response)
	}
}

func TestMockClient_RuntimeService_StartAsyncRunningObservesStatusAndNotReadyResult(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return runningAsyncStart(), nil
		},
		getSession: func(context.Context, string) (factorysessions.SessionReadResult, error) {
			return runningSessionRead(), nil
		},
		getResult: func(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
			return notReadyResult(), nil
		},
	})
	started, err := client.StartAsync(context.Background(), runtimeBusyLoopAsyncRequest("req-mcp-runtime-async-running-001"))
	if err != nil || started.Error != nil || started.Result == nil {
		t.Fatalf("StartAsync = %#v, %v; want success", started, err)
	}
	status, err := client.GetSession(context.Background(), mcpfactorysession.GetSessionInput{SessionID: started.Result.SessionId})
	if err != nil || status.Error != nil || status.Result == nil ||
		status.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("GetSession = %#v, %v; want RUNNING", status, err)
	}
	mode := factoryapi.FactorySessionResultModeFinal
	result, err := client.GetResult(context.Background(), mcpfactorysession.GetResultInput{SessionID: started.Result.SessionId, Mode: &mode})
	if err != nil || result.Error == nil || result.Error.Code != "factory_session.result.not_ready" {
		t.Fatalf("GetResult = %#v, %v; want not-ready", result, err)
	}
}

func TestMockClient_RuntimeService_AsyncPollingObservesTerminalResult(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return runningAsyncStart(), nil
		},
		getSession: func(context.Context, string) (factorysessions.SessionReadResult, error) {
			return successfulSessionRead(), nil
		},
		getResult: func(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
			return finalResult(), nil
		},
	})
	started, err := client.StartAsync(context.Background(), runtimeSimpleFinalAsyncRequest("req-mcp-runtime-async-final-001"))
	if err != nil || started.Error != nil || started.Result == nil {
		t.Fatalf("StartAsync = %#v, %v; want success", started, err)
	}
	status, err := client.GetSession(context.Background(), mcpfactorysession.GetSessionInput{SessionID: started.Result.SessionId})
	if err != nil || status.Error != nil || status.Result == nil ||
		status.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("GetSession = %#v, %v; want SUCCEEDED", status, err)
	}
	mode := factoryapi.FactorySessionResultModeFinal
	result, err := client.GetResult(context.Background(), mcpfactorysession.GetResultInput{SessionID: started.Result.SessionId, Mode: &mode})
	if err != nil || result.Error != nil || result.Result == nil ||
		result.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal ||
		result.Result.PrimaryResult == nil {
		t.Fatalf("GetResult = %#v, %v; want terminal FINAL result", result, err)
	}
}

func TestPackageBoundary_DoesNotImportFactorySessionsInternal(t *testing.T) {
	t.Parallel()

	forbidden := "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal"
	for _, packagePath := range []string{
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog",
	} {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageDirectImportsForbidden(t, packagePath, []string{forbidden})
		})
	}
}

func TestBind_FakeExecutionRootInvokedThroughCanonicalListSessionsTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeExecutionRoot{
		invoked: &invoked,
		listSessions: func(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			if request.Scope != factorysessions.SessionListScopePersisted {
				t.Fatalf("scope = %q, want persisted", request.Scope)
			}
			return factorysessions.ListSessionsResult{Scope: factorysessions.SessionListScopePersisted}, nil
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(context.Background(), mcpfactorysession.ToolListSessions, json.RawMessage(`{"scope":"persisted"}`))
	if err != nil {
		t.Fatalf("CallTool(list_sessions) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake execution root was not invoked")
	}
	if !strings.Contains(string(raw), `"scope":"persisted"`) {
		t.Fatalf("CallTool(list_sessions) = %s, want persisted scope result", raw)
	}
}

func TestBind_FakeExecutionRootInvokedThroughCanonicalGetSessionTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeExecutionRoot{
		invoked: &invoked,
		getSession: func(_ context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
			if sessionID != runningSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, runningSessionID)
			}
			return runningSessionRead(), nil
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolGetSession,
		json.RawMessage(`{"sessionId":"`+runningSessionID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_session) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake execution root was not invoked")
	}
	if !strings.Contains(string(raw), `"status":"RUNNING"`) || !strings.Contains(string(raw), `"sessionId":"`+runningSessionID+`"`) {
		t.Fatalf("CallTool(get_session) = %s, want running session read model", raw)
	}
}

func TestBind_FakeExecutionRootInvokedThroughCanonicalListDispatchesTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeExecutionRoot{
		invoked: &invoked,
		queryDispatches: func(_ context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
			if request.SessionID != successSessionID {
				t.Fatalf("sessionId = %q, want %q", request.SessionID, successSessionID)
			}
			if request.Filters.Phase != "execution" || request.Filters.Status != factorysessions.DispatchStatus("COMPLETED") {
				t.Fatalf("filters = %#v, want execution/COMPLETED", request.Filters)
			}
			return factorysessions.ListDispatchesResult{
				SessionID: successSessionID,
				Dispatches: []factorysessions.DispatchSummary{{
					ID:     "dispatch-001",
					Phase:  "execution",
					Status: factorysessions.DispatchStatus("COMPLETED"),
				}},
			}, nil
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolListDispatches,
		json.RawMessage(`{"sessionId":"`+successSessionID+`","phase":"execution","status":"COMPLETED"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_dispatches) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake execution root was not invoked")
	}
	if !strings.Contains(string(raw), `"id":"dispatch-001"`) || !strings.Contains(string(raw), `"sessionId":"`+successSessionID+`"`) {
		t.Fatalf("CallTool(list_dispatches) = %s, want encoded dispatch list", raw)
	}
}

func TestBind_ReadListToolsInvalidJSONDecodeReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fakeExecutionRoot{invoked: &invoked},
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(context.Background(), mcpfactorysession.ToolGetSession, json.RawMessage(`{"sessionId":`))
	if err != nil {
		t.Fatalf("CallTool(get_session) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake execution root was invoked for invalid JSON decode")
	}
}

func TestBind_ReadListToolsValidationFailureReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	preparation := mcpRequestPreparation{
		listSessions: func(factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
			return factorysessions.ListSessionsRequest{}, errors.New(`unsupported Factory Session scope "workspace"`)
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fakeExecutionRoot{invoked: &invoked},
		Prepare:   preparation,
	})
	raw, err := operation(context.Background(), mcpfactorysession.ToolListSessions, json.RawMessage(`{"scope":"workspace"}`))
	if err != nil {
		t.Fatalf("CallTool(list_sessions) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake execution root was invoked for validation failure")
	}
}

func TestBind_FakeExecutionRootInvokedThroughCanonicalStartAsyncTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeExecutionRoot{
		invoked: &invoked,
		startAsync: func(_ context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			if request.RequestID != "req-js-run-n-001" {
				t.Fatalf("requestId = %q, want req-js-run-n-001", request.RequestID)
			}
			if request.Source.FactoryID != "customer-support-triage" {
				t.Fatalf("factoryId = %q, want customer-support-triage", request.Source.FactoryID)
			}
			return runningAsyncStart(), nil
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolStartAsync,
		json.RawMessage(`{"requestId":"req-js-run-n-001","source":{"kind":"FACTORY_ID","factoryId":"customer-support-triage"}}`),
	)
	if err != nil {
		t.Fatalf("CallTool(start_async) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake execution root was not invoked")
	}
	if !strings.Contains(string(raw), `"sessionId":"`+runningSessionID+`"`) ||
		!strings.Contains(string(raw), `"status":"RUNNING"`) {
		t.Fatalf("CallTool(start_async) = %s, want encoded async start response", raw)
	}
}

func TestBind_FakeExecutionRootInvokedThroughCanonicalControlTool(t *testing.T) {
	t.Parallel()

	const pauseReason = "host maintenance"
	var invoked bool
	fake := fakeExecutionRoot{
		invoked: &invoked,
		pause: func(_ context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			if sessionID != runningSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, runningSessionID)
			}
			if request.Reason != pauseReason {
				t.Fatalf("reason = %q, want %q", request.Reason, pauseReason)
			}
			return acceptedControl(runningSessionID, "PAUSE", factorysessions.LifecycleStatusPaused), nil
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolControl,
		json.RawMessage(`{"sessionId":"`+runningSessionID+`","operation":"PAUSE","reason":"`+pauseReason+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(control) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake execution root was not invoked")
	}
	if !strings.Contains(string(raw), `"outcome":"ACCEPTED"`) ||
		!strings.Contains(string(raw), `"status":"PAUSED"`) ||
		!strings.Contains(string(raw), `"sessionId":"`+runningSessionID+`"`) {
		t.Fatalf("CallTool(control) = %s, want encoded lifecycle control response", raw)
	}
}

func TestBind_StartControlToolsInvalidJSONDecodeReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fakeExecutionRoot{invoked: &invoked},
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(context.Background(), mcpfactorysession.ToolStartAsync, json.RawMessage(`{"requestId":`))
	if err != nil {
		t.Fatalf("CallTool(start_async) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake execution root was invoked for invalid JSON decode")
	}
}

func TestBind_StartControlToolsValidationFailureReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	preparation := mcpRequestPreparation{
		start: func(factorysessions.StartRequest) (factorysessions.StartRequest, error) {
			return factorysessions.StartRequest{}, errors.New("factory session source factoryId is required")
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fakeExecutionRoot{invoked: &invoked},
		Prepare:   preparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolStartAsync,
		json.RawMessage(`{"requestId":"req-malformed-001","source":{"kind":"FACTORY_ID"}}`),
	)
	if err != nil {
		t.Fatalf("CallTool(start_async) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake execution root was invoked for validation failure")
	}
}

func TestBind_GetSessionTypedNotFoundErrorReturnsToolErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeExecutionRoot{
		getSession: func(_ context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
			if sessionID != missingSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, missingSessionID)
			}
			return factorysessions.SessionReadResult{}, factorysessions.ErrDurableSessionNotFound
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolGetSession,
		json.RawMessage(`{"sessionId":"`+missingSessionID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_session) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_session.session.not_found",
		false,
		missingSessionID,
	)
	if envelope.Message != "factory session not found" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "factory session not found", envelope)
	}
}

func TestBind_ListDispatchesExecutionValidationErrorReturnsBadRequestEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeExecutionRoot{
		queryDispatches: func(_ context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
			if request.SessionID != successSessionID {
				t.Fatalf("sessionId = %q, want %q", request.SessionID, successSessionID)
			}
			return factorysessions.ListDispatchesResult{}, &factorysessions.ExecutionValidationError{
				Field:   "status",
				Message: "invalid status",
			}
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolListDispatches,
		json.RawMessage(`{"sessionId":"`+successSessionID+`","status":"BROKEN"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_dispatches) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
	if !strings.Contains(envelope.Message, "invalid status") {
		t.Fatalf("error.message = %q, want validation detail; envelope = %#v", envelope.Message, envelope)
	}
	if envelope.Details == nil || envelope.Details["field"] != "status" {
		t.Fatalf("error.details = %#v, want field=status", envelope.Details)
	}
}

func TestBind_UnmappedRootErrorDoesNotLeakInternalPackagePaths(t *testing.T) {
	t.Parallel()

	fake := fakeExecutionRoot{
		getSession: func(_ context.Context, _ string) (factorysessions.SessionReadResult, error) {
			return factorysessions.SessionReadResult{}, fmt.Errorf(
				"%s: connection reset\ngoroutine 1 [running]:\nmain.main()",
				internalLeakProbePath,
			)
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolGetSession,
		json.RawMessage(`{"sessionId":"`+missingSessionID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_session) transport error = %v, want typed tool response", err)
	}
	assertEnvelopeDoesNotLeakInternalPaths(t, raw)
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_session.execution.internal",
		false,
		"",
	)
	if envelope.Message != "factory session execution failed" {
		t.Fatalf("error.message = %q, want sanitized internal message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestToolOperationPropagatesCallerContextAndCancellation(t *testing.T) {
	type markerKey struct{}
	const markerValue = "mcp-request-context"
	service := scriptedExecutionService{
		listSessions: func(ctx context.Context, _ factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			if got := ctx.Value(markerKey{}); got != markerValue {
				t.Fatalf("ListSessions context marker = %v, want %q", got, markerValue)
			}
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("ListSessions context error = %v, want context.Canceled", ctx.Err())
			}
			return factorysessions.ListSessionsResult{}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), markerKey{}, markerValue))
	cancel()
	response, err := clientWithScript(service).ListSessions(ctx, mcpfactorysession.ListSessionsInput{})
	if err != nil || response.Error != nil {
		t.Fatalf("ListSessions() = %#v, %v", response, err)
	}
}

func TestToolOperationRejectsMissingContext(t *testing.T) {
	operation := mcpfactorysession.BindToolOperation(
		scriptedExecutionService{},
		canonicalMCPRequestPreparation,
		nil,
	)
	if _, err := operation(nil, mcpfactorysession.ToolListSessions, json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("ToolOperation(nil context) error = %v, want required-context error", err)
	}
}

type scriptedExecutionService struct {
	factorysessions.ExecutionService
	startAsync      func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	startSync       func(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error)
	getSession      func(context.Context, string) (factorysessions.SessionReadResult, error)
	getResult       func(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error)
	listSessions    func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
	listDispatches  func(context.Context, string) (factorysessions.ListDispatchesResult, error)
	queryDispatches func(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	listArtifacts   func(context.Context, string) (factorysessions.ListArtifactsResult, error)
	readEvents      func(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error)
	pause           func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	resume          func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	cancel          func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	terminate       func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
}

func (service scriptedExecutionService) StartAsync(
	ctx context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	return service.startAsync(ctx, request)
}

func (service scriptedExecutionService) StartSync(
	ctx context.Context,
	request factorysessions.StartRequest,
) (factorysessions.SyncStartResult, error) {
	return service.startSync(ctx, request)
}

func (service scriptedExecutionService) GetSession(
	ctx context.Context,
	sessionID string,
) (factorysessions.SessionReadResult, error) {
	return service.getSession(ctx, sessionID)
}

func (service scriptedExecutionService) GetResult(
	ctx context.Context,
	sessionID string,
	request factorysessions.ResultRequest,
) (factorysessions.ResultReadResult, error) {
	return service.getResult(ctx, sessionID, request)
}

func (service scriptedExecutionService) ListSessions(
	ctx context.Context,
	request factorysessions.ListSessionsRequest,
) (factorysessions.ListSessionsResult, error) {
	return service.listSessions(ctx, request)
}

func (service scriptedExecutionService) ListDispatches(
	ctx context.Context,
	sessionID string,
) (factorysessions.ListDispatchesResult, error) {
	return service.listDispatches(ctx, sessionID)
}

func (service scriptedExecutionService) QueryDispatches(
	ctx context.Context,
	request factorysessions.DispatchQueryRequest,
) (factorysessions.ListDispatchesResult, error) {
	if service.queryDispatches != nil {
		return service.queryDispatches(ctx, request)
	}
	return service.listDispatches(ctx, request.SessionID)
}

func (service scriptedExecutionService) ListArtifacts(
	ctx context.Context,
	sessionID string,
) (factorysessions.ListArtifactsResult, error) {
	return service.listArtifacts(ctx, sessionID)
}

func (service scriptedExecutionService) ReadEvents(
	ctx context.Context,
	sessionID string,
	request factorysessions.EventReconnectRequest,
) (factorysessions.EventReadResult, error) {
	return service.readEvents(ctx, sessionID, request)
}

func (service scriptedExecutionService) Pause(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return service.pause(ctx, sessionID, request)
}

func (service scriptedExecutionService) Resume(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return service.resume(ctx, sessionID, request)
}

func (service scriptedExecutionService) Cancel(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return service.cancel(ctx, sessionID, request)
}

func (service scriptedExecutionService) Terminate(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return service.terminate(ctx, sessionID, request)
}

func clientWithScript(service scriptedExecutionService) *testClient {
	return newTestClientWithService(service, canonicalMCPRequestPreparation)
}

func runningAsyncStart() factorysessions.AsyncStartResult {
	return factorysessions.AsyncStartResult{
		SessionID: runningSessionID,
		Status:    string(factorysessions.LifecycleStatusRunning),
		Links: factorysessions.InspectionLinks{
			Session: "/factory-sessions/" + runningSessionID,
			Status:  "/factory-sessions/" + runningSessionID,
		},
	}
}

func successfulSyncStart() factorysessions.SyncStartResult {
	return factorysessions.SyncStartResult{
		AsyncStartResult: factorysessions.AsyncStartResult{
			SessionID: successSessionID,
			Status:    string(factorysessions.LifecycleStatusSucceeded),
			Links: factorysessions.InspectionLinks{
				Session: "/factory-sessions/" + successSessionID,
				Status:  "/factory-sessions/" + successSessionID,
			},
		},
		SyncOutcome: factorysessions.SyncOutcome("COMPLETED"),
		Result: json.RawMessage(
			`{"sessionId":"` + successSessionID +
				`","resultStatus":"FINAL","sessionStatus":"SUCCEEDED","primaryResult":[{"type":"text","text":"resolved"}]}`,
		),
	}
}

func runningSessionRead() factorysessions.SessionReadResult {
	return factorysessions.SessionReadResult{
		SessionID: runningSessionID,
		Status:    factorysessions.LifecycleStatusRunning,
		Progress:  &factorysessions.ProgressCounts{TotalDispatches: 1, InFlightDispatches: 1},
		ResultSummary: &factorysessions.ResultSummary{
			ResultStatus: "PARTIAL",
		},
		Usage: factorysessions.EmptySessionUsage(),
		Links: factorysessions.InspectionLinks{
			Session: "/factory-sessions/" + runningSessionID,
			Status:  "/factory-sessions/" + runningSessionID,
		},
	}
}

func successfulSessionRead() factorysessions.SessionReadResult {
	return factorysessions.SessionReadResult{
		SessionID: successSessionID,
		Status:    factorysessions.LifecycleStatusSucceeded,
		ResultSummary: &factorysessions.ResultSummary{
			ResultStatus: "FINAL",
		},
		Usage: factorysessions.EmptySessionUsage(),
		Links: factorysessions.InspectionLinks{
			Session: "/factory-sessions/" + successSessionID,
			Status:  "/factory-sessions/" + successSessionID,
		},
	}
}

func notReadyResult() factorysessions.ResultReadResult {
	return factorysessions.ResultReadResult{
		SessionID:     runningSessionID,
		ResultStatus:  factorysessions.ResultStatusNotReady,
		SessionStatus: factorysessions.LifecycleStatusRunning,
		Mode:          factorysessions.ResultModeFinal,
		Availability: &factorysessions.ResultAvailabilityDetail{
			Reason:    "RESULT_NOT_READY",
			Message:   "Session is still running.",
			Retryable: true,
		},
	}
}

func finalResult() factorysessions.ResultReadResult {
	return factorysessions.ResultReadResult{
		SessionID:     successSessionID,
		ResultStatus:  factorysessions.ResultStatus("FINAL"),
		SessionStatus: factorysessions.LifecycleStatusSucceeded,
		Mode:          factorysessions.ResultModeFinal,
		PrimaryResult: json.RawMessage(`[{"type":"text","text":"resolved"}]`),
	}
}

func persistedSessionList() factorysessions.ListSessionsResult {
	return factorysessions.ListSessionsResult{
		Scope: factorysessions.SessionListScopePersisted,
		DurableSessions: []factorysessions.DurableSessionListSummary{{
			SessionID: successSessionID,
			Status:    factorysessions.LifecycleStatusSucceeded,
			Links:     factorysessions.InspectionLinks{Session: "/factory-sessions/" + successSessionID},
		}},
	}
}

func allSessionList() factorysessions.ListSessionsResult {
	return factorysessions.ListSessionsResult{
		Scope: factorysessions.SessionListScopeAll,
		LiveSessions: []factorysessions.LiveSessionSummary{{
			ID: "dur-sess-petri-run-001",
		}},
		DurableSessions: []factorysessions.DurableSessionListSummary{{
			SessionID: successSessionID,
			Status:    factorysessions.LifecycleStatusSucceeded,
			Links:     factorysessions.InspectionLinks{Session: "/factory-sessions/" + successSessionID},
		}},
	}
}

func asyncRunningExecutionRequest() factoryapi.FactorySessionExecutionRequest {
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-js-run-n-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	}
}

func syncSuccessExecutionRequest() factoryapi.FactorySessionExecutionRequest {
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-success-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
		Args: &map[string]any{"ticketId": "TKT-2002"},
	}
}

func runtimeBusyLoopAsyncRequest(requestID string) factoryapi.FactorySessionExecutionRequest {
	return runtimeInlineAsyncRequest(requestID, `while (true) {}`, map[string]any{"subject": "workflows"})
}

func runtimeSimpleFinalAsyncRequest(requestID string) factoryapi.FactorySessionExecutionRequest {
	return runtimeInlineAsyncRequest(requestID, `return { subject: args.subject };`, map[string]any{"subject": "workflows"})
}

func runtimeInlineAsyncRequest(
	requestID string,
	source string,
	args map[string]any,
) factoryapi.FactorySessionExecutionRequest {
	dialect := "you-workflow-v1"
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   source,
				},
				Dialect: &dialect,
			},
		},
		Args: &args,
	}
}

func assertListScope(
	t *testing.T,
	result *factoryapi.ListFactorySessionsResponse,
	scope factoryapi.FactorySessionListScope,
) {
	t.Helper()
	if result.Scope == nil || *result.Scope != scope {
		t.Fatalf("scope = %#v, want %q", result.Scope, scope)
	}
}

func assertContainsDurableSession(
	t *testing.T,
	result *factoryapi.ListFactorySessionsResponse,
	sessionID string,
) {
	t.Helper()
	if result.DurableSessions != nil {
		for _, row := range *result.DurableSessions {
			if row.SessionId == sessionID {
				return
			}
		}
	}
	t.Fatalf("durableSessions = %#v, want row %q", result.DurableSessions, sessionID)
}

func assertOmitsDurableSession(
	t *testing.T,
	result *factoryapi.ListFactorySessionsResponse,
	sessionID string,
) {
	t.Helper()
	if result.DurableSessions == nil {
		return
	}
	for _, row := range *result.DurableSessions {
		if row.SessionId == sessionID {
			t.Fatalf("durableSessions unexpectedly contain %q", sessionID)
		}
	}
}

func strPtr(value string) *string { return &value }

type fakeExecutionRoot struct {
	factorysessions.ExecutionService
	invoked         *bool
	listSessions    func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
	getSession      func(context.Context, string) (factorysessions.SessionReadResult, error)
	queryDispatches func(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	startAsync      func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	pause           func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
}

func (root fakeExecutionRoot) markInvoked() {
	if root.invoked != nil {
		*root.invoked = true
	}
}

func (root fakeExecutionRoot) ListSessions(
	ctx context.Context,
	request factorysessions.ListSessionsRequest,
) (factorysessions.ListSessionsResult, error) {
	root.markInvoked()
	if root.listSessions == nil {
		panic("unexpected ListSessions on fake execution root")
	}
	return root.listSessions(ctx, request)
}

func (root fakeExecutionRoot) GetSession(
	ctx context.Context,
	sessionID string,
) (factorysessions.SessionReadResult, error) {
	root.markInvoked()
	if root.getSession == nil {
		panic("unexpected GetSession on fake execution root")
	}
	return root.getSession(ctx, sessionID)
}

func (root fakeExecutionRoot) QueryDispatches(
	ctx context.Context,
	request factorysessions.DispatchQueryRequest,
) (factorysessions.ListDispatchesResult, error) {
	root.markInvoked()
	if root.queryDispatches == nil {
		panic("unexpected QueryDispatches on fake execution root")
	}
	return root.queryDispatches(ctx, request)
}

func (root fakeExecutionRoot) StartAsync(
	ctx context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	root.markInvoked()
	if root.startAsync == nil {
		panic("unexpected StartAsync on fake execution root")
	}
	return root.startAsync(ctx, request)
}

func (root fakeExecutionRoot) Pause(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	root.markInvoked()
	if root.pause == nil {
		panic("unexpected Pause on fake execution root")
	}
	return root.pause(ctx, sessionID, request)
}

func assertBadRequestToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()
	assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
}

func assertTypedToolErrorEnvelope(
	t *testing.T,
	raw json.RawMessage,
	wantCode string,
	wantRetryable bool,
	wantSessionID string,
) *mcpfactorysession.ToolErrorEnvelope {
	t.Helper()

	var response struct {
		Result *json.RawMessage                     `json:"result"`
		Error  *mcpfactorysession.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("tool response result = %s, want error envelope only", raw)
	}
	if response.Error == nil {
		t.Fatalf("tool response = %s, want typed error envelope", raw)
	}
	if response.Error.Code != wantCode {
		t.Fatalf("error.code = %q, want %q; envelope = %#v", response.Error.Code, wantCode, response.Error)
	}
	if response.Error.Retryable != wantRetryable {
		t.Fatalf("error.retryable = %v, want %v; envelope = %#v", response.Error.Retryable, wantRetryable, response.Error)
	}
	if wantSessionID != "" && response.Error.SessionID != wantSessionID {
		t.Fatalf("error.sessionId = %q, want %q; envelope = %#v", response.Error.SessionID, wantSessionID, response.Error)
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		t.Fatalf("error.message is required; envelope = %#v", response.Error)
	}
	return response.Error
}

func assertEnvelopeDoesNotLeakInternalPaths(t *testing.T, raw json.RawMessage) {
	t.Helper()
	if strings.Contains(string(raw), internalLeakProbePath) {
		t.Fatalf("tool response leaks internal package path: %s", raw)
	}
}

func assertPackageDirectImportsForbidden(t *testing.T, packagePath string, forbiddenRoots []string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		for _, forbidden := range forbiddenRoots {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf("%s must not import forbidden ownership %s; found direct import %s", packagePath, forbidden, importPath)
			}
		}
	}
}
