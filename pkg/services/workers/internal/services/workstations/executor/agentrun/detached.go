package agentrun

import (
	"context"
	"fmt"
	"sync"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// DetachedRequest is the complete input for one agent-run attempt started from
// the request-scoped Workers Execute path. The caller has already resolved the
// workstation and worker taxonomy, prompt, provider target, and tool policy, so
// this entry point reads no Factory definition and keeps no workstation state.
//
// WSE-09 relocates the request-scoped Workstation implementation; this entry
// point moves with it.
type DetachedRequest struct {
	// Attempt is the resolved provider request that every harness turn runs.
	Attempt workerexecution.RunnerExecutionRequest
	// ProgressPublisher receives the request-scoped canonical final-message
	// observation after a successful harness run.
	ProgressPublisher workerexecution.ProgressPublisher
	// ToolPolicy is the authored agent tool policy applied to the loop. An
	// unset policy disables tool execution.
	ToolPolicy string
	// WorkingDirectory bounds the filesystem tools the policy allows.
	WorkingDirectory string
}

// ExecuteDetached runs one agent loop over runner and reduces it to the runner
// result the caller's own normalization already understands. Outcome
// classification, stop tokens, decision envelopes, and observation delivery
// remain with the caller, exactly as they are for a single provider attempt.
func ExecuteDetached(
	ctx context.Context,
	harness HarnessAdapter,
	runner workerexecution.Runner,
	request DetachedRequest,
) (workerexecution.RunnerExecutionResult, error) {
	if harness == nil {
		return workerexecution.RunnerExecutionResult{}, fmt.Errorf("agent-run harness is required")
	}
	if runner == nil {
		return workerexecution.RunnerExecutionResult{}, fmt.Errorf("agent-run runner is required")
	}
	attempt := request.Attempt
	// The harness owns tool execution for every turn it drives, so the provider
	// request it replays always declares tools required.
	attempt.ToolExecutionMode = workerexecution.RunnerToolExecutionModeRequired
	recorder := NewToolDiagnosticRecorder()
	observed := &lastRunnerResult{runner: runner}
	harnessResult, err := harness.Execute(ctx, HarnessInput{
		SystemPrompt: attempt.SystemPrompt,
		UserMessage:  attempt.UserMessage,
		Inferencer:   newRunnerInferencer(observed, attempt),
		ToolPolicy:   request.ToolPolicy,
		WorkingDir:   request.WorkingDirectory,
		ToolRecorder: recorder,
	})
	// The loop's final message is the last turn the runner produced, so that
	// turn's result is already the answer -- including the Provider Session,
	// provider diagnostics, and the output-policy classification the runner
	// applied. Only a loop that ended without reaching the runner falls back to
	// the harness text.
	result, executed := observed.snapshot()
	finalContent := harnessResult.FinalText
	if !executed {
		result.Content = harnessResult.FinalText
	} else {
		finalContent = result.Content
	}
	result.Diagnostics = mergeAgentRunDiagnostics(
		agentRunDiagnostics(toolDiagnosticsMetadata(request.ToolPolicy, recorder)),
		result.Diagnostics,
	)
	if err != nil {
		return result, err
	}
	publishAgentFinalMessage(
		request.ProgressPublisher,
		request.Attempt.Dispatch.DispatchID,
		finalContent,
	)
	return result, nil
}

// lastRunnerResult keeps the most recent provider turn of one agent loop. It is
// request-scoped: a new value is created for every ExecuteDetached call.
type lastRunnerResult struct {
	runner   workerexecution.Runner
	mu       sync.Mutex
	result   workerexecution.RunnerExecutionResult
	executed bool
}

func (observed *lastRunnerResult) Execute(
	ctx context.Context,
	request workerexecution.RunnerExecutionRequest,
) (workerexecution.RunnerExecutionResult, error) {
	result, err := observed.runner.Execute(ctx, request)
	observed.mu.Lock()
	observed.result = result
	observed.executed = true
	observed.mu.Unlock()
	return result, err
}

func (observed *lastRunnerResult) snapshot() (workerexecution.RunnerExecutionResult, bool) {
	observed.mu.Lock()
	defer observed.mu.Unlock()
	return observed.result, observed.executed
}
