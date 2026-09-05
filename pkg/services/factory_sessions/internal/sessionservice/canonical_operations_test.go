package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type canonicalDurableExecutionFake struct {
	durableexecution.Service
	asyncRequest   factorysessions.StartRequest
	syncRequest    factorysessions.StartRequest
	asyncResult    factorysessions.AsyncStartResult
	syncResult     factorysessions.SyncStartResult
	asyncErr       error
	syncErr        error
	canonicalCalls int
	asyncCalls     int
	syncCalls      int
}

func (fake *canonicalDurableExecutionFake) StartCanonical(
	_ context.Context,
	request factorysessions.StartRequest,
	synchronous bool,
) (durableexecution.CanonicalStartResult, error) {
	fake.canonicalCalls++
	if synchronous {
		fake.syncRequest = request
		started := fake.syncResult
		if fake.syncErr != nil {
			return durableexecution.CanonicalStartResult{}, fake.syncErr
		}
		return durableexecution.CanonicalStartResult{Sync: &started}, nil
	}
	fake.asyncRequest = request
	started := fake.asyncResult
	if fake.asyncErr != nil {
		return durableexecution.CanonicalStartResult{}, fake.asyncErr
	}
	return durableexecution.CanonicalStartResult{Async: &started}, nil
}

func (fake *canonicalDurableExecutionFake) StartAsync(
	_ context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	fake.asyncCalls++
	fake.asyncRequest = request
	return fake.asyncResult, fake.asyncErr
}

func (fake *canonicalDurableExecutionFake) StartSync(
	_ context.Context,
	request factorysessions.StartRequest,
) (factorysessions.SyncStartResult, error) {
	fake.syncCalls++
	fake.syncRequest = request
	return fake.syncResult, fake.syncErr
}

type canonicalSessionInvokerFake struct {
	roles.SessionInvoker
	sessionID      string
	requestID      string
	timeout        int64
	input          *work.PreparedInvocationInput
	result         factorydefinitions.FactoryInvocationResult
	err            error
	canonicalCalls int
	legacyCalls    int
	calls          int
	mutateInput    bool
}

func (fake *canonicalSessionInvokerFake) Invoke(
	_ context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	fake.canonicalCalls++
	return fake.recordInvocation(sessionID, request)
}

func (fake *canonicalSessionInvokerFake) InvokeFactorySession(
	_ context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	fake.legacyCalls++
	return fake.recordInvocation(sessionID, request)
}

func (fake *canonicalSessionInvokerFake) recordInvocation(
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	fake.calls++
	fake.sessionID = sessionID
	if request.RequestID != nil {
		fake.requestID = *request.RequestID
	}
	if request.TimeoutMillis != nil {
		fake.timeout = *request.TimeoutMillis
	}
	fake.input = request.PreparedInvocationInput.Clone()
	if fake.mutateInput && request.PreparedInvocationInput != nil && request.PreparedInvocationInput.ResolvedInput != nil {
		request.PreparedInvocationInput.ResolvedInput.Text = "owner mutation"
	}
	return fake.result, fake.err
}

func TestService_CanonicalStartDurableMapsAndClonesAsyncRequest(t *testing.T) {
	t.Parallel()

	fake := &canonicalDurableExecutionFake{
		asyncResult: factorysessions.AsyncStartResult{
			SessionID: "durable-async-1",
			Status:    "QUEUED",
			Policy: factorysessions.PolicyProjection{
				Requested: map[string]any{"nested": map[string]any{"value": "kept"}},
			},
		},
	}
	service := &Service{durable: fake}
	request := factorysessions.SessionStartRequest{
		Mode: factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{
			RequestID: "  start-async-1  ",
		},
		Source: factorysessions.Source{
			FactoryInline: json.RawMessage(`{"name":"factory"}`),
		},
		Args: map[string]any{
			"nested": map[string]any{"value": "original"},
		},
		Policy: map[string]any{"mode": "safe"},
		Orchestrator: &factorysessions.OrchestratorOverride{
			Kind: "petri",
			Raw:  json.RawMessage(`{"version":1}`),
		},
		RuntimeOptions: &factorysessions.RuntimeOptions{ChildExecutorMode: "direct"},
		Wait: factorysessions.SessionOperationWait{
			TimeoutMillis:   250,
			CancelOnTimeout: true,
		},
	}

	got, err := service.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got.SessionID != "durable-async-1" || got.Mode != factorysessions.SessionOperationModeDurable || got.Status != "QUEUED" {
		t.Fatalf("Start() = %#v, want durable queued result", got)
	}
	if got.Async == nil || got.Sync != nil {
		t.Fatalf("Start() branches = async:%v sync:%v, want async only", got.Async != nil, got.Sync != nil)
	}
	if fake.canonicalCalls != 1 || fake.asyncCalls != 0 || fake.syncCalls != 0 {
		t.Fatalf("durable calls = canonical:%d async:%d sync:%d, want 1/0/0", fake.canonicalCalls, fake.asyncCalls, fake.syncCalls)
	}
	if fake.asyncRequest.RequestID != "start-async-1" || fake.asyncRequest.Wait == nil || fake.asyncRequest.Wait.TimeoutMillis == nil || *fake.asyncRequest.Wait.TimeoutMillis != 250 || !fake.asyncRequest.Wait.CancelOnTimeout {
		t.Fatalf("durable request identity/wait = %#v, want normalized values", fake.asyncRequest)
	}
	if fake.asyncRequest.Args["nested"].(map[string]any)["value"] != "original" {
		t.Fatalf("durable request args = %#v, want cloned original", fake.asyncRequest.Args)
	}
	if string(fake.asyncRequest.Source.FactoryInline) != `{"name":"factory"}` || string(fake.asyncRequest.Orchestrator.Raw) != `{"version":1}` {
		t.Fatalf("durable request retained mutable source values: %#v", fake.asyncRequest)
	}
	if fake.asyncRequest.EventConsumer != nil {
		t.Fatal("canonical durable Start installed an event consumer")
	}

	request.Args["nested"].(map[string]any)["value"] = "caller mutation"
	request.Source.FactoryInline[1] = 'x'
	request.Orchestrator.Raw[1] = 'x'
	if fake.asyncRequest.Args["nested"].(map[string]any)["value"] != "original" {
		t.Fatal("canonical durable Start crossed caller-owned nested args")
	}
	if string(fake.asyncRequest.Source.FactoryInline) != `{"name":"factory"}` || string(fake.asyncRequest.Orchestrator.Raw) != `{"version":1}` {
		t.Fatal("canonical durable Start crossed caller-owned raw source values")
	}

	fake.asyncResult.Policy.Requested["nested"].(map[string]any)["value"] = "owner mutation"
	if got.Async.Policy.Requested["nested"].(map[string]any)["value"] != "kept" {
		t.Fatal("canonical durable Start returned an aliased result map")
	}
}

func TestService_CanonicalStartDurableSelectsSyncFromRequestValue(t *testing.T) {
	t.Parallel()

	fake := &canonicalDurableExecutionFake{
		syncResult: factorysessions.SyncStartResult{
			AsyncStartResult: factorysessions.AsyncStartResult{SessionID: "durable-sync-1"},
			SyncOutcome:      factorysessions.SyncOutcome("COMPLETED"),
		},
	}
	service := &Service{durable: fake}
	got, err := service.Start(context.Background(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "sync-1"},
		Synchronous: true,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if fake.canonicalCalls != 1 || fake.asyncCalls != 0 || fake.syncCalls != 0 {
		t.Fatalf("durable calls = canonical:%d async:%d sync:%d, want 1/0/0", fake.canonicalCalls, fake.asyncCalls, fake.syncCalls)
	}
	if got.SessionID != "durable-sync-1" || got.Status != "COMPLETED" || got.Sync == nil || got.Async != nil {
		t.Fatalf("Start() = %#v, want synchronous completed result", got)
	}
}

func TestService_CanonicalInvokeMapsIdentityAndClonesPreparedWork(t *testing.T) {
	t.Parallel()

	fake := &canonicalSessionInvokerFake{
		mutateInput: true,
		result: factorydefinitions.FactoryInvocationResult{
			RequestID: "invoke-1",
			TraceID:   "trace-1",
			Status:    factorydefinitions.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{
				Type:     work.WorkContentPartTypeText,
				Text:     "done",
				Metadata: map[string]any{"source": "owner"},
			}},
			SessionID: "session-1",
			WorkID:    "work-1",
			WorkName:  "invoke",
			WorkState: "SUCCEEDED",
		},
	}
	service := &Service{invoker: fake}
	input := &work.PreparedInvocationInput{
		Source: work.InputSourcePositionalText,
		ResolvedInput: &work.ResolvedInput{
			Source: work.InputSourcePositionalText,
			Text:   "caller input",
		},
	}

	got, err := service.Invoke(context.Background(), factorysessions.SessionInvokeRequest{
		SessionID:   "  session-1  ",
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "  invoke-1  "},
		Input:       input,
		Wait:        factorysessions.SessionOperationWait{TimeoutMillis: 500},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if fake.canonicalCalls != 1 || fake.legacyCalls != 0 || fake.calls != 1 || fake.sessionID != "session-1" || fake.requestID != "invoke-1" || fake.timeout != 500 {
		t.Fatalf("invoker request = canonical:%d legacy:%d session:%q request:%q timeout:%d calls:%d, want canonical-only normalized values", fake.canonicalCalls, fake.legacyCalls, fake.sessionID, fake.requestID, fake.timeout, fake.calls)
	}
	if input.ResolvedInput.Text != "caller input" {
		t.Fatal("canonical Invoke crossed caller-owned prepared Work input")
	}
	if fake.input == nil || fake.input.ResolvedInput == nil || fake.input.ResolvedInput.Text != "caller input" {
		t.Fatalf("invoker input = %#v, want cloned prepared input", fake.input)
	}
	if got.RequestID != "invoke-1" || got.TraceID != "trace-1" || got.SessionID != "session-1" || got.WorkID != "work-1" || got.WorkName != "invoke" || got.WorkState != "SUCCEEDED" || got.Status != factorysessions.InvocationTerminalStatusCompleted || len(got.PrimaryResult) != 1 || got.PrimaryResult[0].Text != "done" {
		t.Fatalf("Invoke() = %#v, want characterized identity/result", got)
	}
	fake.result.PrimaryResult[0].Metadata["source"] = "owner mutation"
	if got.PrimaryResult[0].Metadata["source"] != "owner" {
		t.Fatal("canonical Invoke returned an aliased primary result")
	}
}

func TestService_CanonicalOperationsRejectInvalidValuesBeforeDependencies(t *testing.T) {
	t.Parallel()

	durable := &canonicalDurableExecutionFake{}
	service := &Service{durable: durable}
	invalidStarts := []factorysessions.SessionStartRequest{
		{Mode: factorysessions.SessionOperationMode("invalid")},
		{Mode: factorysessions.SessionOperationModeDurable, Correlation: factorysessions.SessionOperationCorrelation{RequestID: "request"}, Wait: factorysessions.SessionOperationWait{TimeoutMillis: -1}},
		{Mode: factorysessions.SessionOperationModeDurable},
		{Mode: factorysessions.SessionOperationModeLive, ValidateOnly: true, InitNewFactory: true},
	}
	for index, request := range invalidStarts {
		if _, err := service.Start(context.Background(), request); err == nil {
			t.Fatalf("invalid Start case %d returned nil error", index)
		}
	}
	if durable.asyncCalls != 0 || durable.syncCalls != 0 {
		t.Fatalf("invalid Start calls = async:%d sync:%d, want zero", durable.asyncCalls, durable.syncCalls)
	}

	invoker := &canonicalSessionInvokerFake{}
	service.invoker = invoker
	invalidInvokes := []factorysessions.SessionInvokeRequest{
		{},
		{SessionID: "session", Wait: factorysessions.SessionOperationWait{TimeoutMillis: -1}},
	}
	for index, request := range invalidInvokes {
		if _, err := service.Invoke(context.Background(), request); err == nil {
			t.Fatalf("invalid Invoke case %d returned nil error", index)
		}
	}
	if invoker.calls != 0 {
		t.Fatalf("invalid Invoke calls = %d, want zero", invoker.calls)
	}
}

func TestService_CanonicalOperationsReportTypedAvailabilityFailures(t *testing.T) {
	t.Parallel()

	service := &Service{}
	if _, err := service.Start(context.Background(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "request"},
	}); !errors.Is(err, factorysessions.ErrExecutionServiceNotConfigured) {
		t.Fatalf("Start() error = %v, want durable availability error", err)
	}
	if _, err := service.Invoke(context.Background(), factorysessions.SessionInvokeRequest{SessionID: "session"}); err == nil {
		t.Fatal("Invoke() error = nil, want invocation availability error")
	}
}

func TestService_CanonicalOperationsPropagateOwnerFailuresWithoutLegacyCalls(t *testing.T) {
	t.Parallel()

	startFailure := errors.New("durable request identity conflict")
	durable := &canonicalDurableExecutionFake{asyncErr: startFailure}
	service := &Service{durable: durable}
	if _, err := service.Start(context.Background(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "request"},
	}); !errors.Is(err, startFailure) {
		t.Fatalf("Start() error = %v, want owner failure %v", err, startFailure)
	}
	if durable.canonicalCalls != 1 || durable.asyncCalls != 0 || durable.syncCalls != 0 {
		t.Fatalf("start calls = canonical:%d async:%d sync:%d, want 1/0/0", durable.canonicalCalls, durable.asyncCalls, durable.syncCalls)
	}

	invokeFailure := errors.New("invocation dependency failed")
	invoker := &canonicalSessionInvokerFake{err: invokeFailure}
	service.invoker = invoker
	if _, err := service.Invoke(context.Background(), factorysessions.SessionInvokeRequest{SessionID: "session"}); !errors.Is(err, invokeFailure) {
		t.Fatalf("Invoke() error = %v, want owner failure %v", err, invokeFailure)
	}
	if invoker.canonicalCalls != 1 || invoker.legacyCalls != 0 {
		t.Fatalf("invoke calls = canonical:%d legacy:%d, want 1/0", invoker.canonicalCalls, invoker.legacyCalls)
	}
}
