package agentrun

import (
	"context"
	"strings"
	"time"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/work"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

// AgentRunExecutor executes AGENT_RUN workstations through a go-agent-harness adapter.
type AgentRunExecutor struct {
	runtimeConfig interfaces.RuntimeDefinitionLookup
	runner        runnerContract
	harness       HarnessAdapter
	logger        logging.Logger
	recorder      AgentRunEventRecorder
	now           func() time.Time
}

var _ workstationRequestExecutor = (*AgentRunExecutor)(nil)

type workstationRequestExecutor interface {
	Execute(ctx context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error)
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

func WithAgentRunEventRecorder(recorder AgentRunEventRecorder) AgentRunExecutorOption {
	return func(executor *AgentRunExecutor) {
		if recorder != nil {
			executor.recorder = recorder
		}
	}
}

func WithAgentRunClock(now func() time.Time) AgentRunExecutorOption {
	return func(executor *AgentRunExecutor) {
		if now != nil {
			executor.now = now
		}
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

func (executor *AgentRunExecutor) Execute(ctx context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	start := executor.clockNow()
	workerType := workerTypeForExecutionRequest(request)
	workerDef, ok := executor.runtimeConfig.Worker(workerType)
	if !ok {
		return missingWorkerWorkResult(request.Dispatch, workerType, time.Since(start)), nil
	}
	workerDef = effectiveAgentRunWorkerDefinition(request, workerDef)

	baseReq := agentRunInferenceRequest(request, workerDef)
	inferencer := newRunnerInferencer(executor.runner, baseReq)
	toolPolicy := workerconfig.EffectiveAgentToolPolicy(workerDef.AgentTools)
	toolRecorder := NewToolDiagnosticRecorder()
	harnessResult, err := executor.harness.Execute(ctx, HarnessInput{
		SystemPrompt: request.SystemPrompt,
		UserMessage:  request.UserMessage,
		Inferencer:   inferencer,
		ToolPolicy:   toolPolicy,
		WorkingDir:   request.WorkingDirectory,
		ToolRecorder: toolRecorder,
	})
	if err != nil {
		result := agentRunFailureWorkResult(request.Dispatch, err, time.Since(start), toolPolicy, toolRecorder)
		executor.recordAgentRunResponse(request.Dispatch, result, time.Since(start), harnessResult.Messages)
		return result, nil
	}

	toolMetadata := toolDiagnosticsMetadata(toolPolicy, toolRecorder)

	workstationDef, _ := executor.runtimeConfig.Workstation(request.Dispatch.WorkstationName)
	if goal.UsesGoalRoutingDecisionEnvelope(workstationDef) {
		result := goal.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
			request.Dispatch.DispatchID,
			request.Dispatch.TransitionID,
			harnessResult.FinalText,
		)
		result.Diagnostics = agentRunDiagnostics(toolMetadata)
		result.Metrics = workerexecution.WorkMetrics{Duration: time.Since(start)}
		executor.recordAgentRunResponse(request.Dispatch, result, time.Since(start), harnessResult.Messages)
		return result, nil
	}
	if goal.UsesDecisionEnvelopeOutcome(workstationDef) {
		result := goal.WorkResultFromDecisionEnvelopeJSONOrFailed(
			request.Dispatch.DispatchID,
			request.Dispatch.TransitionID,
			harnessResult.FinalText,
		)
		result.Diagnostics = agentRunDiagnostics(toolMetadata)
		result.Metrics = workerexecution.WorkMetrics{Duration: time.Since(start)}
		executor.recordAgentRunResponse(request.Dispatch, result, time.Since(start), harnessResult.Messages)
		return result, nil
	}

	outcome := evaluateAgentRunOutcome(harnessResult.FinalText, workerDef)
	result := workerexecution.WorkResult{
		DispatchID:   request.Dispatch.DispatchID,
		TransitionID: request.Dispatch.TransitionID,
		Outcome:      outcome,
		Output:       harnessResult.FinalText,
		Diagnostics:  agentRunDiagnostics(toolMetadata),
		Metrics:      workerexecution.WorkMetrics{Duration: time.Since(start)},
	}
	executor.recordAgentRunResponse(request.Dispatch, result, time.Since(start), harnessResult.Messages)
	return result, nil
}

func effectiveAgentRunWorkerDefinition(request workerexecution.WorkstationExecutionRequest, workerDef *workerconfig.Config) *workerconfig.Config {
	if workerDef == nil || (request.Model == "" && request.ModelProvider == "") {
		return workerDef
	}
	effective := *workerDef
	if request.Model != "" {
		effective.Model = request.Model
	}
	if request.ModelProvider != "" {
		effective.ModelProvider = request.ModelProvider
	}
	return &effective
}

func (executor *AgentRunExecutor) recordAgentRunResponse(
	dispatch work.WorkDispatch,
	result workerexecution.WorkResult,
	duration time.Duration,
	transcript []messages.Message,
) {
	if executor == nil || executor.recorder == nil || dispatch.DispatchID == "" {
		return
	}
	executor.recorder(agentRunResponseEvent(dispatch, result, duration, transcript, executor.clockNow()))
}

func (executor *AgentRunExecutor) clockNow() time.Time {
	if executor != nil && executor.now != nil {
		return executor.now()
	}
	return time.Now()
}

func agentRunInferenceRequest(
	request workerexecution.WorkstationExecutionRequest,
	workerDef *workerconfig.Config,
) workerexecution.ProviderInferenceRequest {
	req := workerexecution.ProviderInferenceRequest{
		Dispatch:          work.CloneWorkDispatch(request.Dispatch),
		WorkerType:        request.WorkerType,
		WorkstationType:   request.WorkstationType,
		RunnerID:          request.RunnerID,
		ProjectID:         request.ProjectID,
		InputTokens:       cloneRawInputTokens(request.InputTokens),
		ModelOperation:    request.ModelOperation,
		ModelBindings:     workerexecution.CloneResolvedModelOperationBindings(request.ModelBindings),
		SystemPrompt:      request.SystemPrompt,
		UserMessage:       request.UserMessage,
		OutputSchema:      request.OutputSchema,
		ToolExecutionMode: workerexecution.RunnerToolExecutionModeRequired,
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

func evaluateAgentRunOutcome(output string, workerDef *workerconfig.Config) workerexecution.WorkOutcome {
	if workerDef == nil || workerDef.StopToken == "" {
		return workerexecution.OutcomeAccepted
	}
	if workerprovider.ContainsStopToken(output, workerDef.StopToken) {
		return workerexecution.OutcomeAccepted
	}
	if strings.Contains(output, "<CONTINUE>") {
		return workerexecution.OutcomeContinue
	}
	return workerexecution.OutcomeRejected
}

func agentRunFailureWorkResult(
	dispatch work.WorkDispatch,
	err error,
	duration time.Duration,
	toolPolicy string,
	toolRecorder *ToolDiagnosticRecorder,
) workerexecution.WorkResult {
	failureDiagnostics := agentRunFailureDiagnostics(err)
	if toolRecorder != nil {
		failureDiagnostics = mergeToolDiagnostics(failureDiagnostics, toolPolicy, toolRecorder)
	}
	return workerexecution.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         workerexecution.OutcomeFailed,
		Error:           formatAgentRunError(err),
		FailureMetadata: workerexecution.CloneWorkFailureMetadata(failureMetadataForError(err)),
		Diagnostics:     agentRunDiagnostics(failureDiagnostics),
		Metrics:         workerexecution.WorkMetrics{Duration: duration},
	}
}

func missingWorkerWorkResult(dispatch work.WorkDispatch, workerType string, duration time.Duration) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "worker config not found: " + workerType,
		Diagnostics:  agentRunDiagnostics(nil),
		Metrics:      workerexecution.WorkMetrics{Duration: duration},
	}
}

func workerTypeForExecutionRequest(request workerexecution.WorkstationExecutionRequest) string {
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
