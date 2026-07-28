package factorysessions_test

import (
	"context"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestFactorySessionsConstructsWorkersRequestsThroughRoot proves CUT-SES-WRK
// story 005: Factory Sessions consumer edges construct Workers root requests
// only through the sealed Workers service root contract.
func TestFactorySessionsConstructsWorkersRequestsThroughRoot(t *testing.T) {
	t.Parallel()

	const (
		runnerID      = "session-workers-root-runner"
		dispatchID    = "dispatch-root-boundary"
		sessionID     = "session-root-boundary"
		workerType    = "agent-run-fake-child"
		modelProvider = "mock"
		model         = "gpt-test"
		userMessage   = "summarize workflows"
		commandName   = "boundary-cmd"
	)

	openingRequest := factorysessions.RuntimeOpeningRequest{
		Workers: workers.RuntimeOpeningRequest{
			RunnerID: runnerID,
		},
	}
	if openingRequest.Workers.RunnerID != runnerID {
		t.Fatalf(
			"Workers.RuntimeOpeningRequest.RunnerID = %q, want %q",
			openingRequest.Workers.RunnerID,
			runnerID,
		)
	}

	invocationTarget := factorysessions.InvocationTarget{
		RunnerID:          runnerID,
		MockWorkersConfig: &workers.MockWorkersConfig{},
	}
	if invocationTarget.RunnerID != runnerID {
		t.Fatalf("InvocationTarget.RunnerID = %q, want %q", invocationTarget.RunnerID, runnerID)
	}
	if invocationTarget.MockWorkersConfig == nil {
		t.Fatal("InvocationTarget.MockWorkersConfig is nil, want Workers root mock config")
	}

	inferRequest := workers.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: dispatchID,
			WorkerType: workerType,
		},
		UserMessage:   userMessage,
		ModelProvider: modelProvider,
		Model:         model,
		SessionID:     sessionID,
		RunnerID:      runnerID,
		WorkerType:    workerType,
	}
	invocationInput := workers.InvocationInput{
		Request: inferRequest,
		Attempt: 1,
	}

	commandRequest := workers.CommandRequest{
		Command:    commandName,
		DispatchID: dispatchID,
		WorkerType: workerType,
	}

	stub := &workersRequestBoundaryStub{}
	ctx := context.Background()

	invocationResult, err := stub.Execute(ctx, invocationInput)
	if err != nil {
		t.Fatalf("Execute invocation: %v", err)
	}
	if invocationResult.Attempt != 1 {
		t.Fatalf("invocation result attempt = %d, want 1", invocationResult.Attempt)
	}
	if stub.lastInvocation.Attempt != 1 {
		t.Fatalf("recorded invocation attempt = %d, want 1", stub.lastInvocation.Attempt)
	}
	if stub.lastInvocation.Request.Dispatch.DispatchID != dispatchID {
		t.Fatalf(
			"recorded invocation dispatch id = %q, want %q",
			stub.lastInvocation.Request.Dispatch.DispatchID,
			dispatchID,
		)
	}
	if stub.lastInvocation.Request.SessionID != sessionID {
		t.Fatalf(
			"recorded invocation session id = %q, want %q",
			stub.lastInvocation.Request.SessionID,
			sessionID,
		)
	}
	if stub.lastInvocation.Request.RunnerID != runnerID {
		t.Fatalf(
			"recorded invocation runner id = %q, want %q",
			stub.lastInvocation.Request.RunnerID,
			runnerID,
		)
	}

	inferResponse, err := stub.Infer(ctx, inferRequest)
	if err != nil {
		t.Fatalf("Infer provider request: %v", err)
	}
	if stub.lastInference.Dispatch.DispatchID != dispatchID {
		t.Fatalf(
			"recorded inference dispatch id = %q, want %q",
			stub.lastInference.Dispatch.DispatchID,
			dispatchID,
		)
	}
	if stub.lastInference.ModelProvider != modelProvider || stub.lastInference.Model != model {
		t.Fatalf(
			"recorded inference model = (%q,%q), want (%q,%q)",
			stub.lastInference.ModelProvider,
			stub.lastInference.Model,
			modelProvider,
			model,
		)
	}
	if inferResponse.ProviderSession == nil || inferResponse.ProviderSession.ID != "workers-root-boundary-session" {
		t.Fatalf(
			"inference provider session = %#v, want workers-root-boundary-session",
			inferResponse.ProviderSession,
		)
	}

	commandResult, err := stub.Run(ctx, commandRequest)
	if err != nil {
		t.Fatalf("Run command request: %v", err)
	}
	if stub.lastCommand.Command != commandName {
		t.Fatalf("recorded command = %q, want %q", stub.lastCommand.Command, commandName)
	}
	if stub.lastCommand.DispatchID != dispatchID {
		t.Fatalf(
			"recorded command dispatch id = %q, want %q",
			stub.lastCommand.DispatchID,
			dispatchID,
		)
	}
	if commandResult.ExitCode != 0 {
		t.Fatalf("command result exit code = %d, want 0", commandResult.ExitCode)
	}
}

// TestFactorySessionsWorkersRootContractsCompileAtSessionsRoot proves Sessions
// root contracts name Workers-facing bindings only through Workers root types.
func TestFactorySessionsWorkersRootContractsCompileAtSessionsRoot(t *testing.T) {
	t.Parallel()

	var (
		_ factorysessions.RuntimeOpeningRequest
		_ factorysessions.InvocationTarget
		_ workers.RuntimeOpeningRequest
		_ workers.ProviderInferenceRequest
		_ workers.InvocationInput
		_ workers.CommandRequest
		_ workers.Provider
		_ workers.InvocationExecutor
		_ workers.CommandRunner
		_ workers.PTYAllocator
		_ workers.ProviderRegistry
	)
}

type workersRequestBoundaryStub struct {
	lastInvocation workers.InvocationInput
	lastInference  workers.ProviderInferenceRequest
	lastCommand    workers.CommandRequest
}

func (stub *workersRequestBoundaryStub) Execute(
	_ context.Context,
	input workers.InvocationInput,
) (workers.InvocationResult, error) {
	stub.lastInvocation = input
	return workers.InvocationResult{
		Attempt: input.Attempt,
		Response: workers.InferenceResponse{
			Content: "workers-root-boundary",
			ProviderSession: &workers.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "workers-root-boundary-session",
			},
		},
	}, nil
}

func (stub *workersRequestBoundaryStub) Infer(
	_ context.Context,
	request workers.ProviderInferenceRequest,
) (workers.InferenceResponse, error) {
	stub.lastInference = request
	return workers.InferenceResponse{
		Content: "workers-root-boundary",
		ProviderSession: &workers.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "workers-root-boundary-session",
		},
	}, nil
}

func (stub *workersRequestBoundaryStub) Run(
	_ context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	stub.lastCommand = request
	return workers.CommandResult{ExitCode: 0}, nil
}
