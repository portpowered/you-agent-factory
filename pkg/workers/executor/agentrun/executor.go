package agentrun

import (
	"context"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

// AgentRunExecutor executes AGENT_RUN workstations through a go-agent-harness adapter.
type AgentRunExecutor struct {
	runtimeConfig interfaces.RuntimeDefinitionLookup
	runner        runnerContract
	harness       HarnessAdapter
	logger        logging.Logger
}

var _ workstationRequestExecutor = (*AgentRunExecutor)(nil)

type workstationRequestExecutor interface {
	Execute(ctx context.Context, request interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error)
}

// AgentRunExecutorOption configures an AgentRunExecutor.
type AgentRunExecutorOption func(*AgentRunExecutor)

func WithAgentRunLogger(logger logging.Logger) AgentRunExecutorOption {
	return func(executor *AgentRunExecutor) {
		executor.logger = logging.EnsureLogger(logger)
	}
}

func WithAgentRunHarness(harness HarnessAdapter) AgentRunExecutorOption {
	return func(executor *AgentRunExecutor) {
		executor.harness = harness
	}
}

// NewAgentRunExecutor creates an executor that runs agent loops through the harness adapter.
func NewAgentRunExecutor(
	runtimeConfig interfaces.RuntimeDefinitionLookup,
	runner runnerContract,
	opts ...AgentRunExecutorOption,
) *AgentRunExecutor {
	executor := &AgentRunExecutor{
		runtimeConfig: runtimeConfig,
		runner:        runner,
		harness:       NewLibraryHarnessAdapter(),
		logger:        logging.NoopLogger{},
	}
	for _, opt := range opts {
		opt(executor)
	}
	return executor
}

func (executor *AgentRunExecutor) Execute(ctx context.Context, request interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	start := time.Now()
	workerType := workerTypeForExecutionRequest(request)
	workerDef, ok := executor.runtimeConfig.Worker(workerType)
	if !ok {
		return missingWorkerWorkResult(request.Dispatch, workerType, time.Since(start)), nil
	}

	baseReq := agentRunInferenceRequest(request, workerDef)
	inferencer := newRunnerInferencer(executor.runner, baseReq)
	harnessResult, err := executor.harness.Execute(ctx, HarnessInput{
		SystemPrompt: request.SystemPrompt,
		UserMessage:  request.UserMessage,
		Inferencer:   inferencer,
	})
	if err != nil {
		return agentRunFailureWorkResult(request.Dispatch, err, time.Since(start)), nil
	}

	workstationDef, _ := executor.runtimeConfig.Workstation(request.Dispatch.WorkstationName)
	if goal.UsesGoalRoutingDecisionEnvelope(workstationDef) {
		result := goal.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
			request.Dispatch.DispatchID,
			request.Dispatch.TransitionID,
			harnessResult.FinalText,
		)
		result.Diagnostics = agentRunDiagnostics(nil)
		result.Metrics = interfaces.WorkMetrics{Duration: time.Since(start)}
		return result, nil
	}
	if goal.UsesDecisionEnvelopeOutcome(workstationDef) {
		result := goal.WorkResultFromDecisionEnvelopeJSONOrFailed(
			request.Dispatch.DispatchID,
			request.Dispatch.TransitionID,
			harnessResult.FinalText,
		)
		result.Diagnostics = agentRunDiagnostics(nil)
		result.Metrics = interfaces.WorkMetrics{Duration: time.Since(start)}
		return result, nil
	}

	outcome := evaluateAgentRunOutcome(harnessResult.FinalText, workerDef)
	return interfaces.WorkResult{
		DispatchID:   request.Dispatch.DispatchID,
		TransitionID: request.Dispatch.TransitionID,
		Outcome:      outcome,
		Output:       harnessResult.FinalText,
		Diagnostics:  agentRunDiagnostics(nil),
		Metrics:      interfaces.WorkMetrics{Duration: time.Since(start)},
	}, nil
}

func agentRunInferenceRequest(
	request interfaces.WorkstationExecutionRequest,
	workerDef *interfaces.WorkerConfig,
) interfaces.ProviderInferenceRequest {
	req := interfaces.ProviderInferenceRequest{
		Dispatch:          interfaces.CloneWorkDispatch(request.Dispatch),
		WorkerType:        request.WorkerType,
		WorkstationType:   request.WorkstationType,
		RunnerID:          request.RunnerID,
		ProjectID:         request.ProjectID,
		InputTokens:       cloneRawInputTokens(request.InputTokens),
		ModelOperation:    request.ModelOperation,
		ModelBindings:     interfaces.CloneResolvedModelOperationBindings(request.ModelBindings),
		SystemPrompt:      request.SystemPrompt,
		UserMessage:       request.UserMessage,
		OutputSchema:      request.OutputSchema,
		ToolExecutionMode: interfaces.RunnerToolExecutionModeRequired,
		EnvVars:           cloneEnvVars(request.EnvVars),
		Worktree:          request.Worktree,
		WorkingDirectory:  request.WorkingDirectory,
	}
	if workerDef != nil {
		req.Model = workerDef.Model
		req.ModelProvider = workerDef.ModelProvider
		req.ModelLocality = workerDef.ModelLocality
		req.SessionID = workerDef.SessionID
	}
	return req
}

func evaluateAgentRunOutcome(output string, workerDef *interfaces.WorkerConfig) interfaces.WorkOutcome {
	if workerDef == nil || workerDef.StopToken == "" {
		return interfaces.OutcomeAccepted
	}
	if workerprovider.ContainsStopToken(output, workerDef.StopToken) {
		return interfaces.OutcomeAccepted
	}
	if strings.Contains(output, "<CONTINUE>") {
		return interfaces.OutcomeContinue
	}
	return interfaces.OutcomeRejected
}

func agentRunFailureWorkResult(dispatch interfaces.WorkDispatch, err error, duration time.Duration) interfaces.WorkResult {
	failureClass := failureClassForError(err)
	return interfaces.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         interfaces.OutcomeFailed,
		Error:           formatAgentRunError(err),
		FailureMetadata: interfaces.CloneWorkFailureMetadata(failureMetadataForError(err)),
		Diagnostics: agentRunDiagnostics(map[string]string{
			DiagnosticFailureClass: failureClass,
		}),
		Metrics: interfaces.WorkMetrics{Duration: duration},
	}
}

func missingWorkerWorkResult(dispatch interfaces.WorkDispatch, workerType string, duration time.Duration) interfaces.WorkResult {
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeFailed,
		Error:        "worker config not found: " + workerType,
		Diagnostics:  agentRunDiagnostics(nil),
		Metrics:      interfaces.WorkMetrics{Duration: duration},
	}
}

func workerTypeForExecutionRequest(request interfaces.WorkstationExecutionRequest) string {
	if request.WorkerType != "" {
		return request.WorkerType
	}
	return request.Dispatch.WorkerType
}

func cloneRawInputTokens(raw []any) []any {
	if len(raw) == 0 {
		return nil
	}
	out := make([]any, len(raw))
	copy(out, raw)
	return out
}

func cloneEnvVars(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for key, value := range env {
		out[key] = value
	}
	return out
}
