package factorysessions_test

import (
	"context"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type workersRootBoundaryFixture struct {
	runnerID      string
	dispatchID    string
	sessionID     string
	workerType    string
	modelProvider string
	model         string
	userMessage   string
	commandName   string
}

func newWorkersRootBoundaryFixture() workersRootBoundaryFixture {
	return workersRootBoundaryFixture{
		runnerID:      "session-workers-root-runner",
		dispatchID:    "dispatch-root-boundary",
		sessionID:     "session-root-boundary",
		workerType:    "agent-run-fake-child",
		modelProvider: "mock",
		model:         "gpt-test",
		userMessage:   "summarize workflows",
		commandName:   "boundary-cmd",
	}
}

// TestFactorySessionsConstructsWorkersRequestsThroughRoot proves CUT-SES-WRK
// story 005: Factory Sessions consumer edges construct Workers root requests
// only through the sealed Workers service root contract.
func TestFactorySessionsConstructsWorkersRequestsThroughRoot(t *testing.T) {
	t.Parallel()

	fixture := newWorkersRootBoundaryFixture()
	stub := &workersRequestBoundaryStub{}
	ctx := context.Background()

	runWorkersOpeningAndInvocationTargetProof(t, fixture)
	runWorkersInvocationThroughRootProof(t, ctx, stub, fixture)
	runWorkersInferenceThroughRootProof(t, ctx, stub, fixture)
	runWorkersCommandThroughRootProof(t, ctx, stub, fixture)
}

func runWorkersOpeningAndInvocationTargetProof(t *testing.T, fixture workersRootBoundaryFixture) {
	t.Helper()

	openingRequest := factorysessions.RuntimeOpeningRequest{
		Workers: workers.RuntimeOpeningRequest{
			RunnerID: fixture.runnerID,
		},
	}
	if openingRequest.Workers.RunnerID != fixture.runnerID {
		t.Fatalf(
			"Workers.RuntimeOpeningRequest.RunnerID = %q, want %q",
			openingRequest.Workers.RunnerID,
			fixture.runnerID,
		)
	}

	invocationTarget := factorysessions.InvocationTarget{
		RunnerID:          fixture.runnerID,
		MockWorkersConfig: &workers.MockWorkersConfig{},
	}
	if invocationTarget.RunnerID != fixture.runnerID {
		t.Fatalf("InvocationTarget.RunnerID = %q, want %q", invocationTarget.RunnerID, fixture.runnerID)
	}
	if invocationTarget.MockWorkersConfig == nil {
		t.Fatal("InvocationTarget.MockWorkersConfig is nil, want Workers root mock config")
	}
}

func runWorkersInvocationThroughRootProof(
	t *testing.T,
	ctx context.Context,
	stub *workersRequestBoundaryStub,
	fixture workersRootBoundaryFixture,
) {
	t.Helper()

	invocationInput := workers.InvocationInput{
		Request: workers.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{
				DispatchID: fixture.dispatchID,
				WorkerType: fixture.workerType,
			},
			UserMessage:   fixture.userMessage,
			ModelProvider: fixture.modelProvider,
			Model:         fixture.model,
			SessionID:     fixture.sessionID,
			RunnerID:      fixture.runnerID,
			WorkerType:    fixture.workerType,
		},
		Attempt: 1,
	}

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
	if stub.lastInvocation.Request.Dispatch.DispatchID != fixture.dispatchID {
		t.Fatalf(
			"recorded invocation dispatch id = %q, want %q",
			stub.lastInvocation.Request.Dispatch.DispatchID,
			fixture.dispatchID,
		)
	}
	if stub.lastInvocation.Request.SessionID != fixture.sessionID {
		t.Fatalf(
			"recorded invocation session id = %q, want %q",
			stub.lastInvocation.Request.SessionID,
			fixture.sessionID,
		)
	}
	if stub.lastInvocation.Request.RunnerID != fixture.runnerID {
		t.Fatalf(
			"recorded invocation runner id = %q, want %q",
			stub.lastInvocation.Request.RunnerID,
			fixture.runnerID,
		)
	}
}

func runWorkersInferenceThroughRootProof(
	t *testing.T,
	ctx context.Context,
	stub *workersRequestBoundaryStub,
	fixture workersRootBoundaryFixture,
) {
	t.Helper()

	inferRequest := workers.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: fixture.dispatchID,
			WorkerType: fixture.workerType,
		},
		UserMessage:   fixture.userMessage,
		ModelProvider: fixture.modelProvider,
		Model:         fixture.model,
		SessionID:     fixture.sessionID,
		RunnerID:      fixture.runnerID,
		WorkerType:    fixture.workerType,
	}

	inferResponse, err := stub.Infer(ctx, inferRequest)
	if err != nil {
		t.Fatalf("Infer provider request: %v", err)
	}
	if stub.lastInference.Dispatch.DispatchID != fixture.dispatchID {
		t.Fatalf(
			"recorded inference dispatch id = %q, want %q",
			stub.lastInference.Dispatch.DispatchID,
			fixture.dispatchID,
		)
	}
	if stub.lastInference.ModelProvider != fixture.modelProvider || stub.lastInference.Model != fixture.model {
		t.Fatalf(
			"recorded inference model = (%q,%q), want (%q,%q)",
			stub.lastInference.ModelProvider,
			stub.lastInference.Model,
			fixture.modelProvider,
			fixture.model,
		)
	}
	inferSession := (inferResponse.Continuation).SessionMetadata()
	if inferSession == nil || inferSession.ID != "workers-root-boundary-session" {
		t.Fatalf(
			"inference provider session = %#v, want workers-root-boundary-session",
			inferSession,
		)
	}
}

func runWorkersCommandThroughRootProof(
	t *testing.T,
	ctx context.Context,
	stub *workersRequestBoundaryStub,
	fixture workersRootBoundaryFixture,
) {
	t.Helper()

	commandRequest := platformprocess.CommandRequest{
		Command: fixture.commandName,
	}

	commandResult, err := stub.Run(ctx, commandRequest)
	if err != nil {
		t.Fatalf("Run command request: %v", err)
	}
	if stub.lastCommand.Command != fixture.commandName {
		t.Fatalf("recorded command = %q, want %q", stub.lastCommand.Command, fixture.commandName)
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
		_ platformprocess.CommandRequest
		_ providers.Service
		_ workers.InvocationExecutor
		_ platformprocess.CommandRunner
		_ workers.PTYAllocator
	)
}

type workersRequestBoundaryStub struct {
	lastInvocation workers.InvocationInput
	lastInference  workers.ProviderInferenceRequest
	lastCommand    platformprocess.CommandRequest
}

func (stub *workersRequestBoundaryStub) Execute(
	_ context.Context,
	input workers.InvocationInput,
) (workers.InvocationResult, error) {
	stub.lastInvocation = input
	return workers.InvocationResult{
		Attempt: input.Attempt,
		Response: workers.InferenceResponse{
			Content:      "workers-root-boundary",
			Continuation: (&providers.SessionMetadata{Provider: "mock", Kind: providers.SessionIDKind, ID: "workers-root-boundary-session"}).ContinuationRef(),
		},
	}, nil
}

func (stub *workersRequestBoundaryStub) Infer(
	_ context.Context,
	request workers.ProviderInferenceRequest,
) (workers.InferenceResponse, error) {
	stub.lastInference = request
	return workers.InferenceResponse{
		Content:      "workers-root-boundary",
		Continuation: (&providers.SessionMetadata{Provider: "mock", Kind: providers.SessionIDKind, ID: "workers-root-boundary-session"}).ContinuationRef(),
	}, nil
}

func (stub *workersRequestBoundaryStub) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	stub.lastCommand = request
	return platformprocess.CommandResult{ExitCode: 0}, nil
}
