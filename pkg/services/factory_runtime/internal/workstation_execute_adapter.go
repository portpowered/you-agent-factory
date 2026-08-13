package internal

import (
	"context"
	"fmt"
	"sort"
	"strings"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func executeServiceFromWorkstation(
	service runtimeWorkstationService,
	boundary workers.WorkstationPoolBoundary,
) runtimeExecuteService {
	if service == nil {
		return nil
	}
	if boundary != nil {
		return workstationExecuteAdapter{boundary: boundary}
	}
	if execute, ok := service.(runtimeExecuteService); ok {
		return execute
	}
	return workstationExecuteAdapter{service: service}
}

func buildRuntimeWorkstationBoundary(
	factory factory.WorkstationPoolBoundaryFactory,
	service runtimeWorkstationService,
	executors map[string]workers.WorkerExecutor,
	net *state.Net,
	providerInvocation workers.WorkstationRequestExecutor,
) workers.WorkstationPoolBoundary {
	if factory == nil || service == nil {
		return nil
	}
	return factory(workers.WorkstationPoolBoundaryConfig{
		Service:            service,
		Executors:          executors,
		RouteNames:         runtimeBoundaryRouteNames(net, executors),
		ProviderInvocation: providerInvocation,
		Async:              true,
	})
}

func runtimeBoundaryRouteNames(
	net *state.Net,
	executors map[string]workers.WorkerExecutor,
) []string {
	routes := make(map[string]struct{}, len(executors))
	for name := range executors {
		if name != "" {
			routes[name] = struct{}{}
		}
	}
	if net != nil {
		for id, transition := range net.Transitions {
			if id != "" {
				routes[id] = struct{}{}
			}
			if transition == nil {
				continue
			}
			if transition.Name != "" {
				routes[transition.Name] = struct{}{}
			}
			if transition.WorkerType != "" {
				routes[transition.WorkerType] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(routes))
	for name := range routes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// workstationExecuteAdapter is retained only for callers that still provide
// the pre-stateless Workers test/composition port. The concrete Runtime
// constructor receives only ExecuteService; production Workers services take
// the direct branch above.
type workstationExecuteAdapter struct {
	service  runtimeWorkstationService
	boundary workers.WorkstationPoolBoundary
}

func (adapter workstationExecuteAdapter) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	if adapter.boundary != nil {
		return adapter.executeThroughBoundary(ctx, request)
	}
	if adapter.service == nil {
		return workers.ExecuteResult{}, fmt.Errorf("Workers Execute service is unavailable")
	}
	dispatch := work.CloneWorkDispatch(request.Input.Dispatch)
	dispatch.DispatchID = request.Correlation.DispatchID
	dispatch.WorkerType = firstBuildValue(request.Target.WorkerName, request.Target.WorkerType)
	dispatch.WorkstationName = request.Target.WorkstationName
	dispatch.Execution.RequestID = request.Correlation.RequestID
	dispatch.Execution.TraceID = request.Correlation.TraceID
	if len(dispatch.Execution.WorkIDs) == 0 {
		dispatch.Execution.WorkIDs = make([]string, 0, len(request.Input.Work))
	}
	for _, input := range request.Input.Work {
		known := false
		for _, workID := range dispatch.Execution.WorkIDs {
			if workID == input.WorkID {
				known = true
				break
			}
		}
		if !known {
			dispatch.Execution.WorkIDs = append(dispatch.Execution.WorkIDs, input.WorkID)
		}
	}
	result, err := adapter.service.DispatchWorkstation(ctx, workers.WorkstationDispatchRequest{
		WorkstationName: request.Target.WorkstationName,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:         dispatch,
			WorkerName:       request.Target.WorkerName,
			WorkerType:       request.Target.WorkerType,
			RunnerID:         request.Target.RunnerID,
			FactorySessionID: request.Correlation.FactorySessionID,
			InputTokens:      append([]any(nil), dispatch.InputTokens...),
		},
	})
	if err != nil {
		return workers.ExecuteResult{}, err
	}
	outcome := workers.ExecutionOutcomeAccepted
	switch result.Result.Outcome {
	case workers.OutcomeContinue:
		outcome = workers.ExecutionOutcomeContinue
	case workers.OutcomeRejected:
		outcome = workers.ExecutionOutcomeRejected
	case workers.OutcomeFailed:
		outcome = workers.ExecutionOutcomeFailed
	}
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     outcome,
		Output: workers.ProposedOutput{
			Primary:        []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: result.Result.Output}},
			Feedback:       result.Result.Feedback,
			Classification: result.Result.SelectedClassificationLabel,
		},
	}, nil
}

type workstationDispatchCompletion struct {
	result workers.WorkstationDispatchResult
	err    error
}

func (adapter workstationExecuteAdapter) executeThroughBoundary(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	legacyRequest := workstationDispatchRequestFromExecute(request)
	completion := make(chan workstationDispatchCompletion, 1)
	err := adapter.boundary.Publish(
		ctx,
		legacyRequest,
		func(_ context.Context, _ workers.WorkstationDispatchRequest, result workers.WorkstationDispatchResult, dispatchErr error) {
			completion <- workstationDispatchCompletion{result: result, err: dispatchErr}
		},
	)
	if err != nil {
		return workers.ExecuteResult{}, err
	}
	select {
	case completed := <-completion:
		return executeResultFromWorkstationDispatch(request, completed.result, completed.err)
	case <-ctx.Done():
		_, cancelErr := adapter.boundary.Cancel(
			context.WithoutCancel(ctx),
			workers.WorkstationDispatchCancelRequest{DispatchID: request.Correlation.DispatchID},
		)
		if cancelErr != nil {
			return workers.ExecuteResult{}, cancelErr
		}
		return canceledExecuteResult(request), nil
	}
}

func (adapter workstationExecuteAdapter) Stop(ctx context.Context) error {
	if adapter.boundary == nil {
		return nil
	}
	return adapter.boundary.Stop(ctx)
}

func workstationDispatchRequestFromExecute(
	request workers.ExecuteRequest,
) workers.WorkstationDispatchRequest {
	dispatch := work.CloneWorkDispatch(request.Input.Dispatch)
	dispatch.DispatchID = request.Correlation.DispatchID
	if dispatch.TransitionID == "" {
		dispatch.TransitionID = request.Target.WorkstationName
	}
	dispatch.WorkerType = request.Target.WorkerType
	dispatch.WorkstationName = request.Target.WorkstationName
	dispatch.Execution.RequestID = request.Correlation.RequestID
	dispatch.Execution.TraceID = request.Correlation.TraceID
	if len(dispatch.Execution.WorkIDs) == 0 {
		for _, input := range request.Input.Work {
			dispatch.Execution.WorkIDs = append(dispatch.Execution.WorkIDs, input.WorkID)
		}
	}
	return workers.WorkstationDispatchRequest{
		WorkstationName: request.Target.WorkstationName,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:                    dispatch,
			WorkerName:                  request.Target.WorkerName,
			WorkerType:                  firstBuildValue(request.Target.WorkerName, request.Target.WorkerType),
			RunnerID:                    request.Target.RunnerID,
			FactorySessionID:            request.Correlation.FactorySessionID,
			RecordingID:                 request.Correlation.RuntimeID,
			ProjectID:                   dispatch.ProjectID,
			Capabilities:                cloneBuildCapabilities(request.Target.Capabilities),
			InputTokens:                 append([]any(nil), dispatch.InputTokens...),
			ModelOperation:              request.Input.ModelOperation,
			ModelBindings:               workers.CloneResolvedModelOperationBindings(request.Input.ModelBindings),
			Model:                       request.Target.Model.Name,
			ModelProvider:               request.Target.Model.Provider,
			ReasoningEffort:             request.Target.Model.ReasoningEffort,
			Command:                     request.Target.Command,
			Args:                        append([]string(nil), request.Target.Args...),
			FactoryDirectory:            request.Target.FactoryDirectory,
			OutputFormat:                request.Target.Output.Format,
			StopToken:                   request.Target.Output.StopToken,
			DecisionEnvelope:            request.Target.Output.DecisionEnvelope,
			GoalRoutingDecisionEnvelope: request.Target.Output.GoalRoutingDecisionEnvelope,
			SystemPrompt:                request.Target.Prompt.SystemPrompt,
			UserMessage:                 request.Target.Prompt.UserMessage,
			OutputSchema:                request.Target.Prompt.OutputSchema,
			OutputContract:              request.Target.Output.Contract,
			Timeout:                     request.Target.Timeout,
			EnvVars:                     cloneBuildStringMap(request.Target.Environment.Vars),
			ProcessEnvironment:          append([]string(nil), request.Target.Environment.ProcessEnvironment...),
			Worktree:                    request.Target.Workspace.Worktree,
			WorkingDirectory:            request.Target.Environment.WorkingDirectory,
			WorkingDirectoryAuthored:    request.Target.Environment.WorkingDirectorySet,
			ResumeSession:               buildContinuationSession(request.Input.Resume),
			SkipPermissions:             request.Target.Permissions.SkipPermissions,
		},
	}
}

func executeResultFromWorkstationDispatch(
	request workers.ExecuteRequest,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
) (workers.ExecuteResult, error) {
	outcome := workers.ExecutionOutcomeAccepted
	switch result.Result.Outcome {
	case workers.OutcomeContinue:
		outcome = workers.ExecutionOutcomeContinue
	case workers.OutcomeRejected:
		outcome = workers.ExecutionOutcomeRejected
	case workers.OutcomeFailed:
		outcome = workers.ExecutionOutcomeFailed
	}
	if result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCanceled {
		outcome = workers.ExecutionOutcomeCanceled
	}
	executeResult := workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     outcome,
		Output:      workers.ProposedOutputFromLegacyWorkResult(result.Result),
		Metrics: workers.ExecutionMetrics{
			Duration:   result.Result.Metrics.Duration,
			Cost:       result.Result.Metrics.Cost,
			RetryCount: result.Result.Metrics.RetryCount,
		},
		Continuation: continuationFromWorkstationSession(result.Result.ProviderSession),
	}
	if result.Result.FailureMetadata != nil || strings.TrimSpace(result.Result.Error) != "" {
		executeResult.Failure = executionFailureFromWorkstationResult(result.Result, dispatchErr)
	}
	if dispatchErr != nil {
		executeResult.Outcome = workers.ExecutionOutcomeFailed
		if executeResult.Failure == nil {
			executeResult.Failure = executionFailureFromWorkstationResult(result.Result, dispatchErr)
		}
		return executeResult, dispatchErr
	}
	return executeResult, nil
}

func executionFailureFromWorkstationResult(
	result workers.WorkResult,
	dispatchErr error,
) *workers.ExecutionFailure {
	failure := &workers.ExecutionFailure{Message: strings.TrimSpace(result.Error)}
	if result.FailureMetadata != nil {
		failure.Family = result.FailureMetadata.Family
		failure.Type = result.FailureMetadata.Type
		failure.RetryHint = workers.FailureDecisionFromMetadata(result.FailureMetadata).Retryable
	}
	if failure.Message == "" && dispatchErr != nil {
		failure.Message = dispatchErr.Error()
	}
	return failure
}

func continuationFromWorkstationSession(
	session *workers.ProviderSessionMetadata,
) *workers.ProviderContinuationRef {
	if session == nil {
		return nil
	}
	return &workers.ProviderContinuationRef{
		Provider:          session.Provider,
		ProviderSessionID: session.ID,
		ExternalRef:       session.ID,
	}
}

func canceledExecuteResult(request workers.ExecuteRequest) workers.ExecuteResult {
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeCanceled,
		Failure: &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeUnknown,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: "execution canceled",
		},
	}
}

func cloneBuildCapabilities(value *workers.Capabilities) *workers.Capabilities {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneBuildStringMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	clone := make(map[string]string, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}

func firstBuildValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func buildContinuationSession(value *workers.ProviderContinuationRef) *providers.SessionRef {
	if value == nil {
		return nil
	}
	return &providers.SessionRef{
		Provider: providers.ID(strings.TrimSpace(value.Provider)),
		Kind:     "session",
		ID:       strings.TrimSpace(value.ProviderSessionID),
	}
}
