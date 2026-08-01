package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// AgentRunExecutor executes AGENT_RUN workstations through a go-agent-harness adapter.
type AgentRunExecutor struct {
	runtimeConfig     interfaces.RuntimeDefinitionLookup
	decisionEnvelopes factorydefinitionswire.DecisionEnvelopeService
	runner            runnerContract
	harness           HarnessAdapter
	logger            logging.Logger
	recorder          AgentRunEventRecorder
	progress          workerexecution.ProgressPublisher
	now               func() time.Time
}

// WithProgressPublisher publishes the authoritative final agent message after
// the harness has completed all of its internal model/tool turns.
func (executor *AgentRunExecutor) WithProgressPublisher(
	publisher workerexecution.ProgressPublisher,
) *AgentRunExecutor {
	if executor != nil {
		executor.progress = publisher
	}
	return executor
}

var _ workstationRequestExecutor = (*AgentRunExecutor)(nil)

type workstationRequestExecutor interface {
	Execute(ctx context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error)
}

// NewAgentRunExecutor creates an executor that runs agent loops through the harness adapter.
func NewAgentRunExecutor(
	runtimeConfig interfaces.RuntimeDefinitionLookup,
	runner runnerContract,
	toolFileSystem workerexecution.AgentToolFileSystem,
	now func() time.Time,
) *AgentRunExecutor {
	return NewAgentRunExecutorWithDependencies(
		runtimeConfig,
		runner,
		nil,
		NewLibraryHarnessAdapter(toolFileSystem),
		nil,
		now,
	)
}

// NewAgentRunExecutorWithDependencies creates an executor from direct
// collaborators. Nil optional collaborators receive production defaults.
func NewAgentRunExecutorWithDependencies(
	runtimeConfig interfaces.RuntimeDefinitionLookup,
	runner runnerContract,
	logger logging.Logger,
	harness HarnessAdapter,
	recorder AgentRunEventRecorder,
	now func() time.Time,
	decisionEnvelopes ...factorydefinitionswire.DecisionEnvelopeService,
) *AgentRunExecutor {
	return &AgentRunExecutor{
		runtimeConfig:     runtimeConfig,
		decisionEnvelopes: firstDecisionEnvelopeService(decisionEnvelopes),
		runner:            runner,
		harness:           harness,
		logger:            logging.EnsureLogger(logger),
		recorder:          recorder,
		now:               now,
	}
}

func (executor *AgentRunExecutor) Execute(ctx context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	if executor == nil || executor.now == nil {
		return workerexecution.WorkResult{}, fmt.Errorf("agent-run clock is required")
	}
	if executor.harness == nil {
		return workerexecution.WorkResult{}, fmt.Errorf("agent-run harness is required")
	}
	start := executor.clockNow()
	workerType := workerTypeForExecutionRequest(request)
	workerDef, ok := executor.runtimeConfig.Worker(workerType)
	if !ok {
		return missingWorkerWorkResult(request.Dispatch, workerType, executor.clockNow().Sub(start)), nil
	}
	workerDef = effectiveAgentRunWorkerDefinition(request, workerDef)

	baseReq := agentRunInferenceRequest(request, workerDef)
	inferencer := newRunnerInferencer(executor.runner, baseReq)
	toolPolicy := interfaces.EffectiveAgentToolPolicy(workerDef.AgentTools)
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
		duration := executor.clockNow().Sub(start)
		result := agentRunFailureWorkResult(request.Dispatch, err, duration, toolPolicy, toolRecorder)
		executor.recordAgentRunResponse(request.Dispatch, result, duration, harnessResult.Messages)
		return result, nil
	}

	toolMetadata := toolDiagnosticsMetadata(toolPolicy, toolRecorder)

	workstationDef, _ := executor.runtimeConfig.Workstation(request.Dispatch.WorkstationName)
	if executor.decisionEnvelopes != nil &&
		executor.decisionEnvelopes.UsesGoalRoutingDecisionEnvelope(workstationDef) {
		result := executor.decisionEnvelopes.
			WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
				request.Dispatch.DispatchID,
				request.Dispatch.TransitionID,
				harnessResult.FinalText,
			)
		result.Diagnostics = agentRunDiagnostics(toolMetadata)
		duration := executor.clockNow().Sub(start)
		result.Metrics = workerexecution.WorkMetrics{Duration: duration}
		executor.recordAgentRunResponse(request.Dispatch, result, duration, harnessResult.Messages)
		executor.publishFinalMessage(request.Dispatch.DispatchID, result.Output)
		return result, nil
	}
	if executor.decisionEnvelopes != nil &&
		executor.decisionEnvelopes.UsesDecisionEnvelopeOutcome(workstationDef) {
		result := executor.decisionEnvelopes.
			WorkResultFromDecisionEnvelopeJSONOrFailed(
				request.Dispatch.DispatchID,
				request.Dispatch.TransitionID,
				harnessResult.FinalText,
			)
		result.Diagnostics = agentRunDiagnostics(toolMetadata)
		duration := executor.clockNow().Sub(start)
		result.Metrics = workerexecution.WorkMetrics{Duration: duration}
		executor.recordAgentRunResponse(request.Dispatch, result, duration, harnessResult.Messages)
		executor.publishFinalMessage(request.Dispatch.DispatchID, result.Output)
		return result, nil
	}

	outcome := evaluateAgentRunOutcome(harnessResult.FinalText, workerDef)
	result := workerexecution.WorkResult{
		DispatchID:   request.Dispatch.DispatchID,
		TransitionID: request.Dispatch.TransitionID,
		Outcome:      outcome,
		Output:       harnessResult.FinalText,
		Diagnostics:  agentRunDiagnostics(toolMetadata),
		Metrics:      workerexecution.WorkMetrics{Duration: executor.clockNow().Sub(start)},
	}
	executor.recordAgentRunResponse(request.Dispatch, result, result.Metrics.Duration, harnessResult.Messages)
	executor.publishFinalMessage(request.Dispatch.DispatchID, result.Output)
	return result, nil
}

func (executor *AgentRunExecutor) publishFinalMessage(dispatchID string, content string) {
	if executor == nil || executor.progress == nil || strings.TrimSpace(content) == "" {
		return
	}
	payload, err := json.Marshal(workerexecution.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workerexecution.ContentBlock{{
			Kind: workerexecution.ContentBlockText,
			Text: strings.TrimSpace(content),
		}},
	})
	if err != nil {
		return
	}
	draft := workerexecution.Draft{
		Kind:       workerexecution.KindMessage,
		Phase:      workerexecution.PhaseCompleted,
		DispatchID: strings.TrimSpace(dispatchID),
		ItemID:     strings.TrimSpace(dispatchID) + "-final-message",
		Provenance: workerexecution.Provenance{
			Provider:        "agent-run",
			NativeEventType: "agent_final_response",
			Delivery:        workerexecution.DeliveryNativeFinal,
			Representation:  workerexecution.RepresentationSnapshot,
			Fidelity:        workerexecution.FidelityFinalOnly,
		},
		Payload: payload,
	}
	executor.progress(workerexecution.CanonicalDraftFragment(dispatchID, draft))
}

func firstDecisionEnvelopeService(
	services []factorydefinitionswire.DecisionEnvelopeService,
) factorydefinitionswire.DecisionEnvelopeService {
	if len(services) == 0 {
		return nil
	}
	return services[0]
}

func effectiveAgentRunWorkerDefinition(request workerexecution.WorkstationExecutionRequest, workerDef *interfaces.FactoryWorkerConfig) *interfaces.FactoryWorkerConfig {
	if workerDef == nil {
		return workerDef
	}
	effective := *workerDef
	// Workstation execution has already resolved invocation interpolation.
	// An empty resolved value clears an authored placeholder, but it must not
	// erase an operator-level provider/model override already applied to the
	// runtime worker definition.
	if request.Model != "" || isInvocationInterpolation(workerDef.Model) {
		effective.Model = request.Model
	}
	if request.ModelProvider != "" || isInvocationInterpolation(workerDef.ModelProvider) {
		effective.ModelProvider = request.ModelProvider
	}
	if request.ReasoningEffort != "" || isInvocationInterpolation(workerDef.ReasoningEffort) {
		effective.ReasoningEffort = request.ReasoningEffort
	}
	return &effective
}

func isInvocationInterpolation(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}")
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
	return executor.now()
}

func agentRunInferenceRequest(
	request workerexecution.WorkstationExecutionRequest,
	workerDef *interfaces.FactoryWorkerConfig,
) workerexecution.ProviderInferenceRequest {
	req := workerexecution.ProviderInferenceRequest{
		Dispatch:           work.CloneWorkDispatch(request.Dispatch),
		WorkerType:         request.WorkerType,
		WorkstationType:    request.WorkstationType,
		RunnerID:           request.RunnerID,
		ExecutorProvider:   request.ExecutorProvider,
		ProjectID:          request.ProjectID,
		InputTokens:        cloneRawInputTokens(request.InputTokens),
		ModelOperation:     request.ModelOperation,
		ModelBindings:      workerexecution.CloneResolvedModelOperationBindings(request.ModelBindings),
		SystemPrompt:       request.SystemPrompt,
		UserMessage:        request.UserMessage,
		OutputSchema:       request.OutputSchema,
		ToolExecutionMode:  workerexecution.RunnerToolExecutionModeRequired,
		EnvVars:            cloneEnvVars(request.EnvVars),
		ProcessEnvironment: append([]string(nil), request.ProcessEnvironment...),
		Worktree:           request.Worktree,
		WorkingDirectory:   request.WorkingDirectory,
	}
	if workerDef != nil {
		req.Model = workerDef.Model
		req.ModelProvider = workerDef.ModelProvider
		req.ReasoningEffort = workerDef.ReasoningEffort
		req.ModelLocality = workerDef.ModelLocality
		req.SessionID = workerDef.SessionID
	}
	if identity, err := workerexecution.RunnerIdentityForWorker(req.ExecutorProvider, req.ModelProvider); err == nil && identity != "" {
		req.RunnerID = identity
	}
	if strings.TrimSpace(req.RunnerID) == "" {
		req.RunnerID = strings.TrimSpace(req.ModelProvider)
	}
	return req
}

func evaluateAgentRunOutcome(output string, workerDef *interfaces.FactoryWorkerConfig) workerexecution.WorkOutcome {
	if workerDef == nil || workerDef.StopToken == "" {
		return workerexecution.OutcomeAccepted
	}
	if workerexecution.ContainsStopToken(output, workerDef.StopToken) {
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
