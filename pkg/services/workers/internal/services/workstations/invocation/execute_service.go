package invocation

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ExecuteService is the narrow canonical owner needed by the compatibility
// invocation adapter. Keeping this interface smaller than workers.Service
// makes the adapter testable without constructing a second Workers graph.
type ExecuteService interface {
	Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
}

// NewExecuteService adapts the canonical Workers Execute operation to the
// legacy one-attempt invocation shape. The adapter is deliberately private to
// Workers construction; Runtime and Sessions will retire this compatibility
// shape in their later caller-family cutovers.
// TODO(P6-C): delete this adapter after those caller families stop accepting
// workers.InvocationExecutor values.
func NewExecuteService(service ExecuteService) workers.InvocationExecutor {
	return NewExecuteServiceWithProgress(service, nil)
}

// NewExecuteServiceWithProgress preserves the optional observation capability
// accepted by the older invocation factory while routing the attempt itself
// through the canonical Execute operation.
func NewExecuteServiceWithProgress(
	service ExecuteService,
	publisher workers.ProgressPublisher,
) workers.InvocationExecutor {
	if service == nil {
		return nil
	}
	return &executeServiceInvocation{service: service, publisher: publisher}
}

type executeServiceInvocation struct {
	service   ExecuteService
	publisher workers.ProgressPublisher
}

func (e *executeServiceInvocation) Execute(
	ctx context.Context,
	input workers.InvocationInput,
) (workers.InvocationResult, error) {
	attempt := input.Attempt
	if attempt < 1 {
		attempt = 1
	}
	if e == nil || e.service == nil {
		err := workers.NewProviderError(
			workers.WorkFailureTypeMisconfigured,
			"Workers Execute service is required",
			nil,
		)
		return failedInvocationResult(attempt, err), err
	}
	if ctx == nil {
		err := fmt.Errorf("Workers Execute invocation requires a context")
		return failedInvocationResult(attempt, err), err
	}

	request := executeRequestFromInvocation(input, e.publisher)
	result, err := e.service.Execute(ctx, request)
	if err != nil && result.Outcome == "" {
		return failedInvocationResult(attempt, err), err
	}
	return invocationResultFromExecute(result, attempt), err
}

func executeRequestFromInvocation(
	input workers.InvocationInput,
	publisher workers.ProgressPublisher,
) workers.ExecuteRequest {
	legacy := workers.CloneProviderInferenceRequest(input.Request)
	attempt := input.Attempt
	if attempt < 1 {
		attempt = 1
	}
	correlation := invocationCorrelation(legacy, attempt)
	dispatch := work.CloneWorkDispatch(legacy.Dispatch)
	if dispatch.DispatchID == "" {
		dispatch.DispatchID = correlation.DispatchID
	}

	return workers.ExecuteRequest{
		Correlation: correlation,
		Target:      invocationTarget(legacy),
		Input:       invocationInput(legacy, dispatch, publisher),
		Attempt:     workers.AttemptContext{Number: attempt},
	}
}

func invocationTarget(request workers.ProviderInferenceRequest) workers.ExecutionTarget {
	provider := invocationProvider(request)
	return workers.ExecutionTarget{
		WorkerName:       request.WorkerName,
		WorkerType:       request.WorkerType,
		WorkstationName:  request.WorkstationType,
		RunnerID:         request.RunnerID,
		ExecutorProvider: strings.TrimSpace(request.ExecutorProvider),
		Command:          request.Command,
		Args:             append([]string(nil), request.Args...),
		FactoryDirectory: request.FactoryDirectory,
		Provider:         workers.ProviderReference{ID: provider},
		Model: workers.ModelReference{
			Name:            request.Model,
			Provider:        firstNonEmptyInvocation(request.ModelProvider, provider),
			ReasoningEffort: request.ReasoningEffort,
			Locality:        request.ModelLocality,
		},
		Prompt: workers.PromptPolicy{
			SystemPrompt: request.SystemPrompt,
			UserMessage:  request.UserMessage,
			OutputSchema: request.OutputSchema,
		},
		Tools: workers.ToolPolicy{
			ExecutionMode:                request.ToolExecutionMode,
			RequiredOptionalCapabilities: append([]workers.RunnerOptionalCapability(nil), request.RequiredOptionalCapabilities...),
		},
		Output: workers.OutputPolicy{
			Contract:                    request.OutputContract,
			Format:                      request.OutputFormat,
			StopToken:                   request.StopToken,
			DecisionEnvelope:            request.DecisionEnvelope,
			GoalRoutingDecisionEnvelope: request.GoalRoutingDecisionEnvelope,
		},
		Environment: workers.EnvironmentPolicy{
			Vars:                cloneStringMap(request.EnvVars),
			ProcessEnvironment:  append([]string(nil), request.ProcessEnvironment...),
			WorkingDirectory:    request.WorkingDirectory,
			WorkingDirectorySet: strings.TrimSpace(request.WorkingDirectory) != "",
		},
		Workspace: workers.WorkspacePolicy{
			Worktree:         request.Worktree,
			WorkingDirectory: request.WorkingDirectory,
			FactoryDirectory: request.FactoryDirectory,
		},
		Permissions: workers.PermissionPolicy{SkipPermissions: request.SkipPermissions},
		Timeout:     request.PrintTimeout,
	}
}

func invocationInput(
	request workers.ProviderInferenceRequest,
	dispatch work.WorkDispatch,
	publisher workers.ProgressPublisher,
) workers.ExecutionInput {
	return workers.ExecutionInput{
		Dispatch:                 dispatch,
		ModelBindings:            workers.CloneResolvedModelOperationBindings(request.ModelBindings),
		ModelOperation:           request.ModelOperation,
		ModelRuntime:             request.ModelRuntime.Clone(),
		Resume:                   cloneContinuation(request.Continuation),
		WorkflowContext:          request.WorkflowContext.Clone(),
		ProgressPublisher:        publisher,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
		ExecutionLogger:          request.ExecutionLogger,
	}
}

func invocationCorrelation(
	request workers.ProviderInferenceRequest,
	attempt int,
) workers.ExecutionCorrelation {
	correlation := request.Correlation
	dispatchID := firstNonEmptyInvocation(
		correlation.DispatchID,
		request.Dispatch.DispatchID,
		"detached-invocation",
	)
	seed := dispatchID
	correlation.FactorySessionID = firstNonEmptyInvocation(
		correlation.FactorySessionID,
		workflowSessionID(request.WorkflowContext),
		"detached-session-"+seed,
	)
	correlation.RuntimeID = firstNonEmptyInvocation(
		correlation.RuntimeID,
		"detached-runtime-"+seed,
	)
	correlation.GenerationID = firstNonEmptyInvocation(
		correlation.GenerationID,
		"detached-generation-"+seed,
	)
	correlation.DispatchID = dispatchID
	correlation.AttemptID = firstNonEmptyInvocation(
		correlation.AttemptID,
		fmt.Sprintf("%s-attempt-%d", dispatchID, attempt),
	)
	correlation.RequestID = firstNonEmptyInvocation(
		correlation.RequestID,
		request.Dispatch.Execution.RequestID,
	)
	correlation.TraceID = firstNonEmptyInvocation(
		correlation.TraceID,
		request.Dispatch.Execution.TraceID,
	)
	return correlation
}

func invocationProvider(request workers.ProviderInferenceRequest) string {
	if provider := strings.TrimSpace(request.ModelProvider); provider != "" {
		return provider
	}
	if provider := strings.TrimSpace(request.ExecutorProvider); provider != "" &&
		!strings.EqualFold(provider, workers.ExecutorProviderACP) {
		return provider
	}
	switch strings.ToLower(strings.TrimSpace(request.RunnerID)) {
	case "", "script", "inference", "agent", "mock":
		return ""
	default:
		return strings.TrimSpace(request.RunnerID)
	}
}

func invocationResultFromExecute(
	result workers.ExecuteResult,
	attempt int,
) workers.InvocationResult {
	workDiagnostics := result.Diagnostics.ToWorkDiagnostics()
	response := workers.InferenceResponse{
		Content:        invocationOutputText(result.Output),
		Outcome:        invocationOutcome(result.Outcome),
		Feedback:       result.Output.Feedback,
		Classification: result.Output.Classification,
		Continuation:   cloneContinuation(result.Continuation),
		Diagnostics:    workDiagnostics,
	}
	converted := workers.InvocationResult{
		Response:     response,
		Attempt:      attempt,
		Continuation: cloneContinuation(result.Continuation),
		Diagnostics:  workers.SafeWorkDiagnosticsFromWorkDiagnostics(workDiagnostics),
	}
	if result.Failure != nil {
		metadata := &workers.WorkFailureMetadata{
			Family: result.Failure.Family,
			Type:   result.Failure.Type,
		}
		decision := workers.FailureDecisionFromMetadata(metadata)
		converted.FailureMetadata = metadata
		converted.FailureDecision = &decision
		converted.FailureDetail = workers.CloneFailureDetail(result.Failure.Detail)
	}
	return converted
}

func invocationOutputText(output workers.ProposedOutput) string {
	var builder strings.Builder
	for _, part := range output.Primary {
		builder.WriteString(part.Text)
	}
	return builder.String()
}

func invocationOutcome(outcome workers.ExecutionOutcome) workers.WorkOutcome {
	switch outcome {
	case workers.ExecutionOutcomeContinue:
		return workers.OutcomeContinue
	case workers.ExecutionOutcomeRejected:
		return workers.OutcomeRejected
	case workers.ExecutionOutcomeFailed, workers.ExecutionOutcomeCanceled:
		return workers.OutcomeFailed
	default:
		return workers.OutcomeAccepted
	}
}

func workflowSessionID(workflow *workers.Context) string {
	if workflow == nil {
		return ""
	}
	return strings.TrimSpace(workflow.SessionID)
}

func firstNonEmptyInvocation(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
